package workflow

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/store"
)

func newTestLifecycle(t *testing.T) (*store.DB, *SessionLifecycle) {
	t.Helper()
	db := newTestDB(t)
	wfMgr := NewWorkflowManager(db)
	sessionSvc := NewSessionService(db, t.TempDir())
	cfg := config.DefaultSessionConfig()
	lifecycle := NewSessionLifecycle(db, wfMgr, sessionSvc, &cfg, t.TempDir())
	return db, lifecycle
}

func TestOnIssueClosed(t *testing.T) {
	db, lifecycle := newTestLifecycle(t)

	// Create workflow context
	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 1)
	require.NoError(t, err)
	ctx.Stage = store.StageDeveloping
	require.NoError(t, db.UpdateWorkflowContext(ctx))

	// Create a session
	agent := &store.Agent{Name: "coder", GiteaUsername: "coder", GiteaToken: "t", Role: store.RoleCoder, Status: "active"}
	require.NoError(t, db.CreateAgent(agent))
	sess := &store.AgentSession{
		ID: "sess-1", Repo: "owner/repo", IssueID: 1, AgentID: agent.ID,
		Role: store.RoleCoder, Status: store.SessionActive,
		LastActiveAt: time.Now(), CreatedAt: time.Now(),
	}
	require.NoError(t, db.CreateSession(sess))

	// Close issue
	err = lifecycle.OnIssueClosed("owner/repo", 1)
	require.NoError(t, err)

	// Verify context → done
	got, err := db.GetWorkflowContext("owner/repo", 1)
	require.NoError(t, err)
	assert.Equal(t, store.StageDone, got.Stage)

	// Verify session archived
	s, err := db.GetSession("sess-1")
	require.NoError(t, err)
	assert.Equal(t, store.SessionArchived, s.Status)
}

func TestOnPRClosedMerged(t *testing.T) {
	db, lifecycle := newTestLifecycle(t)

	// Create workflow context with PR
	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 5)
	require.NoError(t, err)
	ctx.Stage = store.StageReviewing
	ctx.PRID = 10
	require.NoError(t, db.UpdateWorkflowContext(ctx))

	// Close PR (merged)
	err = lifecycle.OnPRClosed("owner/repo", 10, 5, true)
	require.NoError(t, err)

	// Verify context → done
	got, err := db.GetWorkflowContext("owner/repo", 5)
	require.NoError(t, err)
	assert.Equal(t, store.StageDone, got.Stage)
}

func TestOnPRClosedNotMerged(t *testing.T) {
	db, lifecycle := newTestLifecycle(t)

	// Create workflow context
	ctx, err := db.GetOrCreateWorkflowContext("owner/repo", 5)
	require.NoError(t, err)
	ctx.Stage = store.StageReviewing
	require.NoError(t, db.UpdateWorkflowContext(ctx))

	// Close PR (not merged)
	err = lifecycle.OnPRClosed("owner/repo", 10, 5, false)
	require.NoError(t, err)

	// Context should NOT be done (retained for pr_closed_retention)
	got, err := db.GetWorkflowContext("owner/repo", 5)
	require.NoError(t, err)
	assert.Equal(t, store.StageReviewing, got.Stage) // Unchanged
}

func TestCleanupIdleSessions(t *testing.T) {
	db, lifecycle := newTestLifecycle(t)

	agent := &store.Agent{Name: "a", GiteaUsername: "u", GiteaToken: "t", Role: store.RoleCoder, Status: "active"}
	require.NoError(t, db.CreateAgent(agent))

	// Create an old idle session
	sess := &store.AgentSession{
		ID: "old-sess", Repo: "owner/repo", IssueID: 1, AgentID: agent.ID,
		Role: store.RoleCoder, Status: store.SessionIdle,
		LastActiveAt: time.Now().Add(-200 * time.Hour), // Older than 7-day default
		CreatedAt:    time.Now(),
	}
	require.NoError(t, db.CreateSession(sess))

	// Run cleanup
	archived, err := lifecycle.CleanupIdleSessions()
	require.NoError(t, err)
	assert.Equal(t, 1, archived)

	// Verify session archived
	s, err := db.GetSession("old-sess")
	require.NoError(t, err)
	assert.Equal(t, store.SessionArchived, s.Status)
}

func TestCleanupIdleSessionsSkipsRecent(t *testing.T) {
	db, lifecycle := newTestLifecycle(t)

	agent := &store.Agent{Name: "a", GiteaUsername: "u", GiteaToken: "t", Role: store.RoleCoder, Status: "active"}
	require.NoError(t, db.CreateAgent(agent))

	// Create a recent idle session
	sess := &store.AgentSession{
		ID: "recent-sess", Repo: "owner/repo", IssueID: 1, AgentID: agent.ID,
		Role: store.RoleCoder, Status: store.SessionIdle,
		LastActiveAt: time.Now().Add(-1 * time.Hour), // Recent
		CreatedAt:    time.Now(),
	}
	require.NoError(t, db.CreateSession(sess))

	// Run cleanup
	archived, err := lifecycle.CleanupIdleSessions()
	require.NoError(t, err)
	assert.Equal(t, 0, archived) // Should not archive recent session
}

func TestCleanupArchivedWorkspaces(t *testing.T) {
	// Create a temp dir for workspaces
	tmpDir := t.TempDir()
	sessDir := tmpDir + "/sessions/old-sess/repo"
	require.NoError(t, os.MkdirAll(sessDir, 0755))
	require.NoError(t, os.WriteFile(sessDir+"/test.txt", []byte("hello"), 0644))

	db := newTestDB(t)
	wfMgr := NewWorkflowManager(db)
	sessionSvc := NewSessionService(db, tmpDir)
	cfg := config.DefaultSessionConfig()
	lifecycle := NewSessionLifecycle(db, wfMgr, sessionSvc, &cfg, tmpDir)

	agent := &store.Agent{Name: "a", GiteaUsername: "u", GiteaToken: "t", Role: store.RoleCoder, Status: "active"}
	require.NoError(t, db.CreateAgent(agent))

	// Create an old archived session with workspace
	sess := &store.AgentSession{
		ID: "old-sess", Repo: "owner/repo", IssueID: 1, AgentID: agent.ID,
		Role: store.RoleCoder, Status: store.SessionArchived,
		WorkspacePath: sessDir,
		LastActiveAt:  time.Now().Add(-48 * time.Hour), // Older than 24h retention
		CreatedAt:     time.Now(),
	}
	require.NoError(t, db.CreateSession(sess))

	// Run cleanup
	cleaned, err := lifecycle.CleanupArchivedWorkspaces()
	require.NoError(t, err)
	assert.Equal(t, 1, cleaned)

	// Verify workspace deleted
	_, err = os.Stat(sessDir)
	assert.True(t, os.IsNotExist(err))

	// Verify session workspace path cleared
	s, err := db.GetSession("old-sess")
	require.NoError(t, err)
	assert.Empty(t, s.WorkspacePath)
}

func TestGetRepoWorkspaceSize(t *testing.T) {
	tmpDir := t.TempDir()
	sessDir := tmpDir + "/sessions/s1/repo"
	require.NoError(t, os.MkdirAll(sessDir, 0755))
	require.NoError(t, os.WriteFile(sessDir+"/file.txt", []byte("12345"), 0644))

	db := newTestDB(t)
	lifecycle := NewSessionLifecycle(db, nil, nil, nil, tmpDir)

	size, err := lifecycle.GetRepoWorkspaceSize("owner/repo")
	require.NoError(t, err)
	assert.Equal(t, int64(5), size)
}
