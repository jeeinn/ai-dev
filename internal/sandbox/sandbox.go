// Package sandbox provides lightweight workspace isolation for agent tasks.
// It does NOT use Docker - instead it uses directory isolation, command whitelisting,
// and timeout control for safety.
package sandbox

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SandboxMode defines the workspace directory mode.
type SandboxMode string

const (
	// ModeTemp uses os.MkdirTemp for automatic temporary directories.
	ModeTemp SandboxMode = "temp"
	// ModeFixed uses a fixed base directory with task subdirectories.
	ModeFixed SandboxMode = "fixed"
)

// SandboxConfig contains sandbox configuration.
type SandboxConfig struct {
	Mode           SandboxMode   `yaml:"mode"`            // "temp" | "fixed"
	BaseDir        string        `yaml:"base_dir"`        // Fixed mode base directory
	CommandTimeout time.Duration `yaml:"command_timeout"` // Single command timeout
	TaskTimeout    time.Duration `yaml:"task_timeout"`    // Total task timeout
	MaxOutput      int           `yaml:"max_output"`      // Max output bytes
	MaxFileSize    int           `yaml:"max_file_size"`   // Max file size for write operations
	CleanupAfter   time.Duration `yaml:"cleanup_after"`   // Failed task retention time
}

// DefaultSandboxConfig returns default sandbox configuration.
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Mode:           ModeFixed,
		BaseDir:        "./workspace",
		CommandTimeout: 5 * time.Minute,
		TaskTimeout:    30 * time.Minute,
		MaxOutput:      1024 * 1024, // 1MB
		MaxFileSize:    1024 * 1024, // 1MB
		CleanupAfter:   24 * time.Hour,
	}
}

// Config is a backward-compatible alias for SandboxConfig.
// Deprecated: Use SandboxConfig instead.
type Config = SandboxConfig

// DefaultConfig returns default sandbox configuration (backward-compatible).
// Deprecated: Use DefaultSandboxConfig() instead.
func DefaultConfig() Config {
	return DefaultSandboxConfig()
}

// Sandbox provides an isolated workspace for task execution.
type Sandbox struct {
	Config      SandboxConfig
	TaskID      int64
	WorkDir     string
	AllowedCmds map[string]bool
	// Persistent marks session-scoped workspaces. Cleanup is a no-op; reclaim
	// via SessionLifecycle (issue closed / PR merged / idle TTL / disk limit).
	Persistent bool
}

// NewWithPath creates a Sandbox with a custom working directory (for session-level workspaces).
// The workspace is Persistent: Setup never replaces WorkDir with ModeTemp MkdirTemp,
// and Cleanup / CleanupWithDelay are no-ops.
func NewWithPath(cfg SandboxConfig, taskID int64, workDir string) *Sandbox {
	s := New(cfg, taskID)
	s.WorkDir = workDir
	s.Persistent = true
	return s
}

// New creates a new Sandbox for the given task.
func New(cfg SandboxConfig, taskID int64) *Sandbox {
	var workDir string

	switch cfg.Mode {
	case ModeTemp:
		// Temp directory will be created in Setup()
		workDir = "" // Will be set in Setup()
	default: // ModeFixed
		workDir = filepath.Join(cfg.BaseDir, fmt.Sprintf("task_%d", taskID))
	}

	s := &Sandbox{
		Config:  cfg,
		TaskID:  taskID,
		WorkDir: workDir,
		AllowedCmds: map[string]bool{
			// Shell
			"sh": true, "bash": true, "cmd": true, "powershell": true, "pwsh": true,
			// Git commands
			"git": true,
			// Build tools
			"go": true, "make": true, "cargo": true,
			// Runtimes
			"python": true, "python3": true, "node": true, "npm": true, "npx": true,
			// Utilities (Unix)
			"cat": true, "ls": true, "pwd": true, "echo": true, "grep": true, "rg": true,
			"find": true, "head": true, "tail": true, "wc": true, "sort": true,
			"mkdir": true, "cp": true, "mv": true, "touch": true, "rm": true,
			// Utilities (Windows)
			"dir": true, "type": true, "where": true, "findstr": true, "tree": true,
			// Testing
			"pytest": true, "jest": true, "mocha": true,
		},
	}

	return s
}

// Setup creates the workspace directory.
// If WorkDir is already set (ModeFixed from New, or session path from NewWithPath),
// it only ensures the directory exists — ModeTemp must never overwrite a preset path.
func (s *Sandbox) Setup() error {
	if s.WorkDir != "" {
		if err := os.MkdirAll(s.WorkDir, 0755); err != nil {
			return fmt.Errorf("create workspace: %w", err)
		}
		log.Printf("[INFO] Sandbox workspace ready: %s (persistent=%v)", s.WorkDir, s.Persistent)
		return nil
	}

	switch s.Config.Mode {
	case ModeTemp:
		tempDir, err := os.MkdirTemp("", fmt.Sprintf("agent-task-%d-*", s.TaskID))
		if err != nil {
			return fmt.Errorf("create temp directory: %w", err)
		}
		s.WorkDir = tempDir
		log.Printf("[INFO] Sandbox temp workspace created: %s", s.WorkDir)
		return nil
	default:
		// ModeFixed always presets WorkDir in New; defensive fallback.
		s.WorkDir = filepath.Join(s.Config.BaseDir, fmt.Sprintf("task_%d", s.TaskID))
		if err := os.MkdirAll(s.WorkDir, 0755); err != nil {
			return fmt.Errorf("create workspace: %w", err)
		}
		log.Printf("[INFO] Sandbox fixed workspace created: %s", s.WorkDir)
		return nil
	}
}

