package agent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jeeinn/matea/internal/sandbox"
)

// CodeContext holds information about the codebase for the LLM.
type CodeContext struct {
	Structure   string // Directory tree
	KeyFiles    string // Contents of key files
	TotalTokens int    // Approximate token count
}

// LoadCodeContext loads the codebase context for the LLM.
// It scans the directory structure and reads key files.
func LoadCodeContext(sb *sandbox.Sandbox, maxTokens int) (*CodeContext, error) {
	ctx := &CodeContext{}

	// Load directory structure
	structure, err := loadDirectoryStructure(sb, ".", 3)
	if err != nil {
		return nil, fmt.Errorf("load directory structure: %w", err)
	}
	ctx.Structure = structure

	// Load key files
	keyFiles := identifyKeyFiles(structure)
	var fileContents []string
	totalTokens := estimateTokens(structure)

	for _, file := range keyFiles {
		content, err := sb.ReadFile(file)
		if err != nil {
			continue // Skip files that can't be read
		}

		fileStr := fmt.Sprintf("=== %s ===\n%s", file, string(content))
		fileTokens := estimateTokens(fileStr)

		if totalTokens+fileTokens > maxTokens {
			break // Stop if we exceed token limit
		}

		fileContents = append(fileContents, fileStr)
		totalTokens += fileTokens
	}

	ctx.KeyFiles = strings.Join(fileContents, "\n\n")
	ctx.TotalTokens = totalTokens

	return ctx, nil
}

// loadDirectoryStructure recursively loads the directory structure.
func loadDirectoryStructure(sb *sandbox.Sandbox, dir string, maxDepth int) (string, error) {
	cmd, args := treeCmd(dir, maxDepth)
	result := sb.Execute(cmd, args...)
	if result.Error != nil && result.Stdout == "" {
		return "", result.Error
	}
	return result.Stdout, nil
}

// identifyKeyFiles identifies key files to load based on the directory structure.
func identifyKeyFiles(structure string) []string {
	var files []string

	// Common key files
	keyPatterns := []string{
		"go.mod",
		"go.sum",
		"main.go",
		"README.md",
		"Makefile",
		"Dockerfile",
		".env.example",
		"package.json",
		"requirements.txt",
		"Cargo.toml",
	}

	lines := strings.Split(structure, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this line matches a key file pattern
		for _, pattern := range keyPatterns {
			if strings.HasSuffix(line, pattern) {
				files = append(files, line)
				break
			}
		}
	}

	// Add some Go source files if this is a Go project
	goFileCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".go") && !strings.HasSuffix(line, "_test.go") {
			files = append(files, line)
			goFileCount++
			if goFileCount >= 5 {
				break
			}
		}
	}

	return files
}

var cjkRegex = regexp.MustCompile(`[\p{Han}\p{Katakana}\p{Hiragana}\p{Hangul}]+`)

// estimateTokens estimates the token count for a string.
// Uses differentiated approximation: CJK ≈ 2 chars/token, other ≈ 4 chars/token.
func estimateTokens(s string) int {
	matches := cjkRegex.FindAllString(s, -1)
	cjkLen := 0
	for _, m := range matches {
		cjkLen += len(m)
	}
	otherLen := len(s) - cjkLen
	return (cjkLen / 2) + (otherLen / 4)
}

// FormatCodeContext formats the code context for the LLM prompt.
func FormatCodeContext(ctx *CodeContext) string {
	var sb strings.Builder

	sb.WriteString("## Codebase Structure\n\n")
	sb.WriteString("```\n")
	sb.WriteString(ctx.Structure)
	sb.WriteString("```\n\n")

	if ctx.KeyFiles != "" {
		sb.WriteString("## Key Files\n\n")
		sb.WriteString(ctx.KeyFiles)
		sb.WriteString("\n")
	}

	return sb.String()
}

// TaskContext contains the task information for prompt building.
type TaskContext struct {
	IssueTitle string
	IssueBody  string
	RepoName   string
	TaskType   string
}

// BuildSolveToolPrompt returns explicit tool-use instructions for solve-type tasks.
func BuildSolveToolPrompt() string {
	return `## Tool-use requirements

You MUST implement changes using tools — do not stop at analysis-only responses.

- Use write_file or apply_diff to modify source code
- Use read_file, list_files, rg (preferred), and search_code to explore the codebase
- Run tests with run_command when applicable (e.g. go test, npm test)
- The gateway may run configured verify_commands after you finish; ensure tests pass before ending
- The gateway automatically commits, pushes, and opens a PR when git detects changes
- Prioritize delivering working code changes within your iteration budget; avoid lengthy analysis-only final messages
- If a change attempt fails, try a different approach — repeating the same failing edit wastes iterations`
}

