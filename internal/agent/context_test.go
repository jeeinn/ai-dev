package agent

import (
	"strings"
	"testing"
)

func TestBuildSolveToolPrompt(t *testing.T) {
	prompt := BuildSolveToolPrompt()
	for _, want := range []string{
		"## Tool-use requirements",
		"write_file",
		"apply_diff",
		"run_command",
		"automatically commits",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("BuildSolveToolPrompt() missing %q", want)
		}
	}
}

func TestMergeAgentSystemPrompt(t *testing.T) {
	base := "Base prompt content"

	t.Run("empty agent prompt returns base unchanged", func(t *testing.T) {
		got := MergeAgentSystemPrompt(base, "")
		if got != base {
			t.Errorf("expected base unchanged, got %q", got)
		}
	})

	t.Run("whitespace-only agent prompt returns base unchanged", func(t *testing.T) {
		got := MergeAgentSystemPrompt(base, "  \n\t  ")
		if got != base {
			t.Errorf("expected base unchanged, got %q", got)
		}
	})

	t.Run("non-empty agent prompt appended with section header", func(t *testing.T) {
		custom := "Always add unit tests."
		got := MergeAgentSystemPrompt(base, custom)
		if !strings.HasPrefix(got, base) {
			t.Error("expected base prompt at start")
		}
		if !strings.Contains(got, "## Agent-specific instructions") {
			t.Error("expected agent-specific section header")
		}
		if !strings.HasSuffix(got, custom) {
			t.Errorf("expected custom prompt at end, got %q", got)
		}
	})
}

func TestBuildDevPromptIncludesToolInstructions(t *testing.T) {
	task := TaskContext{
		IssueTitle: "Add feature X",
		IssueBody:  "Implement feature X in module Y.",
		RepoName:   "owner/repo",
		TaskType:   "dev",
	}
	prompt := BuildDevPrompt(task, nil)
	for _, want := range []string{
		"## Task",
		"Add feature X",
		"## Instructions",
		"## Tool-use requirements",
		"write_file",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("BuildDevPrompt() missing %q", want)
		}
	}
}

func TestEstimateTokensDifferentiatesCJK(t *testing.T) {
	tests := []struct {
		name  string
		input string
		min   int
		max   int
	}{
		{"empty", "", 0, 0},
		{"english 16 chars", "hello world test", 3, 5},       // 16/4 = 4
		{"chinese 6 chars", "你好世界测试", 8, 10},           // 18 bytes -> 18/2 = 9
		{"mixed", "hello你好", 3, 6},                        // 5 + 6 = 11 bytes -> 5/4 + 6/2 = 1 + 3 = 4
		{"long english", strings.Repeat("a", 100), 24, 26},  // 100/4 = 25
		{"long chinese", strings.Repeat("中", 50), 70, 80},  // 150 bytes -> 150/2 = 75
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.input)
			if got < tt.min || got > tt.max {
				t.Errorf("estimateTokens(%q) = %d, want between %d and %d", tt.input, got, tt.min, tt.max)
			}
		})
	}
}

func TestBuildBugfixPromptIncludesToolInstructions(t *testing.T) {
	task := TaskContext{
		IssueTitle: "Null pointer crash",
		IssueBody:  "App crashes on empty input.",
		RepoName:   "owner/repo",
		TaskType:   "bugfix",
	}
	prompt := BuildBugfixPrompt(task, nil)
	for _, want := range []string{
		"## Bug Report",
		"Null pointer crash",
		"## Instructions",
		"## Tool-use requirements",
		"apply_diff",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("BuildBugfixPrompt() missing %q", want)
		}
	}
}

func TestBuildAnalyzePromptIsReadOnly(t *testing.T) {
	task := TaskContext{
		IssueTitle: "Investigate slow query",
		IssueBody:  "The user list endpoint is slow.",
		RepoName:   "owner/repo",
		TaskType:   "analyze",
	}
	prompt := BuildAnalyzePrompt(task, nil)
	for _, want := range []string{
		"read-only analysis",
		"## Task",
		"Investigate slow query",
		"## Instructions",
		"read-only tools",
		"Do NOT write any files",
		"analysis only",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("BuildAnalyzePrompt() missing %q", want)
		}
	}
}
