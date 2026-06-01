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

// Sandbox provides an isolated workspace for task execution.
type Sandbox struct {
	BaseDir     string        // Base directory for all workspaces
	TaskID      int64         // Current task ID
	WorkDir     string        // Current task workspace directory
	Timeout     time.Duration // Command timeout
	MaxOutput   int           // Max output bytes (default 1MB)
	AllowedCmds map[string]bool // Allowed commands
}

// Config contains sandbox configuration.
type Config struct {
	BaseDir   string        // Base workspace directory
	Timeout   time.Duration // Command timeout
	MaxOutput int           // Max output bytes
}

// DefaultConfig returns default sandbox configuration.
func DefaultConfig() Config {
	return Config{
		BaseDir:   "./workspace",
		Timeout:   5 * time.Minute,
		MaxOutput: 1024 * 1024, // 1MB
	}
}

// New creates a new Sandbox for the given task.
func New(cfg Config, taskID int64) *Sandbox {
	workDir := filepath.Join(cfg.BaseDir, fmt.Sprintf("task_%d", taskID))

	s := &Sandbox{
		BaseDir:   cfg.BaseDir,
		TaskID:    taskID,
		WorkDir:   workDir,
		Timeout:   cfg.Timeout,
		MaxOutput: cfg.MaxOutput,
		AllowedCmds: map[string]bool{
			// Shell
			"sh": true, "bash": true, "cmd": true,
			// Git commands
			"git": true,
			// Build tools
			"go": true, "make": true, "cargo": true,
			// Runtimes
			"python": true, "python3": true, "node": true, "npm": true, "npx": true,
			// Utilities
			"cat": true, "ls": true, "pwd": true, "echo": true, "grep": true,
			"find": true, "head": true, "tail": true, "wc": true, "sort": true,
			"mkdir": true, "cp": true, "mv": true, "touch": true,
			// Testing
			"pytest": true, "jest": true, "mocha": true,
		},
	}

	return s
}

// Setup creates the workspace directory.
func (s *Sandbox) Setup() error {
	if err := os.MkdirAll(s.WorkDir, 0755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	log.Printf("[INFO] Sandbox workspace created: %s", s.WorkDir)
	return nil
}

// Cleanup removes the workspace directory.
func (s *Sandbox) Cleanup() error {
	if err := os.RemoveAll(s.WorkDir); err != nil {
		return fmt.Errorf("remove workspace: %w", err)
	}
	log.Printf("[INFO] Sandbox workspace removed: %s", s.WorkDir)
	return nil
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
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = s.WorkDir

	// Capture output with limits
	var stdout, stderr strings.Builder
	cmd.Stdout = &limitWriter{max: s.MaxOutput, w: &stdout}
	cmd.Stderr = &limitWriter{max: s.MaxOutput, w: &stderr}

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
func (s *Sandbox) ExecuteShell(command string) *Result {
	return s.Execute("sh", "-c", command)
}

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
	fullPath := filepath.Join(s.WorkDir, path)

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
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return content, nil
}

// FileExists checks if a file exists in the workspace.
func (s *Sandbox) FileExists(path string) bool {
	fullPath := filepath.Join(s.WorkDir, path)
	_, err := os.Stat(fullPath)
	return err == nil
}

// limitWriter limits the number of bytes written.
type limitWriter struct {
	max   int
	written int
	w     *strings.Builder
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
