// Package agent implements the AI Agent with Tool-Use capabilities.
package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jeeinn/matea/internal/llm"
	"github.com/jeeinn/matea/internal/sandbox"
)

// ToolFunc is the function signature for tool implementations.
type ToolFunc func(params map[string]interface{}) (string, error)

// ToolDef defines a tool that the LLM can call.
type ToolDef struct {
	Name        string
	Description string
	Parameters  llm.Parameters
	Fn          ToolFunc
}

// ToLLMTool converts a ToolDef to an llm.Tool for the API request.
func (t *ToolDef) ToLLMTool() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.Function{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		},
	}
}

// ToolRegistry manages the available tools.
type ToolRegistry struct {
	tools map[string]*ToolDef
}

// NewToolRegistry creates a new ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*ToolDef),
	}
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(tool *ToolDef) {
	r.tools[tool.Name] = tool
}

// Get returns a tool by name.
func (r *ToolRegistry) Get(name string) (*ToolDef, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// ToLLMTools returns all tools as llm.Tool slice for the API request.
func (r *ToolRegistry) ToLLMTools() []llm.Tool {
	tools := make([]llm.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t.ToLLMTool())
	}
	return tools
}

// ExecuteTool executes a tool call from the LLM.
func (r *ToolRegistry) ExecuteTool(call llm.ToolCall) (string, error) {
	tool, ok := r.tools[call.Function.Name]
	if !ok {
		// Common shell command names are sometimes emitted as tool names.
		// Rewrite to the canonical tool when possible; otherwise return a hint.
		if rewritten, rewrittenOK := rewriteShellAliasCall(call); rewrittenOK {
			if t, exists := r.tools[rewritten.Function.Name]; exists {
				call = rewritten
				tool = t
				ok = true
			}
		}
	}
	if !ok {
		if hint, hintOK := shellAliasHint(call.Function.Name); hintOK {
			return "", fmt.Errorf("unknown tool: %s; %s", call.Function.Name, hint)
		}
		return "", fmt.Errorf("unknown tool: %s; available tools: %s", call.Function.Name, r.availableToolNames())
	}

	// Parse arguments
	var params map[string]interface{}
	if call.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
			return "", fmt.Errorf("parse arguments: %w", err)
		}
	}

	return tool.Fn(params)
}

// --- Tool builders (individual, for AssembleToolRegistry) ------------------

func newReadFileTool(sb *sandbox.Sandbox) *ToolDef {
	return &ToolDef{
		Name:        "read_file",
		Description: "Read the contents of a file at the given path. Returns the file content as a string.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"path": {
					Type:        "string",
					Description: "The path to the file to read, relative to the workspace root.",
				},
			},
			Required: []string{"path"},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			path, _ := params["path"].(string)
			if path == "" {
				return "", fmt.Errorf("path is required")
			}
			content, err := sb.ReadFile(path)
			if err != nil {
				return fmt.Sprintf("Error: %v", err), nil
			}
			return string(content), nil
		},
	}
}

func newWriteFileTool(sb *sandbox.Sandbox) *ToolDef {
	return &ToolDef{
		Name:        "write_file",
		Description: "Write content to a file at the given path. Creates the file if it doesn't exist, overwrites if it does. Creates parent directories as needed.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"path": {
					Type:        "string",
					Description: "The path to the file to write, relative to the workspace root.",
				},
				"content": {
					Type:        "string",
					Description: "The content to write to the file.",
				},
			},
			Required: []string{"path", "content"},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			path, _ := params["path"].(string)
			content, _ := params["content"].(string)
			if path == "" {
				return "", fmt.Errorf("path is required")
			}
			if err := sb.WriteFile(path, []byte(content)); err != nil {
				return fmt.Sprintf("Error: %v", err), nil
			}
			return fmt.Sprintf("File written successfully: %s", path), nil
		},
	}
}

