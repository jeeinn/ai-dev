package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSandbox(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 1 * time.Minute,
		MaxOutput:      1024,
	}

	s := New(cfg, 123)

	assert.Equal(t, int64(123), s.TaskID)
	assert.Equal(t, 1*time.Minute, s.Config.CommandTimeout)
	assert.Equal(t, 1024, s.Config.MaxOutput)
}

func TestSandboxSetupCleanup(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 1 * time.Minute,
		MaxOutput:      1024,
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

func TestSandboxTempMode(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeTemp,
		CommandTimeout: 1 * time.Minute,
		MaxOutput:      1024,
	}

	s := New(cfg, 789)

	// Setup should create temp directory
	err := s.Setup()
	require.NoError(t, err)
	assert.NotEmpty(t, s.WorkDir)

	// Verify directory exists
	_, err = os.Stat(s.WorkDir)
	assert.False(t, os.IsNotExist(err), "Temp directory should exist after Setup")

	// Cleanup
	err = s.Cleanup()
	require.NoError(t, err)

	// Verify directory removed
	_, err = os.Stat(s.WorkDir)
	assert.True(t, os.IsNotExist(err), "Temp directory should not exist after Cleanup")
}

func TestSandboxWriteReadFile(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 1 * time.Minute,
		MaxOutput:      1024,
		MaxFileSize:    1024,
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

func TestSandboxWriteFileSubdirectory(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 1 * time.Minute,
		MaxOutput:      1024,
		MaxFileSize:    1024,
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
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 1 * time.Minute,
		MaxOutput:      1024,
		MaxFileSize:    1024,
	}

	s := New(cfg, 202)
	err := s.Setup()
	require.NoError(t, err)
	defer s.Cleanup()

	// File doesn't exist yet
	assert.False(t, s.FileExists("test.txt"), "File should not exist before writing")

	// Write file
	err = s.WriteFile("test.txt", []byte("content"))
	require.NoError(t, err, "WriteFile should succeed")

	// File should exist now
	assert.True(t, s.FileExists("test.txt"), "File should exist after writing")
}

func TestSandboxIsAllowed(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 1 * time.Minute,
		MaxOutput:      1024,
	}

	s := New(cfg, 303)

	// Allowed commands
	allowed := []string{"git", "go", "python", "node", "cat", "ls", "make"}
	for _, cmd := range allowed {
		assert.True(t, s.IsAllowed(cmd), "Command '%s' should be allowed", cmd)
	}

	// Disallowed commands
	disallowed := []string{"dd", "mkfs", "shutdown", "reboot", "format"}
	for _, cmd := range disallowed {
		assert.False(t, s.IsAllowed(cmd), "Command '%s' should NOT be allowed", cmd)
	}
}

func TestSandboxExecute(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 10 * time.Second,
		MaxOutput:      1024,
	}

	s := New(cfg, 404)
	s.Setup()
	defer s.Cleanup()

	// Use shell so echo works on both Windows (cmd) and Unix (sh)
	result := s.ExecuteShell("echo hello")

	require.NoError(t, result.Error)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, strings.TrimSpace(result.Stdout), "hello")
}

func TestSandboxExecuteDisallowed(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 10 * time.Second,
		MaxOutput:      1024,
	}

	s := New(cfg, 505)
	s.Setup()
	defer s.Cleanup()

	// Try to execute disallowed command
	result := s.Execute("dd", "if=/dev/zero", "of=/tmp/x")

	assert.Error(t, result.Error, "Execute should fail for disallowed command")
	assert.Equal(t, -1, result.ExitCode)
}

func TestSandboxExecuteShell(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 10 * time.Second,
		MaxOutput:      1024,
	}

	s := New(cfg, 606)
	s.Setup()
	defer s.Cleanup()

	// Execute shell command
	result := s.ExecuteShell("echo hello world")

	require.NoError(t, result.Error)
	assert.Equal(t, "hello world", strings.TrimSpace(result.Stdout))
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, ModeFixed, cfg.Mode)
	assert.Equal(t, "./workspace", cfg.BaseDir)
	assert.Equal(t, 5*time.Minute, cfg.CommandTimeout)
	assert.Equal(t, 1024*1024, cfg.MaxOutput)
}

func TestPathTraversal(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 10 * time.Second,
		MaxOutput:      1024,
		MaxFileSize:    1024,
	}

	s := New(cfg, 707)
	s.Setup()
	defer s.Cleanup()

	// Try to write outside workspace
	err := s.WriteFile("../../etc/passwd", []byte("malicious"))
	assert.Error(t, err, "Should reject path traversal")

	// Try to read outside workspace
	_, err = s.ReadFile("../../etc/passwd")
	assert.Error(t, err, "Should reject path traversal")
}

func TestFileSizeLimit(t *testing.T) {
	cfg := SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 10 * time.Second,
		MaxOutput:      1024,
		MaxFileSize:    10, // Very small limit
	}

	s := New(cfg, 808)
	s.Setup()
	defer s.Cleanup()

	// Try to write file exceeding limit
	err := s.WriteFile("large.txt", []byte("this is too large"))
	assert.Error(t, err, "Should reject file exceeding size limit")
}
