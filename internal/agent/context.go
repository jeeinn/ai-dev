package agent

import (
	"fmt"
	"strings"

	"gitea-agent-gateway/internal/sandbox"
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

// estimateTokens estimates the token count for a string.
// Rough approximation: 1 token ≈ 4 characters for English, 2 characters for Chinese.
func estimateTokens(s string) int {
	// Simple approximation
	return len(s) / 4
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
	sb.WriteString("2. Locate the relevant code using search_code and read_file\n")
	sb.WriteString("3. Identify the root cause\n")
	sb.WriteString("4. Implement a minimal fix\n")
	sb.WriteString("5. Run tests to verify the fix works\n")
	sb.WriteString("6. Ensure no regressions are introduced\n")

	return sb.String()
}
