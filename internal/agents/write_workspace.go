package agents

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/sandbox"
	"gitea-agent-gateway/internal/store"
)

// WriteWorkspaceContext holds the prepared workspace state for a write task.
// It is produced by prepareWriteWorkspace and consumed by finalizeWriteChanges,
// so the coding phase (AgentLoop / CodingBackend.Run) sits between the two.
type WriteWorkspaceContext struct {
	Sandbox       *sandbox.Sandbox
	Git           *sandbox.Git
	Audit         *sandbox.AuditLogger
	BranchName    string
	Owner         string
	Repo          string
	RepoInfo      *gitea.RepoInfo
	UseSession    bool // true if workspace is session-scoped (persists, no auto-cleanup)
	SessionBranch string
}

// prepareWriteWorkspace sets up the sandbox, clones or syncs the repository, and
// prepares the working branch for a write task (dev / bugfix).
//
// This is a pure extraction of the workspace-preparation phase previously inlined
// in runWriteTask; behavior is unchanged. On error the non-session sandbox is
// cleaned up here (mirroring the original defer). On success the caller owns the
// sandbox lifecycle: if !wwc.UseSession, the caller must `defer wwc.Sandbox.Cleanup()`.
func prepareWriteWorkspace(ctx context.Context, task *store.Task, agent *store.Agent, factory *RunnerFactory, taskSubType string) (*WriteWorkspaceContext, error) {
	_ = ctx // reserved for future use (e.g. cancellable clone)

	// Parse repo owner/name
	parts := strings.SplitN(task.Repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", task.Repo)
	}
	owner, repo := parts[0], parts[1]

	// Get Gitea client
	client := factory.giteaFactory.GetGiteaClient(agent.GiteaToken)

	// Get repo info for clone URL
	repoInfo, err := client.GetRepo(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("get repo info: %w", err)
	}
	cloneURL, err := gitea.AuthenticatedCloneURL(repoInfo.CloneURL, agent.GiteaUsername, agent.GiteaToken)
	if err != nil {
		return nil, fmt.Errorf("authenticated clone url: %w", err)
	}
	redactedCloneURL := gitea.RedactCloneURL(cloneURL)

	// Determine workspace strategy: session-level or task-level
	var sb *sandbox.Sandbox
	useSessionWorkspace := false
	var sessionBranch string

	if task.SessionID != "" && factory.db != nil {
		// Look up session for workspace reuse
		if session, err := factory.db.GetSession(task.SessionID); err == nil && session.WorkspacePath != "" {
			useSessionWorkspace = true
			sessionBranch = session.Branch
			sb = sandbox.NewWithPath(factory.sandboxCfg, task.ID, session.WorkspacePath)
			log.Printf("[INFO] Using session workspace: %s", session.WorkspacePath)
		}
	}

	if sb == nil {
		sb = sandbox.New(factory.sandboxCfg, task.ID)
	}

	if err := sb.Setup(); err != nil {
		return nil, fmt.Errorf("setup sandbox: %w", err)
	}

	wwc := &WriteWorkspaceContext{
		Sandbox:       sb,
		Owner:         owner,
		Repo:          repo,
		RepoInfo:      repoInfo,
		UseSession:    useSessionWorkspace,
		SessionBranch: sessionBranch,
	}
	// cleanupOnErr mirrors the original `defer sb.Cleanup()` for non-session
	// workspaces when runWriteTask returned an error during preparation.
	cleanupOnErr := func() {
		if !useSessionWorkspace && wwc.Sandbox != nil {
			wwc.Sandbox.Cleanup()
		}
	}

	// Create audit logger
	audit := sandbox.NewAuditLogger(factory.db, task.ID, agent.ID)
	wwc.Audit = audit

	// Clone or fetch repository
	git := sandbox.NewGit(sb)
	wwc.Git = git

	if useSessionWorkspace && sb.WorkDir != "" {
		// Check if the session workspace already has a git repo
		gitDir := filepath.Join(sb.WorkDir, ".git")
		if _, statErr := os.Stat(gitDir); statErr == nil {
			log.Printf("[INFO] Session workspace has existing repo, syncing")
			if err := syncSessionWorkspace(sb, git, audit, task, sessionBranch); err != nil {
				cleanupOnErr()
				return nil, err
			}
		} else {
			// New session workspace — clone
			cloneResult := git.Clone(cloneURL)
			audit.LogCommand("git", []string{"clone", redactedCloneURL}, cloneResult)
			if cloneResult.Error != nil {
				errMsg := cloneResult.Stderr
				if errMsg == "" {
					errMsg = cloneResult.Error.Error()
				}
				cleanupOnErr()
				return nil, fmt.Errorf("clone repo: %s", errMsg)
			}
		}
	} else {
		// Standard task-level clone
		cloneResult := git.Clone(cloneURL)
		audit.LogCommand("git", []string{"clone", redactedCloneURL}, cloneResult)
		if cloneResult.Error != nil {
			errMsg := cloneResult.Stderr
			if errMsg == "" {
				errMsg = cloneResult.Error.Error()
			}
			cleanupOnErr()
			return nil, fmt.Errorf("clone repo: %s", errMsg)
		}
	}

	branchName, isExistingBranch := resolveBranchPlan(task, sessionBranch, taskSubType, git)
	wwc.BranchName = branchName

	if isExistingBranch {
		if err := prepareExistingBranch(sb, git, audit, branchName); err != nil {
			cleanupOnErr()
			return nil, err
		}
		log.Printf("[INFO] Checked out existing branch: %s", branchName)
	} else {
		// Create new branch
		branchResult := git.CreateBranch(branchName)
		audit.LogCommand("git", []string{"checkout", "-b", branchName}, branchResult)
		if branchResult.Error != nil {
			errMsg := branchResult.Stderr
			if errMsg == "" {
				errMsg = branchResult.Error.Error()
			}
			cleanupOnErr()
			return nil, fmt.Errorf("create branch: %s", errMsg)
		}
		saveSessionBranch(factory, task, branchName)
	}

	return wwc, nil
}

