package agents

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	agentpkg "gitea-agent-gateway/internal/agent"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/sandbox"
	"gitea-agent-gateway/internal/store"
)

const (
	fallbackMaxOutput = 2048
	fallbackMaxInput  = 65536
	fallbackTemp      = 0.3
	fallbackTimeout   = "5m"
)

// Runner is the interface for task execution strategies.
type Runner interface {
	// Run executes the task and returns the result.
	Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error)
}

// Result contains the output of a task execution.
type Result struct {
	Content    string                 // Main content (comment body)
	Action     string                 // Optional action: "comment", "label", "pr"
	PRID       int                    // PR number created by DevRunner (0 if no PR created)
	ActionData map[string]interface{} // Additional data for the action
}

// GiteaClientFactory creates Gitea clients.
type GiteaClientFactory interface {
	GetGiteaClient(token string) *gitea.Client
	GetAdminGiteaClient() *gitea.Client
}

// ModelMetaProvider can return model metadata for a given provider+model.
type ModelMetaProvider interface {
	GetModelMeta(provider, model string) *config.ModelDefinition
}

// RunnerFactory creates runners based on task type.
type RunnerFactory struct {
	llmRegistry      *llm.Registry
	giteaFactory     GiteaClientFactory
	sandboxCfg       sandbox.Config
	db               *store.DB
	defaultMaxOutput int
	defaultMaxInput  int
	defaultTemp      float64
	defaultTimeout   string
	defaultLoop      config.AgentLoopConfig
	getDebugConfig   func() config.DebugConfig
	modelMeta        ModelMetaProvider
}

// NewRunnerFactory creates a new RunnerFactory from agent defaults and loop config.
func NewRunnerFactory(llmRegistry *llm.Registry, giteaFactory GiteaClientFactory, db *store.DB, defaults config.AgentDefaultsConfig, defaultLoop config.AgentLoopConfig, getDebugConfig func() config.DebugConfig) *RunnerFactory {
	maxOut := defaults.MaxOutputTokens
	if maxOut <= 0 {
		maxOut = fallbackMaxOutput
	}
	maxIn := defaults.MaxInputTokens
	if maxIn <= 0 {
		maxIn = fallbackMaxInput
	}
	temp := defaults.Temperature
	if temp <= 0 {
		temp = fallbackTemp
	}
	timeout := defaults.Timeout
	if timeout == "" {
		timeout = fallbackTimeout
	}
	if defaultLoop.MaxIterations <= 0 {
		defaultLoop = config.DefaultAgentLoopConfig()
	}
	return &RunnerFactory{
		llmRegistry:      llmRegistry,
		giteaFactory:     giteaFactory,
		sandboxCfg:       sandbox.DefaultConfig(),
		db:               db,
		defaultMaxOutput: maxOut,
		defaultMaxInput:  maxIn,
		defaultTemp:      temp,
		defaultTimeout:   timeout,
		defaultLoop:      defaultLoop,
		getDebugConfig:   getDebugConfig,
	}
}

// SetModelMetaProvider sets the model metadata provider for adaptive token limits.
func (f *RunnerFactory) SetModelMetaProvider(m ModelMetaProvider) {
	f.modelMeta = m
}

func (f *RunnerFactory) resolveMaxOutputTokens(agentMax int, provider, model string) int {
	base := f.defaultMaxOutput
	if agentMax > 0 {
		base = agentMax
	}
	// Clamp to model's max_output if known
	if f.modelMeta != nil {
		if meta := f.modelMeta.GetModelMeta(provider, model); meta != nil && meta.MaxOutput > 0 {
			if base == 0 || base > meta.MaxOutput {
				return meta.MaxOutput
			}
		}
	}
	if base <= 0 {
		return fallbackMaxOutput
	}
	return base
}

func (f *RunnerFactory) resolveMaxInputTokens(agentMax int, provider, model string) int {
	base := f.defaultMaxInput
	if agentMax > 0 {
		base = agentMax
	}
	// Use 90% of model context window as upper limit (reserve 10% for output)
	if f.modelMeta != nil {
		if meta := f.modelMeta.GetModelMeta(provider, model); meta != nil && meta.ContextWindow > 0 {
			modelLimit := int(float64(meta.ContextWindow) * 0.9)
			if base == 0 || base > modelLimit {
				return modelLimit
			}
		}
	}
	if base <= 0 {
		return fallbackMaxInput
	}
	return base
}

