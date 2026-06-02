package sandbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSandbox(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   1 * time.Minute,
		MaxOutput: 1024,
	}

	s := New(cfg, 123)

	assert.Equal(t, int64(123), s.TaskID)
	assert.Equal(t, 1*time.Minute, s.Timeout)
	assert.Equal(t, 1024, s.MaxOutput)
}

func TestSandboxSetupCleanup(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   1 * time.Minute,
		MaxOutput: 1024,
	}

	s := New(cfg, 456)

	// Setup
	err := s.Setup()
	require.NoError(t, err)

	// Verify directory exists
	_, err = os.Stat(s.WorkDir)
	assert.False(t, os.IsNotExist(err), "Workspace directory should exist after Setup")

	// Cleanup
	err = s.Cleanup()
	require.NoError(t, err)

	// Verify directory removed
	_, err = os.Stat(s.WorkDir)
	assert.True(t, os.IsNotExist(err), "Workspace directory should not exist after Cleanup")
}

func TestSandboxWriteReadFile(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   1 * time.Minute,
		MaxOutput: 1024,
	}

	s := New(cfg, 789)
	s.Setup()
	defer s.Cleanup()

	// Write file
	content := []byte("hello world")
	err := s.WriteFile("test.txt", content)
	require.NoError(t, err)

	// Read file
	read, err := s.ReadFile("test.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(read))
}

// TestSandboxWriteReadNestedFiles tests writing and reading multiple nested files.
// This complements TestSandboxWriteReadFile which tests single files.
func TestSandboxWriteReadNestedFiles(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   10 * time.Second,
		MaxOutput: 1024,
	}

	s := New(cfg, 555)
	s.Setup()
	defer s.Cleanup()

	files := map[string]string{
		"src/main.go":        "package main",
		"src/utils.go":       "package main",
		"docs/README.md":     "# Test",
		"tests/main_test.go": "package main",
	}

	for path, content := range files {
		err := s.WriteFile(path, []byte(content))
		require.NoError(t, err)
	}

	for path, expected := range files {
		assert.True(t, s.FileExists(path), "File %s should exist", path)

		content, err := s.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, expected, string(content))
	}
}

func TestSandboxWriteFileSubdirectory(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   1 * time.Minute,
		MaxOutput: 1024,
	}

	s := New(cfg, 101)
	s.Setup()
	defer s.Cleanup()

	// Write file in subdirectory
	content := []byte("nested content")
	err := s.WriteFile("sub/dir/file.txt", content)
	require.NoError(t, err)

	// Verify file exists
	fullPath := filepath.Join(s.WorkDir, "sub", "dir", "file.txt")
	_, err = os.Stat(fullPath)
	assert.False(t, os.IsNotExist(err), "Nested file should exist")
}

func TestSandboxFileExists(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   1 * time.Minute,
		MaxOutput: 1024,
	}

	s := New(cfg, 202)
	s.Setup()
	defer s.Cleanup()

	// File doesn't exist yet
	assert.False(t, s.FileExists("test.txt"), "File should not exist before writing")

	// Write file
	s.WriteFile("test.txt", []byte("content"))

	// File should exist now
	assert.True(t, s.FileExists("test.txt"), "File should exist after writing")
}

func TestSandboxIsAllowed(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   1 * time.Minute,
		MaxOutput: 1024,
	}

	s := New(cfg, 303)

	// Allowed commands
	allowed := []string{"git", "go", "python", "node", "cat", "ls", "make"}
	for _, cmd := range allowed {
		assert.True(t, s.IsAllowed(cmd), "Command '%s' should be allowed", cmd)
	}

	// Disallowed commands
	disallowed := []string{"rm", "dd", "mkfs", "shutdown", "reboot"}
	for _, cmd := range disallowed {
		assert.False(t, s.IsAllowed(cmd), "Command '%s' should NOT be allowed", cmd)
	}
}

func TestSandboxExecute(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   10 * time.Second,
		MaxOutput: 1024,
	}

	s := New(cfg, 404)
	s.Setup()
	defer s.Cleanup()

	// Execute echo command
	result := s.Execute("echo", "hello")

	require.NoError(t, result.Error)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
}

func TestSandboxExecuteDisallowed(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   10 * time.Second,
		MaxOutput: 1024,
	}

	s := New(cfg, 505)
	s.Setup()
	defer s.Cleanup()

	// Try to execute disallowed command
	result := s.Execute("rm", "-rf", "/")

	assert.Error(t, result.Error, "Execute should fail for disallowed command")
	assert.Equal(t, -1, result.ExitCode)
}

func TestSandboxExecuteShell(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   10 * time.Second,
		MaxOutput: 1024,
	}

	s := New(cfg, 606)
	s.Setup()
	defer s.Cleanup()

	// Execute shell command
	result := s.ExecuteShell("echo hello world")

	require.NoError(t, result.Error)
	assert.Equal(t, "hello world\n", result.Stdout)
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "./workspace", cfg.BaseDir)
	assert.Equal(t, 5*time.Minute, cfg.Timeout)
	assert.Equal(t, 1024*1024, cfg.MaxOutput)
}