// finalizeWriteChanges checks for uncommitted changes, stages, commits, pushes,
// and creates or updates the PR. Behavior is identical to the finalize phase
// previously inlined in runWriteTask.
//
// The provider is resolved once in the coding phase (runWriteTask) and passed in
// so the same instance is reused for the commit-message LLM call, matching the
// pre-refactor behavior. If the workspace has no changes, a comment-style Result
// is returned without touching git/PR. The agentResult string is the coder's
// summary (used as PR body / comment content and as input to the commit-message
// generator).
func finalizeWriteChanges(ctx context.Context, wwc *WriteWorkspaceContext, task *store.Task, agent *store.Agent, factory *RunnerFactory, provider llm.Provider, taskSubType, agentResult string) (*Result, error) {
	git := wwc.Git
	branchName := wwc.BranchName
	audit := wwc.Audit

	// Check if there are changes to commit
	if !git.HasChanges() {
		return &Result{
			Content: agentResult,
			Action:  "comment",
		}, nil
	}

	// Stage and commit
	git.Add()
	commitMsg := GenerateCommitMessage(ctx, CommitMessageInput{
		Git:          git,
		Provider:     provider,
		Model:        agent.Model,
		Temperature:  factory.resolveTemperature(agent.Temperature, agent.Provider, agent.Model),
		TaskSubType:  taskSubType,
		Task:         task,
		AgentSummary: agentResult,
	})
	log.Printf("[INFO] Task %d commit message: %s", task.ID, commitMsg)
	commitResult := git.Commit(commitMsg)
	audit.LogCommand("git", []string{"commit"}, commitResult)
	if commitResult.Error != nil {
		return nil, fmt.Errorf("commit: %w", commitResult.Error)
	}

	// Push to remote
	pushResult := git.Push("origin", branchName)
	audit.LogCommand("git", []string{"push", "origin", branchName}, pushResult)
	if pushResult.Error != nil {
		errMsg := pushResult.Stderr
		if errMsg == "" {
			errMsg = pushResult.Error.Error()
		}
		return nil, fmt.Errorf("push: %s", errMsg)
	}

	// Update session branch after successful push
	if wwc.UseSession {
		saveSessionBranch(factory, task, branchName)
	}

	adminClient := factory.giteaFactory.GetAdminGiteaClient()
	return finalizeWriteTaskPR(adminClient, wwc.Owner, wwc.Repo, branchName, wwc.RepoInfo.DefaultBranch, task, taskSubType, agentResult)
}

// prepareAnalyzeWorkspace sets up a temporary sandbox with a shallow clone of
// the repository's default branch for read-only analysis tasks.
//
// No branch is created, no changes are pushed, and the workspace is always
// cleaned up by the caller. On clone failure an error is returned so the
// caller can fall back to single-shot analysis.
func prepareAnalyzeWorkspace(ctx context.Context, task *store.Task, agent *store.Agent, factory *RunnerFactory) (*WriteWorkspaceContext, error) {
	_ = ctx

	// Parse repo owner/name
	parts := strings.SplitN(task.Repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", task.Repo)
	}
	owner, repo := parts[0], parts[1]

	// Get Gitea client
	client := factory.giteaFactory.GetGiteaClient(agent.GiteaToken)

	// Get repo info for clone URL and default branch
	repoInfo, err := client.GetRepo(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("get repo info: %w", err)
	}
	cloneURL, err := gitea.AuthenticatedCloneURL(repoInfo.CloneURL, agent.GiteaUsername, agent.GiteaToken)
	if err != nil {
		return nil, fmt.Errorf("authenticated clone url: %w", err)
	}
	redactedCloneURL := gitea.RedactCloneURL(cloneURL)

	// Create temporary sandbox (always cleaned up by caller)
	sb := sandbox.New(factory.sandboxCfg, task.ID)
	if err := sb.Setup(); err != nil {
		return nil, fmt.Errorf("setup sandbox: %w", err)
	}

	wwc := &WriteWorkspaceContext{
		Sandbox:    sb,
		Owner:      owner,
		Repo:       repo,
		RepoInfo:   repoInfo,
		UseSession: false,
	}

	git := sandbox.NewGit(sb)
	wwc.Git = git

	// Shallow clone default branch
	cloneResult := git.CloneBranch(cloneURL, repoInfo.DefaultBranch)
	wwc.Audit = sandbox.NewAuditLogger(factory.db, task.ID, agent.ID)
	wwc.Audit.LogCommand("git", []string{"clone", "--depth", "1", "--branch", repoInfo.DefaultBranch, redactedCloneURL}, cloneResult)

	if cloneResult.Error != nil {
		errMsg := cloneResult.Stderr
		if errMsg == "" {
			errMsg = cloneResult.Error.Error()
		}
		sb.Cleanup()
		return nil, fmt.Errorf("clone repo: %s", errMsg)
	}

	return wwc, nil
}