// resolveTemperature returns agent.Temperature if explicitly set (> 0), otherwise the factory default.
// Note: Temperature=0 (deterministic output) is a valid value but rarely used in practice.
// Agents with Temperature=0 will fall back to default — set it via Agent edit if needed.
func (f *RunnerFactory) resolveTemperature(agentTemp float64, provider, model string) float64 {
	base := f.defaultTemp
	if agentTemp > 0 {
		base = agentTemp
	}
	if f.modelMeta != nil {
		if meta := f.modelMeta.GetModelMeta(provider, model); meta != nil {
			if p := meta.DefaultParams.Temperature; p != nil && *p > 0 && agentTemp <= 0 {
				return *p
			}
		}
	}
	if base <= 0 {
		return fallbackTemp
	}
	return base
}

func (f *RunnerFactory) getModelMeta(provider, model string) *config.ModelDefinition {
	if f.modelMeta == nil {
		return nil
	}
	return f.modelMeta.GetModelMeta(provider, model)
}

func (f *RunnerFactory) recordTaskUsage(taskID int64, provider, model string, usage llm.Usage) {
	if f.db == nil {
		return
	}
	go func() {
		if err := f.db.CreateTaskUsage(&store.TaskUsage{
			TaskID:           taskID,
			Provider:         provider,
			Model:            model,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}); err != nil {
			log.Printf("[WARN] Failed to record task usage: %v", err)
		}
	}()
}

func (f *RunnerFactory) resolveTimeout(agentTimeout string) string {
	if agentTimeout != "" {
		return agentTimeout
	}
	if f.defaultTimeout != "" {
		return f.defaultTimeout
	}
	return fallbackTimeout
}

// GetRunner returns the appropriate runner for the task type.
func (f *RunnerFactory) GetRunner(taskType string) Runner {
	switch taskType {
	case "review_pr":
		return NewReviewRunner(f)
	case "reply_comment":
		return NewInteractionRunner(f)
	case "analyze_issue", "trigger":
		return NewAnalyzeRunner(f)
	case "solve_issue", "solve_comment":
		return NewDevRunner(f)
	case "fix_bug":
		return NewBugfixRunner(f)
	default:
		return NewAnalyzeRunner(f)
	}
}

// --- AnalyzeRunner ---

// AnalyzeRunner handles issue analysis tasks.
type AnalyzeRunner struct {
	factory *RunnerFactory
}

// NewAnalyzeRunner creates a new AnalyzeRunner.
func NewAnalyzeRunner(factory *RunnerFactory) *AnalyzeRunner {
	return &AnalyzeRunner{factory: factory}
}