// MergeAgentSystemPrompt appends WebUI agent instructions to a base task prompt.
func MergeAgentSystemPrompt(basePrompt, agentSystemPrompt string) string {
	agentSystemPrompt = strings.TrimSpace(agentSystemPrompt)
	if agentSystemPrompt == "" {
		return basePrompt
	}
	var sb strings.Builder
	sb.WriteString(basePrompt)
	sb.WriteString("\n\n## Agent-specific instructions\n\n")
	sb.WriteString(agentSystemPrompt)
	return sb.String()
}

// BuildDevPrompt builds the prompt for development tasks.
func BuildDevPrompt(task TaskContext, codeCtx *CodeContext) string {
	var sb strings.Builder

	sb.WriteString("You are a senior software engineer.\n\n")
	sb.WriteString("## Task\n\n")
	sb.WriteString(task.IssueTitle)
	sb.WriteString("\n\n")
	sb.WriteString(task.IssueBody)
	sb.WriteString("\n\n")

	if codeCtx != nil {
		sb.WriteString(FormatCodeContext(codeCtx))
	}

	sb.WriteString("\n## Instructions\n\n")
	sb.WriteString("1. Analyze the existing codebase structure\n")
	sb.WriteString("2. Understand the project's patterns and conventions\n")
	sb.WriteString("3. Write code that integrates seamlessly\n")
	sb.WriteString("4. Use the available tools to read, write, and test code\n")
	sb.WriteString("5. Run tests to verify your changes work correctly\n")
	sb.WriteString("\n\n")
	sb.WriteString(BuildSolveToolPrompt())

	return sb.String()
}

// BuildAnalyzePrompt builds the prompt for analyze tasks (read-only).
func BuildAnalyzePrompt(task TaskContext, codeCtx *CodeContext) string {
	var sb strings.Builder

	sb.WriteString("You are a senior software engineer performing a read-only analysis.\n\n")
	sb.WriteString("## Task\n\n")
	sb.WriteString(task.IssueTitle)
	sb.WriteString("\n\n")
	sb.WriteString(task.IssueBody)
	sb.WriteString("\n\n")

	if codeCtx != nil {
		sb.WriteString(FormatCodeContext(codeCtx))
	}

	sb.WriteString("\n## Instructions\n\n")
	sb.WriteString("1. Analyze the issue carefully using the available read-only tools\n")
	sb.WriteString("2. Explore the codebase to understand the relevant files and patterns\n")
	sb.WriteString("3. Identify the root cause or provide a detailed assessment\n")
	sb.WriteString("4. Reference specific file paths and line numbers in your response\n")
	sb.WriteString("5. Do NOT write any files or run arbitrary commands — this is analysis only\n")
	sb.WriteString("6. Keep your response concise but technically precise\n")

	return sb.String()
}

// BuildBugfixPrompt builds the prompt for bug fix tasks.
func BuildBugfixPrompt(task TaskContext, codeCtx *CodeContext) string {
	var sb strings.Builder

	sb.WriteString("You are a senior software engineer specializing in debugging.\n\n")
	sb.WriteString("## Bug Report\n\n")
	sb.WriteString(task.IssueTitle)
	sb.WriteString("\n\n")
	sb.WriteString(task.IssueBody)
	sb.WriteString("\n\n")

	if codeCtx != nil {
		sb.WriteString(FormatCodeContext(codeCtx))
	}

	sb.WriteString("\n## Instructions\n\n")
	sb.WriteString("1. Understand the bug report carefully\n")
	sb.WriteString("2. Locate the relevant code using rg (or search_code) and read_file\n")
	sb.WriteString("3. Identify the root cause\n")
	sb.WriteString("4. Implement a minimal fix\n")
	sb.WriteString("5. Run tests to verify the fix works\n")
	sb.WriteString("6. Ensure no regressions are introduced\n")
	sb.WriteString("\n\n")
	sb.WriteString(BuildSolveToolPrompt())

	return sb.String()
}