func newListFilesTool(sb *sandbox.Sandbox) *ToolDef {
	return &ToolDef{
		Name:        "list_files",
		Description: "List files under the given path (max depth 3). Returns one path per line.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"path": {
					Type:        "string",
					Description: "The directory path to list. Defaults to current directory if empty.",
				},
			},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			path, _ := params["path"].(string)
			cmd, args := listFilesCmd(path)
			result := sb.Execute(cmd, args...)
			if result.Error != nil && result.Stdout == "" {
				return fmt.Sprintf("Error: %v\n%s", result.Error, result.Stderr), nil
			}
			if result.Stdout == "" {
				return "(no files found)", nil
			}
			return result.Stdout, nil
		},
	}
}

func newSearchCodeTool(sb *sandbox.Sandbox) *ToolDef {
	return &ToolDef{
		Name:        "search_code",
		Description: "Search for a pattern in the codebase. Returns matching lines with file paths and line numbers.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"pattern": {
					Type:        "string",
					Description: "The search pattern (supports regex).",
				},
				"path": {
					Type:        "string",
					Description: "The directory to search in. Defaults to current directory if empty.",
				},
			},
			Required: []string{"pattern"},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			pattern, _ := params["pattern"].(string)
			path, _ := params["path"].(string)
			if pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}
			cmd, args := searchCodeCmd(pattern, path)
			result := sb.Execute(cmd, args...)
			// grep exit 1 = no matches; PowerShell may return empty stdout
			if result.Error != nil && result.ExitCode != 1 && result.Stdout == "" {
				return fmt.Sprintf("Error: %v\n%s", result.Error, result.Stderr), nil
			}
			if result.Stdout == "" {
				return "No matches found.", nil
			}
			return result.Stdout, nil
		},
	}
}

func newRgTool(sb *sandbox.Sandbox) *ToolDef {
	return &ToolDef{
		Name: "rg",
		Description: "Fast codebase search via ripgrep (preferred over search_code when available). " +
			"Returns matching lines with file paths and line numbers. Falls back to search_code if rg is not installed.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"pattern": {
					Type:        "string",
					Description: "The search pattern (supports regex).",
				},
				"path": {
					Type:        "string",
					Description: "Directory or file to search. Defaults to current directory if empty.",
				},
				"glob": {
					Type:        "string",
					Description: "Optional file glob filter (e.g. '*.go', '*.{ts,tsx}').",
				},
			},
			Required: []string{"pattern"},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			pattern, _ := params["pattern"].(string)
			path, _ := params["path"].(string)
			glob, _ := params["glob"].(string)
			if pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}

			result := sb.Execute("rg", rgCmd(pattern, path, glob)...)
			if isCommandMissing(result) {
				cmd, args := searchCodeCmd(pattern, path)
				result = sb.Execute(cmd, args...)
			}
			// rg/grep exit 1 = no matches
			if result.Error != nil && result.ExitCode != 1 && result.Stdout == "" {
				return fmt.Sprintf("Error: %v\n%s", result.Error, result.Stderr), nil
			}
			if result.Stdout == "" {
				return "No matches found.", nil
			}
			return result.Stdout, nil
		},
	}
}

func newRunCommandTool(sb *sandbox.Sandbox) *ToolDef {
	return &ToolDef{
		Name:        "run_command",
		Description: "Execute a shell command in the workspace. On Windows uses cmd /C; on Unix uses sh -c. Prefer portable commands (go, git) when possible.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"command": {
					Type:        "string",
					Description: "The command to execute (e.g., 'go build ./...', 'go test ./...'). On Windows use cmd syntax (dir); on Unix use sh syntax (ls -la).",
				},
			},
			Required: []string{"command"},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			command, _ := params["command"].(string)
			if command == "" {
				return "", fmt.Errorf("command is required")
			}
			result := sb.ExecuteShell(command)
			output := result.Stdout
			if result.Stderr != "" {
				output += "\n" + result.Stderr
			}
			if result.Error != nil {
				output += fmt.Sprintf("\nError: %v", result.Error)
			}
			output += fmt.Sprintf("\nExit code: %d", result.ExitCode)
			return output, nil
		},
	}
}

