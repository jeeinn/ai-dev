// Package agent implements the AI Agent with Tool-Use capabilities.
package agent

import (
	"encoding/json"
	"fmt"

	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/sandbox"
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
		return "", fmt.Errorf("unknown tool: %s", call.Function.Name)
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

// DefaultTools creates the default set of tools for code editing.
func DefaultTools(sb *sandbox.Sandbox) *ToolRegistry {
	registry := NewToolRegistry()

	// read_file
	registry.Register(&ToolDef{
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
	})

	// write_file
	registry.Register(&ToolDef{
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
	})

	// list_files
	registry.Register(&ToolDef{
		Name:        "list_files",
		Description: "List files and directories at the given path. Returns a tree-like structure.",
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
			result := sb.Execute("find", path, "-maxdepth", "3", "-type", "f")
			if result.Error != nil {
				return fmt.Sprintf("Error: %v", result.Error), nil
			}
			return result.Stdout, nil
		},
	})

	// search_code
	registry.Register(&ToolDef{
		Name:        "search_code",
		Description: "Search for a pattern in the codebase using grep. Returns matching lines with file paths.",
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
			if path == "" {
				path = "."
			}
			if pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}
			result := sb.Execute("grep", "-rn", "--include=*.go", "--include=*.py", "--include=*.js", "--include=*.ts", pattern, path)
			if result.Error != nil && result.ExitCode != 1 {
				return fmt.Sprintf("Error: %v", result.Error), nil
			}
			if result.Stdout == "" {
				return "No matches found.", nil
			}
			return result.Stdout, nil
		},
	})

	// run_command
	registry.Register(&ToolDef{
		Name:        "run_command",
		Description: "Execute a shell command in the workspace. Use for building, testing, or inspecting the project.",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"command": {
					Type:        "string",
					Description: "The command to execute (e.g., 'go build ./...', 'go test ./...', 'ls -la').",
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
	})

	// apply_diff
	registry.Register(&ToolDef{
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
			// Write diff to temp file
			if err := sb.WriteFile("_patch.diff", []byte(diff)); err != nil {
				return fmt.Sprintf("Error writing diff: %v", err), nil
			}
			// Apply patch
			result := sb.Execute("git", "apply", "_patch.diff")
			// Clean up
			sb.Execute("rm", "_patch.diff")
			if result.Error != nil {
				return fmt.Sprintf("Error applying diff: %v\n%s", result.Error, result.Stderr), nil
			}
			return "Diff applied successfully.", nil
		},
	})

	return registry
}
