package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/sandbox"
)

// TestSandboxFullWorkflow tests the complete sandbox workflow:
// init → write → commit → branch → modify → commit → log
// This is a true integration test that validates the full Git lifecycle.
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

// TestSandboxWriteReadNestedFiles tests writing and reading multiple nested files.
// This complements the unit test TestSandboxWriteReadFile which tests single files.
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
		"src/main.go":        "package main",
		"src/utils.go":       "package main",
		"docs/README.md":     "# Test",
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