func newApplyDiffTool(sb *sandbox.Sandbox) *ToolDef {
	return &ToolDef{
		Name:        "apply_diff",
		Description: "Apply a unified diff patch to a file. The diff should be in standard unified diff format.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"diff": {
					Type:        "string",
					Description: "The unified diff patch to apply.",
				},
			},
			Required: []string{"diff"},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			diff, _ := params["diff"].(string)
			if diff == "" {
				return "", fmt.Errorf("diff is required")
			}
			if err := sb.WriteFile("_patch.diff", []byte(diff)); err != nil {
				return fmt.Sprintf("Error writing diff: %v", err), nil
			}
			result := sb.Execute("git", "apply", "_patch.diff")
			rmCmd, rmArgs := removeFileCmd("_patch.diff")
			sb.Execute(rmCmd, rmArgs...)
			if result.Error != nil {
				return fmt.Sprintf("Error applying diff: %v\n%s", result.Error, result.Stderr), nil
			}
			return "Diff applied successfully.", nil
		},
	}
}

func newTreeTool(sb *sandbox.Sandbox) *ToolDef {
	return &ToolDef{
		Name:        "tree",
		Description: "Show directory structure (files and directories). Useful for understanding project layout.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"path": {
					Type:        "string",
					Description: "Directory path to show. Defaults to current directory.",
				},
				"depth": {
					Type:        "number",
					Description: "Maximum depth to show. Defaults to 3.",
				},
			},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			path, _ := params["path"].(string)
			depth := 3
			if d, ok := params["depth"].(float64); ok && d > 0 {
				depth = int(d)
			}
			cmd, args := treeCmd(path, depth)
			result := sb.Execute(cmd, args...)
			if result.Error != nil && result.Stdout == "" {
				return fmt.Sprintf("Error: %v\n%s", result.Error, result.Stderr), nil
			}
			if result.Stdout == "" {
				return "(empty)", nil
			}
			return result.Stdout, nil
		},
	}
}

func newGitLogTool(sb *sandbox.Sandbox) *ToolDef {
	return &ToolDef{
		Name:        "git_log",
		Description: "Show git commit history. Useful for understanding recent changes.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"count": {
					Type:        "number",
					Description: "Number of commits to show. Defaults to 10.",
				},
				"path": {
					Type:        "string",
					Description: "File or directory to show history for. Optional.",
				},
			},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			count := 10
			if c, ok := params["count"].(float64); ok && c > 0 {
				count = int(c)
			}
			path, _ := params["path"].(string)

			args := []string{"log", fmt.Sprintf("-%d", count), "--oneline", "--graph"}
			if path != "" {
				args = append(args, "--", path)
			}
			result := sb.Execute("git", args...)
			if result.Error != nil {
				return fmt.Sprintf("Error: %v", result.Error), nil
			}
			if result.Stdout == "" {
				return "No commits found.", nil
			}
			return result.Stdout, nil
		},
	}
}

func newGitBlameTool(sb *sandbox.Sandbox) *ToolDef {
	return &ToolDef{
		Name:        "git_blame",
		Description: "Show who last modified each line of a file. Useful for understanding code history.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"path": {
					Type:        "string",
					Description: "File to show blame for.",
				},
				"start_line": {
					Type:        "number",
					Description: "Start line number (1-based). Optional.",
				},
				"end_line": {
					Type:        "number",
					Description: "End line number (1-based). Optional.",
				},
			},
			Required: []string{"path"},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			path, _ := params["path"].(string)
			if path == "" {
				return "", fmt.Errorf("path is required")
			}

			args := []string{"blame", "--porcelain"}
			if start, ok := params["start_line"].(float64); ok && start > 0 {
				if end, ok := params["end_line"].(float64); ok && end > start {
					args = append(args, fmt.Sprintf("-L %d,%d", int(start), int(end)))
				}
			}
			args = append(args, path)

			result := sb.Execute("git", args...)
			if result.Error != nil {
				return fmt.Sprintf("Error: %v", result.Error), nil
			}
			return result.Stdout, nil
		},
	}
}

// toolBuilders maps canonical tool names to their constructors.
// All built-in tools must be registered here so that AssembleToolRegistry
// can construct a registry from a pack definition.
var toolBuilders = map[string]func(*sandbox.Sandbox) *ToolDef{
	"read_file":   newReadFileTool,
	"write_file":  newWriteFileTool,
	"list_files":  newListFilesTool,
	"search_code": newSearchCodeTool,
	"rg":          newRgTool,
	"run_command": newRunCommandTool,
	"apply_diff":  newApplyDiffTool,
	"tree":        newTreeTool,
	"git_log":     newGitLogTool,
	"git_blame":   newGitBlameTool,
}