// Cleanup removes the workspace directory.
// Persistent (session) workspaces are never deleted here — SessionLifecycle owns reclaim.
func (s *Sandbox) Cleanup() error {
	if s.Persistent {
		log.Printf("[INFO] Sandbox skip cleanup (persistent/session): %s", s.WorkDir)
		return nil
	}
	if s.WorkDir == "" {
		return nil
	}

	if err := os.RemoveAll(s.WorkDir); err != nil {
		return fmt.Errorf("remove workspace: %w", err)
	}
	log.Printf("[INFO] Sandbox workspace removed: %s", s.WorkDir)
	return nil
}

// CleanupWithDelay removes the workspace directory after a delay (for failed tasks).
// Persistent workspaces are never scheduled for deletion.
func (s *Sandbox) CleanupWithDelay(delay time.Duration) {
	if s.Persistent || s.WorkDir == "" {
		return
	}

	go func() {
		time.Sleep(delay)
		if err := os.RemoveAll(s.WorkDir); err != nil {
			log.Printf("[WARN] Failed to cleanup workspace %s: %v", s.WorkDir, err)
		} else {
			log.Printf("[INFO] Sandbox workspace removed after delay: %s", s.WorkDir)
		}
	}()
}

// Result contains the output of a command execution.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Error    error
}

// Execute runs a command in the sandbox workspace.
func (s *Sandbox) Execute(command string, args ...string) *Result {
	start := time.Now()

	// Validate command is allowed
	if !s.IsAllowed(command) {
		return &Result{
			ExitCode: -1,
			Duration: time.Since(start),
			Error:    fmt.Errorf("command not allowed: %s", command),
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.Config.CommandTimeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = s.WorkDir

	// Capture output with limits
	var stdout, stderr strings.Builder
	cmd.Stdout = &limitWriter{max: s.Config.MaxOutput, w: &stdout}
	cmd.Stderr = &limitWriter{max: s.Config.MaxOutput, w: &stderr}

	// Set environment
	cmd.Env = append(os.Environ(), "SANDBOX=1")

	// Execute
	err := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		result.Error = err
	}

	log.Printf("[DEBUG] Sandbox exec: %s %v (exit=%d, duration=%v)",
		command, args, result.ExitCode, duration)

	return result
}

// ExecuteShell runs a shell command in the sandbox workspace.
// Implementation is platform-specific (see shell_unix.go / shell_windows.go).

// IsAllowed checks if a command is in the whitelist.
func (s *Sandbox) IsAllowed(command string) bool {
	// Extract base command name
	base := filepath.Base(command)
	// Remove .exe extension on Windows
	base = strings.TrimSuffix(base, ".exe")
	return s.AllowedCmds[base]
}

// WriteFile writes content to a file in the workspace.
func (s *Sandbox) WriteFile(path string, content []byte) error {
	// Check file size limit
	if len(content) > s.Config.MaxFileSize {
		return fmt.Errorf("file size %d exceeds limit %d", len(content), s.Config.MaxFileSize)
	}

	fullPath := filepath.Join(s.WorkDir, path)

	// Validate path is within workspace (prevent path traversal)
	if err := s.validatePath(fullPath); err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// ReadFile reads a file from the workspace.
func (s *Sandbox) ReadFile(path string) ([]byte, error) {
	fullPath := filepath.Join(s.WorkDir, path)

	// Validate path is within workspace
	if err := s.validatePath(fullPath); err != nil {
		return nil, err
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return content, nil
}

// ReadFileWithLines reads a file and returns specific line range.
func (s *Sandbox) ReadFileWithLines(path string, startLine, endLine int) (string, error) {
	fullPath := filepath.Join(s.WorkDir, path)

	// Validate path is within workspace
	if err := s.validatePath(fullPath); err != nil {
		return "", err
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	// Adjust to 0-based index
	start := startLine - 1
	end := endLine

	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		return "", nil
	}

	return strings.Join(lines[start:end], "\n"), nil
}

// FileExists checks if a file exists in the workspace.
func (s *Sandbox) FileExists(path string) bool {
	fullPath := filepath.Join(s.WorkDir, path)
	_, err := os.Stat(fullPath)
	return err == nil
}

// validatePath checks if a path is within the workspace directory.
func (s *Sandbox) validatePath(path string) error {
	// Clean the path
	cleanPath := filepath.Clean(path)
	cleanWorkDir := filepath.Clean(s.WorkDir)

	// Convert to lowercase for case-insensitive comparison on Windows
	cleanPathLower := strings.ToLower(cleanPath)
	cleanWorkDirLower := strings.ToLower(cleanWorkDir)

	// Ensure workspace directory ends with separator for prefix check
	if !strings.HasSuffix(cleanWorkDirLower, string(filepath.Separator)) {
		cleanWorkDirLower += string(filepath.Separator)
	}

	// Check if path is within workspace
	if !strings.HasPrefix(cleanPathLower, cleanWorkDirLower) && cleanPathLower != strings.TrimSuffix(cleanWorkDirLower, string(filepath.Separator)) {
		return fmt.Errorf("path %s is outside workspace %s", path, s.WorkDir)
	}

	return nil
}

// limitWriter limits the number of bytes written.
type limitWriter struct {
	max     int
	written int
	w       *strings.Builder
}

func (l *limitWriter) Write(p []byte) (n int, err error) {
	remaining := l.max - l.written
	if remaining <= 0 {
		return len(p), nil // Silently drop
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err = l.w.Write(p)
	l.written += n
	return len(p), nil // Return full length to avoid short write errors
}