// Run executes the analyze task.
func (r *AnalyzeRunner) Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error) {
	// Get LLM provider
	provider, err := r.factory.llmRegistry.Get(agent.Provider)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Build messages
	messages := []llm.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: task.Context},
	}

	messages, err = agentpkg.TruncateMessages(messages, nil, r.factory.resolveMaxInputTokens(agent.MaxInputTokens, agent.Provider, agent.Model), r.factory.getModelMeta(agent.Provider, agent.Model))
	if err != nil {
		return nil, fmt.Errorf("truncate messages: %w", err)
	}

	// Call LLM
	resp, err := provider.ChatCompletion(ctx, &llm.ChatRequest{
		Model:       agent.Model,
		Messages:    messages,
		MaxTokens:   r.factory.resolveMaxOutputTokens(agent.MaxOutputTokens, agent.Provider, agent.Model),
		Temperature: r.factory.resolveTemperature(agent.Temperature, agent.Provider, agent.Model),
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	log.Printf("[INFO] Task %d LLM response: %d tokens used", task.ID, resp.Usage.TotalTokens)
	r.factory.recordTaskUsage(task.ID, agent.Provider, agent.Model, resp.Usage)

	return &Result{
		Content: resp.Content,
		Action:  "comment",
	}, nil
}

// --- ReviewRunner ---

// ReviewRunner handles PR review tasks.
type ReviewRunner struct {
	factory *RunnerFactory
}

// NewReviewRunner creates a new ReviewRunner.
func NewReviewRunner(factory *RunnerFactory) *ReviewRunner {
	return &ReviewRunner{factory: factory}
}

// Run executes the review task.
func (r *ReviewRunner) Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error) {
	// Parse repo owner/name
	parts := strings.SplitN(task.Repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", task.Repo)
	}
	owner, repo := parts[0], parts[1]

	// Use PRID for PR API calls; fall back to IssueID for backward compatibility
	prID := task.PRID
	if prID == 0 {
		prID = task.IssueID
		log.Printf("[WARN] Task %d has no PRID, falling back to IssueID=%d for PR API calls", task.ID, prID)
	}

	// Get Gitea client
	client := r.factory.giteaFactory.GetGiteaClient(agent.GiteaToken)

	// Get PR diff
	diff, err := client.PRDiff(owner, repo, prID)
	if err != nil {
		return nil, fmt.Errorf("get PR diff: %w", err)
	}

	// Get PR details
	pr, err := client.PRGet(owner, repo, prID)
	if err != nil {
		return nil, fmt.Errorf("get PR: %w", err)
	}

	// Get PR files
	files, err := client.PRFiles(owner, repo, prID)
	if err != nil {
		log.Printf("[WARN] Failed to get PR files: %v", err)
	}

	// Build context with diff
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Repository: %s\n", task.Repo))
	sb.WriteString(fmt.Sprintf("PR #%d: %v\n", prID, pr["title"]))
	sb.WriteString(fmt.Sprintf("Description: %v\n\n", pr["body"]))
	sb.WriteString("## Changed Files\n")
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("- %s (+%d/-%d)\n", f.Filename, f.Additions, f.Deletions))
	}
	sb.WriteString("\n## Diff\n")
	sb.WriteString(diff)

	// Get LLM provider
	provider, err := r.factory.llmRegistry.Get(agent.Provider)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Build messages
	messages := []llm.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: sb.String()},
	}

	messages, err = agentpkg.TruncateMessages(messages, nil, r.factory.resolveMaxInputTokens(agent.MaxInputTokens, agent.Provider, agent.Model), r.factory.getModelMeta(agent.Provider, agent.Model))
	if err != nil {
		return nil, fmt.Errorf("truncate messages: %w", err)
	}

	// Call LLM
	resp, err := provider.ChatCompletion(ctx, &llm.ChatRequest{
		Model:       agent.Model,
		Messages:    messages,
		MaxTokens:   r.factory.resolveMaxOutputTokens(agent.MaxOutputTokens, agent.Provider, agent.Model),
		Temperature: r.factory.resolveTemperature(agent.Temperature, agent.Provider, agent.Model),
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	log.Printf("[INFO] Task %d LLM response: %d tokens used", task.ID, resp.Usage.TotalTokens)
	r.factory.recordTaskUsage(task.ID, agent.Provider, agent.Model, resp.Usage)

	return &Result{
		Content: resp.Content,
		Action:  "comment",
	}, nil
}

// --- InteractionRunner ---

// InteractionRunner handles @Mention reply tasks.
type InteractionRunner struct {
	factory *RunnerFactory
}

// NewInteractionRunner creates a new InteractionRunner.
func NewInteractionRunner(factory *RunnerFactory) *InteractionRunner {
	return &InteractionRunner{factory: factory}
}

// Run executes the interaction task.
func (r *InteractionRunner) Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error) {
	// Parse repo owner/name
	parts := strings.SplitN(task.Repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", task.Repo)
	}
	owner, repo := parts[0], parts[1]

	// Get Gitea client
	client := r.factory.giteaFactory.GetGiteaClient(agent.GiteaToken)

	// Get comment history for context
	comments, err := client.IssueComments(owner, repo, task.IssueID)
	if err != nil {
		log.Printf("[WARN] Failed to get comments: %v", err)
	}

	// Build context with comment history
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Repository: %s\n", task.Repo))
	sb.WriteString(fmt.Sprintf("Issue/PR #%d\n\n", task.IssueID))
	sb.WriteString("## Comment History\n")
	for i, c := range comments {
		if i >= 10 { // Limit to last 10 comments
			sb.WriteString("... (truncated)\n")
			break
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", c.User.Login, c.Body))
	}

	// Get LLM provider
	provider, err := r.factory.llmRegistry.Get(agent.Provider)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Build messages
	messages := []llm.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: sb.String()},
	}

	messages, err = agentpkg.TruncateMessages(messages, nil, r.factory.resolveMaxInputTokens(agent.MaxInputTokens, agent.Provider, agent.Model), r.factory.getModelMeta(agent.Provider, agent.Model))
	if err != nil {
		return nil, fmt.Errorf("truncate messages: %w", err)
	}

	// Call LLM
	resp, err := provider.ChatCompletion(ctx, &llm.ChatRequest{
		Model:       agent.Model,
		Messages:    messages,
		MaxTokens:   r.factory.resolveMaxOutputTokens(agent.MaxOutputTokens, agent.Provider, agent.Model),
		Temperature: r.factory.resolveTemperature(agent.Temperature, agent.Provider, agent.Model),
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	log.Printf("[INFO] Task %d LLM response: %d tokens used", task.ID, resp.Usage.TotalTokens)
	r.factory.recordTaskUsage(task.ID, agent.Provider, agent.Model, resp.Usage)

	return &Result{
		Content: resp.Content,
		Action:  "comment",
	}, nil
}

// --- DevRunner ---

// DevRunner handles development tasks (read issue → write code → create PR).
type DevRunner struct {
	factory *RunnerFactory
}

// NewDevRunner creates a new DevRunner.
func NewDevRunner(factory *RunnerFactory) *DevRunner {
	return &DevRunner{factory: factory}
}

// Run executes the development task.
func (r *DevRunner) Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error) {
	return runWriteTask(ctx, task, agent, r.factory, "dev")
}