// KnownToolNames returns the sorted list of all built-in tool names.
// Useful for validation, documentation, and config generation.
func KnownToolNames() []string {
	names := make([]string, 0, len(toolBuilders))
	for n := range toolBuilders {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// AssembleToolRegistry builds a ToolRegistry from an ordered list of tool
// names. Unknown names return an error so that typos in config are caught
// early rather than silently dropping tools.
//
// The caller (runner) decides which pack to use: e.g.
//
//	coder-default    → all tools (read+write)
//	analyze-readonly → read-only subset
func AssembleToolRegistry(toolNames []string, sb *sandbox.Sandbox) (*ToolRegistry, error) {
	registry := NewToolRegistry()
	var unknown []string
	for _, name := range toolNames {
		builder, ok := toolBuilders[name]
		if !ok {
			unknown = append(unknown, name)
			continue
		}
		registry.Register(builder(sb))
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown tool(s): %s; available tool builders: %s", strings.Join(unknown, ", "), strings.Join(availableBuilderNames(), ", "))
	}
	return registry, nil
}

// availableToolNames returns the registered tool names, sorted for stable
// output (used in "unknown tool" error messages).
func (r *ToolRegistry) availableToolNames() string {
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// availableBuilderNames returns the names of all known tool builders.
func availableBuilderNames() []string {
	names := make([]string, 0, len(toolBuilders))
	for n := range toolBuilders {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// rewriteShellAliasCall maps common shell-style tool names to canonical tools.
// Returns the rewritten call and true when a safe rewrite is available.
func rewriteShellAliasCall(call llm.ToolCall) (llm.ToolCall, bool) {
	var params map[string]interface{}
	if call.Function.Arguments != "" {
		_ = json.Unmarshal([]byte(call.Function.Arguments), &params)
	}

	switch call.Function.Name {
	case "ls", "dir":
		path := "."
		if p, _ := params["path"].(string); strings.TrimSpace(p) != "" {
			path = p
		}
		args, _ := json.Marshal(map[string]string{"path": path})
		return llm.ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: llm.FuncCall{
				Name:      "list_files",
				Arguments: string(args),
			},
		}, true
	case "pwd":
		return llm.ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: llm.FuncCall{
				Name:      "run_command",
				Arguments: `{"command":"pwd"}`,
			},
		}, true
	case "cat", "type":
		path, _ := params["path"].(string)
		if path == "" {
			path, _ = params["file"].(string)
		}
		if strings.TrimSpace(path) == "" {
			return call, false
		}
		args, _ := json.Marshal(map[string]string{"path": path})
		return llm.ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: llm.FuncCall{
				Name:      "read_file",
				Arguments: string(args),
			},
		}, true
	default:
		return call, false
	}
}

// shellAliasHint maps common shell command names (often emitted as tool calls
// by the model) to guidance when an automatic rewrite is not possible.
func shellAliasHint(name string) (string, bool) {
	switch name {
	case "ls", "dir":
		return `use "list_files" (or "run_command" with ls)`, true
	case "cat", "type":
		return `use "read_file" with {"path":"..."}; shell cat must go through "run_command"`, true
	case "pwd":
		return `the workspace root is the current directory; shell pwd must go through "run_command"`, true
	case "cd":
		return `all paths are relative to the workspace root; use "run_command" only if you must change directory`, true
	case "grep", "find":
		return `use "search_code" or "rg" instead of shell grep/find`, true
	}
	return "", false
}

// DefaultTools creates the default set of tools for code editing.
// It is equivalent to AssembleToolRegistry with the "coder-default" pack.
// Kept for backward compatibility; new code should use AssembleToolRegistry.
func DefaultTools(sb *sandbox.Sandbox) *ToolRegistry {
	registry, _ := AssembleToolRegistry([]string{
		"read_file", "write_file", "list_files", "search_code", "rg",
		"run_command", "apply_diff", "tree", "git_log", "git_blame",
	}, sb)
	return registry
}
