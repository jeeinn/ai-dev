package workflow

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/store"
)

// SessionLifecycle manages session archival and workspace cleanup.
type SessionLifecycle struct {
	db         *store.DB
	wfMgr      *WorkflowManager
	sessionSvc *SessionService
	cfg        *config.SessionConfig
	baseDir    string
}

// NewSessionLifecycle creates a new SessionLifecycle manager.
func NewSessionLifecycle(db *store.DB, wfMgr *WorkflowManager, sessionSvc *SessionService, cfg *config.SessionConfig, baseDir string) *SessionLifecycle {
	if cfg == nil {
		defaultCfg := config.DefaultSessionConfig()
		cfg = &defaultCfg
	}
	return &SessionLifecycle{
		db:         db,
		wfMgr:      wfMgr,
		sessionSvc: sessionSvc,
		cfg:        cfg,
		baseDir:    baseDir,
	}
}

// OnIssueClosed handles issues.closed event — archive sessions, set context to done.
func (sl *SessionLifecycle) OnIssueClosed(repo string, issueID int) error {
	// Update workflow context to done
	ctx, err := sl.db.GetWorkflowContext(repo, issueID)
	if err != nil {
		// No context exists — nothing to do
		log.Printf("[DEBUG] No workflow context for %s#%d on close", repo, issueID)
		return nil
	}

	ctx.Stage = store.StageDone
	if err := sl.db.UpdateWorkflowContext(ctx); err != nil {
		return fmt.Errorf("update context to done: %w", err)
	}

	// Archive all sessions for this issue
	if err := sl.sessionSvc.ArchiveByIssue(repo, issueID); err != nil {
		log.Printf("[WARN] Failed to archive sessions for %s#%d: %v", repo, issueID, err)
	}

	log.Printf("[INFO] Issue %s#%d closed → context=done, sessions archived", repo, issueID)
	return nil
}

// OnPRClosed handles pull_request.closed event.
func (sl *SessionLifecycle) OnPRClosed(repo string, prID int, issueID int, merged bool) error {
	if merged {
		// PR merged → archive sessions, context=done, schedule workspace deletion
		if issueID > 0 {
			if err := sl.OnIssueClosed(repo, issueID); err != nil {
				return err
			}
		}
		// Schedule workspace deletion after retention period
		sl.scheduleWorkspaceDeletion(repo, issueID, sl.cfg.WorkspaceRetention)
		log.Printf("[INFO] PR %s#%d merged → sessions archived, workspace deletion scheduled", repo, prID)
	} else {
		// PR closed without merge → retain for pr_closed_retention
		log.Printf("[INFO] PR %s#%d closed (not merged) → sessions retained for %s", repo, prID, sl.cfg.PRClosedRetention)
	}
	return nil
}

// CleanupIdleSessions archives sessions that have been idle beyond the TTL.
func (sl *SessionLifecycle) CleanupIdleSessions() (int, error) {
	idleTTL, err := time.ParseDuration(sl.cfg.IdleTTL)
	if err != nil {
		return 0, fmt.Errorf("parse idle_ttl: %w", err)
	}

	cutoff := time.Now().Add(-idleTTL)
	sessions, err := sl.db.ListIdleSessionsOlderThan(cutoff)
	if err != nil {
		return 0, fmt.Errorf("list idle sessions: %w", err)
	}

	archived := 0
	for _, session := range sessions {
		if err := sl.db.ArchiveSession(session.ID); err != nil {
			log.Printf("[WARN] Failed to archive idle session %s: %v", session.ID, err)
			continue
		}
		archived++
		log.Printf("[INFO] Archived idle session %s (last active: %s)", session.ID, session.LastActiveAt.Format(time.RFC3339))
	}

	if archived > 0 {
		log.Printf("[INFO] Archived %d idle sessions", archived)
	}
	return archived, nil
}

// CleanupArchivedWorkspaces deletes workspace directories for archived sessions
// that have exceeded the workspace retention period.
func (sl *SessionLifecycle) CleanupArchivedWorkspaces() (int, error) {
	retention, err := time.ParseDuration(sl.cfg.WorkspaceRetention)
	if err != nil {
		return 0, fmt.Errorf("parse workspace_retention: %w", err)
	}

	sessions, err := sl.db.ListArchivedSessionsWithWorkspace()
	if err != nil {
		return 0, fmt.Errorf("list archived sessions: %w", err)
	}

	cleaned := 0
	for _, session := range sessions {
		// Check if retention period has passed
		if time.Since(session.LastActiveAt) < retention {
			continue
		}

		if session.WorkspacePath == "" {
			continue
		}

		// Delete workspace directory
		if err := os.RemoveAll(session.WorkspacePath); err != nil {
			log.Printf("[WARN] Failed to delete workspace %s: %v", session.WorkspacePath, err)
			continue
		}

		// Clear workspace path in session
		session.WorkspacePath = ""
		if err := sl.db.UpdateSession(session); err != nil {
			log.Printf("[WARN] Failed to clear workspace path for session %s: %v", session.ID, err)
		}

		cleaned++
		log.Printf("[INFO] Deleted archived workspace: %s", session.WorkspacePath)
	}

	if cleaned > 0 {
		log.Printf("[INFO] Cleaned up %d archived workspaces", cleaned)
	}
	return cleaned, nil
}

// StartCleanupLoop starts a background goroutine that periodically cleans up
// idle sessions and archived workspaces.
func (sl *SessionLifecycle) StartCleanupLoop(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("[INFO] Session cleanup loop started (interval: %s)", interval)
		for range ticker.C {
			if _, err := sl.CleanupIdleSessions(); err != nil {
				log.Printf("[WARN] Idle session cleanup error: %v", err)
			}
			if _, err := sl.CleanupArchivedWorkspaces(); err != nil {
				log.Printf("[WARN] Workspace cleanup error: %v", err)
			}
		}
	}()
}

// scheduleWorkspaceDeletion schedules workspace deletion after the given duration.
func (sl *SessionLifecycle) scheduleWorkspaceDeletion(repo string, issueID int, retentionStr string) {
	retention, err := time.ParseDuration(retentionStr)
	if err != nil {
		log.Printf("[WARN] Invalid retention duration %q: %v", retentionStr, err)
		return
	}

	go func() {
		time.Sleep(retention)
		if err := sl.sessionSvc.ArchiveByIssue(repo, issueID); err != nil {
			log.Printf("[WARN] Scheduled workspace deletion failed for %s#%d: %v", repo, issueID, err)
		}
	}()
}

// GetRepoWorkspaceSize returns the total size of session workspaces for a repo.
func (sl *SessionLifecycle) GetRepoWorkspaceSize(repo string) (int64, error) {
	if sl.baseDir == "" {
		return 0, nil
	}

	sessionsDir := filepath.Join(sl.baseDir, "sessions")
	var totalSize int64

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionPath := filepath.Join(sessionsDir, entry.Name(), "repo")
		size, err := dirSize(sessionPath)
		if err != nil {
			continue
		}
		totalSize += size
	}

	return totalSize, nil
}

// dirSize returns the total size of files in a directory.
func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
