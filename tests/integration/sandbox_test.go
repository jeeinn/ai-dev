package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/sandbox"
)

func TestSandboxFullWorkflow(t *testing.T) {
	cfg := sandbox.Config{
		BaseDir:   t.TempDir(),
		Timeout:   30 * time.Second,
		MaxOutput: 1024 * 1024,
	}

	// Create sandbox
	sb := sandbox.New(cfg, 999)
	err := sb.Setup()
	require.NoError(t, err)
	defer sb.Cleanup()

	// Initialize git repo
	result := sb.Execute("git", "init")
	require.NoError(t, result.Error)
	assert.Equal(t, 0, result.ExitCode)

	// Configure git
	sb.Execute("git", "config", "user.email", "test@test.com")
	sb.Execute("git", "config", "user.name", "Test")

	// Create a file
	err = sb.WriteFile("main.go", []byte(`package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`))
	require.NoError(t, err)

	// Verify file exists
	assert.True(t, sb.FileExists("main.go"))

	// Read file
	content, err := sb.ReadFile("main.go")
	require.NoError(t, err)
	assert.Contains(t, string(content), "Hello, World!")

	// Stage and commit
	git := sandbox.NewGit(sb)
	result = git.Add()
	require.NoError(t, result.Error)

	result = git.Commit("initial commit")
	require.NoError(t, result.Error)

	// Check status (should be clean)
	result = git.Status()
	require.NoError(t, result.Error)
	assert.Empty(t, result.Stdout)

	// Create branch
	result = git.CreateBranch("ai/dev/task-999")
	require.NoError(t, result.Error)

	// Get current branch
	branch, err := git.GetCurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "ai/dev/task-999", branch)

	// Make changes
	err = sb.WriteFile("helper.go", []byte(`package main

func helper() string {
	return "helper"
}
`))
	require.NoError(t, err)

	// Verify changes detected
	assert.True(t, git.HasChanges())

	// Stage and commit changes
	git.Add()
	result = git.Commit("add helper")
	require.NoError(t, result.Error)

	// Verify log
	result = git.Log(2)
	require.NoError(t, result.Error)
	assert.Contains(t, result.Stdout, "add helper")
	assert.Contains(t, result.Stdout, "initial commit")
}

func TestSandboxCommandWhitelist(t *testing.T) {
	cfg := sandbox.Config{
		BaseDir:   t.TempDir(),
		Timeout:   10 * time.Second,
		MaxOutput: 1024,
	}

	sb := sandbox.New(cfg, 888)
	sb.Setup()
	defer sb.Cleanup()

	// Allowed commands should work
	tests := []struct {
		cmd  string
		args []string
	}{
		{"echo", []string{"hello"}},
		{"git", []string{"--version"}},
		{"go", []string{"version"}},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := sb.Execute(tt.cmd, tt.args...)
			// Some commands may fail but should not be blocked
			assert.NotEqual(t, -1, result.ExitCode, "Command %s should be allowed", tt.cmd)
		})
	}

	// Disallowed commands should be blocked
	disallowed := []struct {
		cmd  string
		args []string
	}{
		{"rm", []string{"-rf", "/"}},
		{"dd", []string{"if=/dev/zero", "of=/dev/sda"}},
		{"mkfs", []string{"/dev/sda1"}},
	}

	for _, tt := range disallowed {
		t.Run(tt.cmd, func(t *testing.T) {
			result := sb.Execute(tt.cmd, tt.args...)
			assert.Equal(t, -1, result.ExitCode)
			assert.Error(t, result.Error)
		})
	}
}

func TestSandboxTimeout(t *testing.T) {
	cfg := sandbox.Config{
		BaseDir:   t.TempDir(),
		Timeout:   1 * time.Second, // Very short timeout
		MaxOutput: 1024,
	}

	sb := sandbox.New(cfg, 777)
	sb.Setup()
	defer sb.Cleanup()

	// Command that takes too long
	result := sb.Execute("sleep", "10")

	// Should timeout
	assert.Error(t, result.Error)
	assert.NotEqual(t, 0, result.ExitCode)
}

func TestSandboxOutputLimit(t *testing.T) {
	cfg := sandbox.Config{
		BaseDir:   t.TempDir(),
		Timeout:   10 * time.Second,
		MaxOutput: 100, // Very small limit
	}

	sb := sandbox.New(cfg, 666)
	sb.Setup()
	defer sb.Cleanup()

	// Generate large output
	result := sb.Execute("python", "-c", "print('x' * 1000)")

	// Output should be truncated
	assert.LessOrEqual(t, len(result.Stdout), 100+20) // +20 for truncation message
}

func TestSandboxWriteReadNestedFiles(t *testing.T) {
	cfg := sandbox.Config{
		BaseDir:   t.TempDir(),
		Timeout:   10 * time.Second,
		MaxOutput: 1024,
	}

	sb := sandbox.New(cfg, 555)
	sb.Setup()
	defer sb.Cleanup()

	// Write nested files
	files := map[string]string{
		"src/main.go":     "package main",
		"src/utils.go":    "package main",
		"docs/README.md":  "# Test",
		"tests/main_test.go": "package main",
	}

	for path, content := range files {
		err := sb.WriteFile(path, []byte(content))
		require.NoError(t, err)
	}

	// Verify all files exist and can be read
	for path, expected := range files {
		assert.True(t, sb.FileExists(path), "File %s should exist", path)

		content, err := sb.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, expected, string(content))
	}
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
			err := sandbox.ValidateBranchName(tt.branch)
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
		{"dev", 1, "ai/dev/task-1"},
		{"bugfix", 42, "ai/bugfix/task-42"},
		{"solve_issue", 100, "ai/solve-issue/task-100"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := sandbox.GenerateBranchName(tt.taskType, tt.taskID)
			assert.Equal(t, tt.expected, result)
		})
	}
}