// ReviewPromptInput is the structured input for an independent review/checker prompt.
type ReviewPromptInput struct {
	Repo         string
	PRNumber     int
	PRTitle      string
	PRBody       string
	ChangedFiles string // preformatted bullet list; optional
	Diff         string
}

// BuildReviewPrompt builds a skeptical, independent review prompt (Maker ≠ Checker).
// It must not include coder conversation history — only the PR artifact under review.
func BuildReviewPrompt(in ReviewPromptInput) string {
	var sb strings.Builder

	sb.WriteString("You are an independent code reviewer (Checker). You did NOT author these changes.\n")
	sb.WriteString("Be skeptical. Do not rubber-stamp. Prefer concrete findings over praise.\n\n")

	sb.WriteString("## Pull Request\n\n")
	if in.Repo != "" {
		sb.WriteString(fmt.Sprintf("Repository: %s\n", in.Repo))
	}
	if in.PRNumber > 0 {
		sb.WriteString(fmt.Sprintf("PR #%d: %s\n", in.PRNumber, in.PRTitle))
	} else if in.PRTitle != "" {
		sb.WriteString(fmt.Sprintf("Title: %s\n", in.PRTitle))
	}
	if in.PRBody != "" {
		sb.WriteString("\n### Description\n\n")
		sb.WriteString(in.PRBody)
		sb.WriteString("\n")
	}
	if in.ChangedFiles != "" {
		sb.WriteString("\n## Changed Files\n\n")
		sb.WriteString(in.ChangedFiles)
		sb.WriteString("\n")
	}
	if in.Diff != "" {
		sb.WriteString("\n## Diff\n\n")
		sb.WriteString(in.Diff)
		sb.WriteString("\n")
	}

	sb.WriteString("\n## Review criteria\n\n")
	sb.WriteString("1. Correctness and edge cases\n")
	sb.WriteString("2. Security and data handling risks\n")
	sb.WriteString("3. Regressions and missing tests\n")
	sb.WriteString("4. Clarity / maintainability only when it affects risk\n")
	sb.WriteString("5. Explicitly list residual risks or \"none found\"\n\n")
	sb.WriteString("Output a structured review. If you approve, say so only after addressing the criteria.\n")

	return sb.String()
}

// CheckerPromptInput is the input for a post-coding independent checker (fresh context).
type CheckerPromptInput struct {
	IssueTitle string
	IssueBody  string
	Diff       string
	Summary    string // optional coder summary; treat as untrusted claim
}

// BuildIndependentCheckerPrompt asks for a PASS/FAIL verdict on a git diff without
// any coder agent-loop history (anti self-review at the context layer).
func BuildIndependentCheckerPrompt(in CheckerPromptInput) string {
	var sb strings.Builder

	sb.WriteString("You are an independent Checker. You did not write the patch below.\n")
	sb.WriteString("Decide whether the change is acceptable to commit/open a PR.\n\n")

	sb.WriteString("## Requirement\n\n")
	sb.WriteString(in.IssueTitle)
	sb.WriteString("\n\n")
	sb.WriteString(in.IssueBody)
	sb.WriteString("\n\n")

	if in.Summary != "" {
		sb.WriteString("## Author summary (untrusted)\n\n")
		sb.WriteString(in.Summary)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Git diff\n\n")
	if in.Diff == "" {
		sb.WriteString("(empty diff)\n\n")
	} else {
		sb.WriteString(in.Diff)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("1. Judge the diff against the requirement only\n")
	sb.WriteString("2. Fail on clear bugs, security issues, or requirement mismatch\n")
	sb.WriteString("3. Pass if the change is a reasonable, safe step toward the requirement\n")
	sb.WriteString("4. End your reply with exactly one line: VERDICT: PASS or VERDICT: FAIL\n")
	sb.WriteString("5. If FAIL, briefly explain why above the verdict line\n")

	return sb.String()
}

// ParseCheckerVerdict extracts PASS/FAIL from an independent checker response.
// Returns ok=false when no verdict line is found.
func ParseCheckerVerdict(content string) (pass bool, ok bool) {
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.Contains(upper, "VERDICT:") && strings.Contains(upper, "PASS") && !strings.Contains(upper, "FAIL"):
			return true, true
		case strings.Contains(upper, "VERDICT:") && strings.Contains(upper, "FAIL"):
			return false, true
		case upper == "PASS" || upper == "VERDICT: PASS":
			return true, true
		case upper == "FAIL" || upper == "VERDICT: FAIL":
			return false, true
		}
	}
	return false, false
}
