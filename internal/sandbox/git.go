package sandbox

import (
	"fmt"
	"log"
	"strings"
)

// Git provides Git operations within a sandbox workspace.
type Git struct {
	sandbox *Sandbox
}

// NewGit creates a new Git helper for the sandbox.
func NewGit(sandbox *Sandbox) *Git {
	return &Git{sandbox: sandbox}
}

// Clone clones a repository into the workspace.
func (g *Git) Clone(repoURL string) *Result {
	// Use shallow clone for efficiency
	result := g.sandbox.Execute("git", "clone", "--depth", "1", repoURL, ".")
	if result.Error != nil {
		return result
	}
	log.Printf("[INFO] Cloned repository: %s", repoURL)
	return result
}

// CloneBranch clones a specific branch of a repository.
func (g *Git) CloneBranch(repoURL, branch string) *Result {
	result := g.sandbox.Execute("git", "clone", "--depth", "1", "--branch", branch, repoURL, ".")
	if result.Error != nil {
		return result
	}
	log.Printf("[INFO] Cloned repository branch: %s @ %s", repoURL, branch)
	return result
}

// CreateBranch creates and switches to a new branch.
func (g *Git) CreateBranch(branch string) *Result {
	result := g.sandbox.Execute("git", "checkout", "-b", branch)
	if result.Error != nil {
		return result
	}
	log.Printf("[INFO] Created branch: %s", branch)
	return result
}

// Checkout switches to an existing branch.
func (g *Git) Checkout(branch string) *Result {
	return g.sandbox.Execute("git", "checkout", branch)
}

// Add stages all changes.
func (g *Git) Add() *Result {
	return g.sandbox.Execute("git", "add", ".")
}

// AddFile stages a specific file.
func (g *Git) AddFile(path string) *Result {
	return g.sandbox.Execute("git", "add", path)
}

// Commit creates a commit with the given message.
func (g *Git) Commit(message string) *Result {
	return g.sandbox.Execute("git", "commit", "-m", message)
}

// Push pushes the current branch to remote.
func (g *Git) Push(remote, branch string) *Result {
	result := g.sandbox.Execute("git", "push", remote, branch)
	if result.Error != nil {
		return result
	}
	log.Printf("[INFO] Pushed to %s/%s", remote, branch)
	return result
}

// Status returns the git status.
func (g *Git) Status() *Result {
	return g.sandbox.Execute("git", "status", "--short")
}

// Diff returns the diff of unstaged changes.
func (g *Git) Diff() *Result {
	return g.sandbox.Execute("git", "diff")
}

// DiffStaged returns the diff of staged changes.
func (g *Git) DiffStaged() *Result {
	return g.sandbox.Execute("git", "diff", "--cached")
}

// Log returns the git log.
func (g *Git) Log(count int) *Result {
	return g.sandbox.Execute("git", "log", fmt.Sprintf("-%d", count), "--oneline")
}

// GetCurrentBranch returns the current branch name.
func (g *Git) GetCurrentBranch() (string, error) {
	result := g.sandbox.Execute("git", "rev-parse", "--abbrev-ref", "HEAD")
	if result.Error != nil {
		return "", fmt.Errorf("get current branch: %w", result.Error)
	}
	return strings.TrimSpace(result.Stdout), nil
}

// HasChanges checks if there are any uncommitted changes.
func (g *Git) HasChanges() bool {
	status := g.Status()
	return strings.TrimSpace(status.Stdout) != ""
}

// ValidateBranchName checks if a branch name is safe (must start with "ai/").
func ValidateBranchName(branch string) error {
	if !strings.HasPrefix(branch, "ai/") {
		return fmt.Errorf("branch must start with 'ai/', got: %s", branch)
	}
	// Check for dangerous characters
	if strings.ContainsAny(branch, " ;&|`$") {
		return fmt.Errorf("branch name contains dangerous characters: %s", branch)
	}
	return nil
}

// GenerateBranchName generates a safe branch name for a task.
func GenerateBranchName(taskType string, taskID int64) string {
	// Clean task type
	cleanType := strings.ReplaceAll(taskType, "_", "-")
	return fmt.Sprintf("ai/%s/task-%d", cleanType, taskID)
}