// --- BugfixRunner ---

// BugfixRunner handles bug fix tasks (read bug → locate → fix → create PR).
type BugfixRunner struct {
	factory *RunnerFactory
}

// NewBugfixRunner creates a new BugfixRunner.
func NewBugfixRunner(factory *RunnerFactory) *BugfixRunner {
	return &BugfixRunner{factory: factory}
}

// Run executes the bugfix task.
func (r *BugfixRunner) Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error) {
	return runWriteTask(ctx, task, agent, r.factory, "bugfix")
}

// runWriteTask is the shared implementation for write-type runners.
func runWriteTask(ctx context.Context, task *store.Task, agentCfg *store.Agent,
	factory *RunnerFactory, taskSubType string) (*Result, error) {

	// Parse repo owner/name
	parts := strings.SplitN(task.Repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", task.Repo)
	}
	owner, repo := parts[0], parts[1]

	// Get Gitea client
	client := factory.giteaFactory.GetGiteaClient(agentCfg.GiteaToken)

	// Get repo info for clone URL
	repoInfo, err := client.GetRepo(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("get repo info: %w", err)
	}
	cloneURL, err := gitea.AuthenticatedCloneURL(repoInfo.CloneURL, agentCfg.GiteaUsername, agentCfg.GiteaToken)
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

	// Only cleanup for non-session workspaces (session workspaces persist)
	if !useSessionWorkspace {
		defer sb.Cleanup()
	}

	// Create audit logger
	audit := sandbox.NewAuditLogger(factory.db, task.ID, agentCfg.ID)

	// Clone or fetch repository
	git := sandbox.NewGit(sb)

	if useSessionWorkspace && sb.WorkDir != "" {
		// Check if the session workspace already has a git repo
		gitDir := filepath.Join(sb.WorkDir, ".git")
		if _, statErr := os.Stat(gitDir); statErr == nil {
			log.Printf("[INFO] Session workspace has existing repo, syncing")
			if err := syncSessionWorkspace(sb, git, audit, task, sessionBranch); err != nil {
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
			return nil, fmt.Errorf("clone repo: %s", errMsg)
		}
	}

	branchName, isExistingBranch := resolveBranchPlan(task, sessionBranch, taskSubType, git)

	if isExistingBranch {
		if err := prepareExistingBranch(sb, git, audit, branchName); err != nil {
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
			return nil, fmt.Errorf("create branch: %s", errMsg)
		}
		saveSessionBranch(factory, task, branchName)
	}

	// Get LLM provider
	provider, err := factory.llmRegistry.Get(agentCfg.Provider)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	maxInput := factory.resolveMaxInputTokens(agentCfg.MaxInputTokens, agentCfg.Provider, agentCfg.Model)

	// Load code context
	codeCtx, err := agentpkg.LoadCodeContext(sb, maxInput)
	if err != nil {
		log.Printf("[WARN] Failed to load code context: %v", err)
	}

	// Build prompt based on task type
	taskCtx := agentpkg.TaskContext{
		IssueTitle: task.Event,
		IssueBody:  task.Context,
		RepoName:   task.Repo,
		TaskType:   taskSubType,
	}

	var basePrompt string
	if taskSubType == "dev" {
		basePrompt = agentpkg.BuildDevPrompt(taskCtx, codeCtx)
	} else {
		basePrompt = agentpkg.BuildBugfixPrompt(taskCtx, codeCtx)
	}
	systemPrompt := agentpkg.MergeAgentSystemPrompt(basePrompt, agentCfg.SystemPrompt)

	// Create tool registry
	toolRegistry := agentpkg.DefaultTools(sb)

	// Create agent loop with config priority: Agent.LoopConfig > system agents.loop defaults
	maxOutput := factory.resolveMaxOutputTokens(agentCfg.MaxOutputTokens, agentCfg.Provider, agentCfg.Model)
	mergedLoop := MergeLoopConfig(agentCfg.LoopConfig, factory.defaultLoop)

	loop := agentpkg.NewAgentLoopWithConfig(
		provider,
		toolRegistry,
		agentCfg.Model,
		maxOutput,
		maxInput,
		factory.resolveTemperature(agentCfg.Temperature, agentCfg.Provider, agentCfg.Model),
		mergedLoop,
	)

	loop.SetModelMeta(factory.getModelMeta(agentCfg.Provider, agentCfg.Model))
	loop.SetProviderName(agentCfg.Provider)
	loop.SetUsageRecorder(func(p, m string, usage llm.Usage) {
		factory.recordTaskUsage(task.ID, p, m, usage)
	})

	if factory.getDebugConfig != nil {
		debugCfg := factory.getDebugConfig()
		if debugCfg.ConversationLog.Enabled && factory.db != nil {
			loop.SetConversationRecorder(
				newConversationRecorder(factory.db, debugCfg.ConversationLog.MaxContentChars),
				task.ID,
			)
		}
	}

	// Run agent loop
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task.Context},
	}

	result, err := loop.Run(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}

	log.Printf("[INFO] Task %d agent loop completed", task.ID)

	// Check if there are changes to commit
	if !git.HasChanges() {
		return &Result{
			Content: result,
			Action:  "comment",
		}, nil
	}

	// Stage and commit
	git.Add()
	commitMsg := GenerateCommitMessage(ctx, CommitMessageInput{
		Git:          git,
		Provider:     provider,
		Model:        agentCfg.Model,
		Temperature:  factory.resolveTemperature(agentCfg.Temperature, agentCfg.Provider, agentCfg.Model),
		TaskSubType:  taskSubType,
		Task:         task,
		AgentSummary: result,
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
	if useSessionWorkspace {
		saveSessionBranch(factory, task, branchName)
	}

	adminClient := factory.giteaFactory.GetAdminGiteaClient()
	return finalizeWriteTaskPR(adminClient, owner, repo, branchName, repoInfo.DefaultBranch, task, taskSubType, result)
}

// finalizeWriteTaskPR comments on an existing open PR or creates one if the branch has no open PR yet.
func finalizeWriteTaskPR(adminClient *gitea.Client, owner, repo, branchName, baseBranch string, task *store.Task, taskSubType, agentResult string) (*Result, error) {
	existingPR, err := adminClient.FindOpenPRByHead(owner, repo, branchName)
	if err != nil {
		return nil, fmt.Errorf("find open PR: %w", err)
	}
	if existingPR != nil {
		log.Printf("[INFO] Task %d updated existing branch: %s (PR #%d)", task.ID, branchName, existingPR.Number)
		return &Result{
			Content: fmt.Sprintf("Updated PR branch `%s` with new changes.\n\n%s", branchName, agentResult),
			Action:  "comment",
			PRID:    existingPR.Number,
		}, nil
	}

	prTitle := fmt.Sprintf("AI Solution: %s", task.Event)
	if taskSubType == "bugfix" {
		prTitle = fmt.Sprintf("Bugfix: %s", task.Event)
	}
	contentPreview := agentResult
	if len(contentPreview) > 500 {
		contentPreview = contentPreview[:500] + "..."
	}
	issueLink := ""
	if task.IssueID > 0 {
		issueLink = fmt.Sprintf("\n\nFixes #%d", task.IssueID)
	}
	prBody := fmt.Sprintf("## AI Generated Solution\n\n%s\n\n---\n*Task ID: %d*%s", contentPreview, task.ID, issueLink)

	pr, err := adminClient.CreatePR(owner, repo, gitea.CreatePRRequest{
		Title: prTitle,
		Body:  prBody,
		Head:  branchName,
		Base:  baseBranch,
	})
	if err != nil {
		return nil, fmt.Errorf("create PR: %w", err)
	}

	log.Printf("[INFO] Task %d PR created: %s (PR #%d)", task.ID, pr.HTMLURL, pr.Number)
	return &Result{
		Content: fmt.Sprintf("PR created: %s\n\n%s", pr.HTMLURL, agentResult),
		Action:  "pr",
		PRID:    pr.Number,
	}, nil
}

// saveSessionBranch persists the working branch on the session for workspace reuse.
func saveSessionBranch(factory *RunnerFactory, task *store.Task, branchName string) {
	if task.SessionID == "" || factory.db == nil {
		return
	}
	session, err := factory.db.GetSession(task.SessionID)
	if err != nil {
		return
	}
	if session.Branch == branchName {
		return
	}
	session.Branch = branchName
	factory.db.UpdateSession(session)
}
