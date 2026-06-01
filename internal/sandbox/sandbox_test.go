package sandbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSandbox(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   1 * time.Minute,
		MaxOutput: 1024,
	}

	s := New(cfg, 123)

	if s.TaskID != 123 {
		t.Errorf("Expected taskID=123, got %d", s.TaskID)
	}
	if s.Timeout != 1*time.Minute {
		t.Errorf("Expected timeout=1m, got %v", s.Timeout)
	}
	if s.MaxOutput != 1024 {
		t.Errorf("Expected maxOutput=1024, got %d", s.MaxOutput)
	}
}

func TestSandboxSetupCleanup(t *testing.T) {
	cfg := Config{
		BaseDir:   t.TempDir(),
		Timeout:   1 * time.Minute,
		MaxOutput: 1024,
	}

	s := New(cfg, 456)

	// Setup
	if err := s.Setup(); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Verify directory exists
	if _, err := os.Stat(s.WorkDir); os.IsNotExist(err) {
		t.Error("Workspace directory should exist after Setup")
	}

	// Cleanup
	if err := s.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify directory removed
	if _, err := os.Stat(s.WorkDir); !os.IsNotExist(err) {
		t.Error("Workspace directory should not exist after Cleanup")
	}
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
	if err := s.WriteFile("test.txt", content); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Read file
	read, err := s.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(read) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(read))
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
	if err := s.WriteFile("sub/dir/file.txt", content); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify file exists
	fullPath := filepath.Join(s.WorkDir, "sub", "dir", "file.txt")
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Error("Nested file should exist")
	}
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
	if s.FileExists("test.txt") {
		t.Error("File should not exist before writing")
	}

	// Write file
	s.WriteFile("test.txt", []byte("content"))

	// File should exist now
	if !s.FileExists("test.txt") {
		t.Error("File should exist after writing")
	}
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
		if !s.IsAllowed(cmd) {
			t.Errorf("Command '%s' should be allowed", cmd)
		}
	}

	// Disallowed commands
	disallowed := []string{"rm", "dd", "mkfs", "shutdown", "reboot"}
	for _, cmd := range disallowed {
		if s.IsAllowed(cmd) {
			t.Errorf("Command '%s' should NOT be allowed", cmd)
		}
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

	if result.Error != nil {
		t.Fatalf("Execute failed: %v", result.Error)
	}
	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Expected 'hello\\n', got '%s'", result.Stdout)
	}
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

	if result.Error == nil {
		t.Error("Execute should fail for disallowed command")
	}
	if result.ExitCode != -1 {
		t.Errorf("Expected exit code -1, got %d", result.ExitCode)
	}
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

	if result.Error != nil {
		t.Fatalf("ExecuteShell failed: %v", result.Error)
	}
	if result.Stdout != "hello world\n" {
		t.Errorf("Expected 'hello world\\n', got '%s'", result.Stdout)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BaseDir == "" {
		t.Error("Default BaseDir should not be empty")
	}
	if cfg.Timeout == 0 {
		t.Error("Default Timeout should not be 0")
	}
	if cfg.MaxOutput == 0 {
		t.Error("Default MaxOutput should not be 0")
	}
}
