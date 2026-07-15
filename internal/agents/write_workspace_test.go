package agents

import (
	"context"
	"testing"
	"time"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/sandbox"
	"gitea-agent-gateway/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupLocalGitRepo builds a minimal local git workspace (no network) suitable
// for exercising finalizeWriteChanges without a Gitea server.
func setupLocalGitRepo(t *testing.T) (*sandbox.Sandbox, *sandbox.Git, *sandbox.AuditLogger) {
	t.Helper()
	cfg := sandbox.SandboxConfig{
		Mode:           sandbox.ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 30 * time.Second,
		MaxOutput:      4096,
		MaxFileSize:    4096,
	}
	s := sandbox.New(cfg, 9001)
	require.NoError(t, s.Setup())
	t.Cleanup(func() { s.Cleanup() })

	git := sandbox.NewGit(s)
	s.Execute("git", "init")
	s.Execute("git", "config", "user.email", "test@test.com")
	s.Execute("git", "config", "user.name", "Test")
	require.NoError(t, s.WriteFile("README.md", []byte("hello")))
	git.Add()
	require.NoError(t, git.Commit("initial").Error)

	audit := testAuditLogger(t)
	return s, git, audit
}

// TestFinalizeWriteChangesNoChangesReturnsComment verifies the no-changes early
// return: when the workspace has no uncommitted changes, finalizeWriteChanges
// returns a comment-style Result without touching git or PR.
func TestFinalizeWriteChangesNoChangesReturnsComment(t *testing.T) {
	s, git, audit := setupLocalGitRepo(t)

	wwc := &WriteWorkspaceContext{
		Sandbox:    s,
		Git:        git,
		Audit:      audit,
		BranchName: "main",
	}
	task := &store.Task{ID: 9001, Repo: "owner/repo"}
	agent := &store.Agent{Provider: "mock", Model: "m"}

	factory := NewRunnerFactory(nil, nil, nil, config.DefaultAgentDefaults(), config.DefaultAgentLoopConfig(), nil, nil, nil, sandbox.DefaultConfig())

	result, err := finalizeWriteChanges(context.Background(), wwc, task, agent, factory, nil, "dev", "nothing changed")
	require.NoError(t, err)
	assert.Equal(t, "comment", result.Action)
	assert.Equal(t, "nothing changed", result.Content)
	assert.Equal(t, 0, result.PRID)
}
