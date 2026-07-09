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
