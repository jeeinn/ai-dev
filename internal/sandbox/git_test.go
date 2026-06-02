package sandbox

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitOperations(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 30 * time.Second,
		MaxOutput:      1024,
		MaxFileSize:    1024,
	}

	s := New(cfg, 1001)
	err := s.Setup()
	require.NoError(t, err)
	defer s.Cleanup()

	git := NewGit(s)

	// Initialize a git repo
	result := s.Execute("git", "init")
	require.NoError(t, result.Error)

	// Configure git user
	s.Execute("git", "config", "user.email", "test@test.com")
	s.Execute("git", "config", "user.name", "Test")

	// Create a file
	err = s.WriteFile("test.txt", []byte("initial content"))
	require.NoError(t, err)

	// Stage
	result = git.Add()
	require.NoError(t, result.Error)

	// Commit
	result = git.Commit("initial commit")
	if result.Error != nil {
		t.Logf("Commit stderr: %s", result.Stderr)
		t.Logf("Commit stdout: %s", result.Stdout)
		require.NoError(t, result.Error)
	}

	// Check status - should be clean
	result = git.Status()
	require.NoError(t, result.Error)
	assert.Empty(t, result.Stdout)
}

func TestGitCreateBranch(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 30 * time.Second,
		MaxOutput:      1024,
		MaxFileSize:    1024,
	}

	s := New(cfg, 1002)
	err := s.Setup()
	require.NoError(t, err)
	defer s.Cleanup()

	git := NewGit(s)

	// Initialize a git repo
	s.Execute("git", "init")
	s.Execute("git", "config", "user.email", "test@test.com")
	s.Execute("git", "config", "user.name", "Test")
	err = s.WriteFile("test.txt", []byte("content"))
	require.NoError(t, err)
	git.Add()
	git.Commit("initial")

	// Create branch
	result := git.CreateBranch("ai/test/task-1002")
	require.NoError(t, result.Error)

	// Get current branch
	branch, err := git.GetCurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "ai/test/task-1002", branch)
}

func TestGitHasChanges(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 30 * time.Second,
		MaxOutput:      1024,
		MaxFileSize:    1024,
	}

	s := New(cfg, 1003)
	err := s.Setup()
	require.NoError(t, err)
	defer s.Cleanup()

	git := NewGit(s)

	// Initialize a git repo
	s.Execute("git", "init")
	s.Execute("git", "config", "user.email", "test@test.com")
	s.Execute("git", "config", "user.name", "Test")
	err = s.WriteFile("test.txt", []byte("content"))
	require.NoError(t, err)
	git.Add()
	git.Commit("initial")

	// Should not have changes
	assert.False(t, git.HasChanges(), "Should not have changes after commit")

	// Add a new file
	err = s.WriteFile("new.txt", []byte("new content"))
	require.NoError(t, err)

	// Should have changes now
	assert.True(t, git.HasChanges(), "Should have changes after adding new file")
}

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
	}{
		{"valid dev", "ai/dev/task-1", false},
		{"valid bugfix", "ai/bugfix/task-123", false},
		{"valid review", "ai/review/pr-456", false},
		{"invalid no prefix", "main", true},
		{"invalid feature", "feature/test", true},
		{"invalid with semicolon", "ai/test;rm -rf /", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBranchName(tt.branch)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
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
		t.Run(tt.expected, func(t *testing.T) {
			result := GenerateBranchName(tt.taskType, tt.taskID)
			assert.Equal(t, tt.expected, result)
		})
	}
}
