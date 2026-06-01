package sandbox

import (
	"testing"
	"time"
)

func TestGitOperations(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   30 * time.Second,
		MaxOutput: 1024,
	}

	s := New(cfg, 1001)
	s.Setup()
	defer s.Cleanup()

	git := NewGit(s)

	// Initialize a git repo
	result := s.Execute("git", "init")
	if result.Error != nil {
		t.Fatalf("git init failed: %v", result.Error)
	}

	// Configure git user
	s.Execute("git", "config", "user.email", "test@test.com")
	s.Execute("git", "config", "user.name", "Test")

	// Create a file
	s.WriteFile("test.txt", []byte("initial content"))

	// Stage
	result = git.Add()
	if result.Error != nil {
		t.Fatalf("git add failed: %v", result.Error)
	}

	// Commit
	result = git.Commit("initial commit")
	if result.Error != nil {
		t.Fatalf("git commit failed: %v", result.Error)
	}

	// Check status
	result = git.Status()
	if result.Error != nil {
		t.Fatalf("git status failed: %v", result.Error)
	}

	// Should be clean
	if result.Stdout != "" {
		t.Errorf("Expected clean status, got: %s", result.Stdout)
	}
}

func TestGitCreateBranch(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   30 * time.Second,
		MaxOutput: 1024,
	}

	s := New(cfg, 1002)
	s.Setup()
	defer s.Cleanup()

	git := NewGit(s)

	// Initialize a git repo
	s.Execute("git", "init")
	s.Execute("git", "config", "user.email", "test@test.com")
	s.Execute("git", "config", "user.name", "Test")
	s.WriteFile("test.txt", []byte("content"))
	git.Add()
	git.Commit("initial")

	// Create branch
	result := git.CreateBranch("ai/test/task-1002")
	if result.Error != nil {
		t.Fatalf("CreateBranch failed: %v", result.Error)
	}

	// Get current branch
	branch, err := git.GetCurrentBranch()
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}

	if branch != "ai/test/task-1002" {
		t.Errorf("Expected branch 'ai/test/task-1002', got '%s'", branch)
	}
}

func TestGitHasChanges(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   30 * time.Second,
		MaxOutput: 1024,
	}

	s := New(cfg, 1003)
	s.Setup()
	defer s.Cleanup()

	git := NewGit(s)

	// Initialize a git repo
	s.Execute("git", "init")
	s.Execute("git", "config", "user.email", "test@test.com")
	s.Execute("git", "config", "user.name", "Test")
	s.WriteFile("test.txt", []byte("content"))
	git.Add()
	git.Commit("initial")

	// Should not have changes
	if git.HasChanges() {
		t.Error("Should not have changes after commit")
	}

	// Add a new file
	s.WriteFile("new.txt", []byte("new content"))

	// Should have changes now
	if !git.HasChanges() {
		t.Error("Should have changes after adding new file")
	}
}

func TestValidateBranchName(t *testing.T) {
	// Valid branch names
	valid := []string{
		"ai/dev/task-1",
		"ai/bugfix/task-123",
		"ai/review/pr-456",
	}

	for _, branch := range valid {
		if err := ValidateBranchName(branch); err != nil {
			t.Errorf("Branch '%s' should be valid: %v", branch, err)
		}
	}

	// Invalid branch names
	invalid := []string{
		"main",
		"feature/test",
		"ai/test;rm -rf /",
		"dev/task-1",
	}

	for _, branch := range invalid {
		if err := ValidateBranchName(branch); err == nil {
			t.Errorf("Branch '%s' should be invalid", branch)
		}
	}
}

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		taskType string
		taskID   int64
		expected string
	}{
		{"dev", 123, "ai/dev/task-123"},
		{"bugfix", 456, "ai/bugfix/task-456"},
		{"solve_issue", 789, "ai/solve-issue/task-789"},
	}

	for _, tt := range tests {
		result := GenerateBranchName(tt.taskType, tt.taskID)
		if result != tt.expected {
			t.Errorf("GenerateBranchName(%s, %d) = %s, want %s",
				tt.taskType, tt.taskID, result, tt.expected)
		}
	}
}
