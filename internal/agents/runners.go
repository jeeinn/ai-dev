package agents

import (
	"context"
	"fmt"
	"log"
	"strings"

	"gitea-agent-gateway/internal/agent"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/sandbox"
	"gitea-agent-gateway/internal/store"
)

// Runner is the interface for task execution strategies.
type Runner interface {
	// Run executes the task and returns the result.
	Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error)
}

// Result contains the output of a task execution.
type Result struct {
	Content     string // Main content (comment body)
	Action      string // Optional action: "comment", "label", "pr"
	ActionData  map[string]interface{} // Additional data for the action
}

// GiteaClientFactory creates Gitea clients.
type GiteaClientFactory interface {
	GetGiteaClient(token string) *gitea.Client
}

// RunnerFactory creates runners based on task type.
type RunnerFactory struct {
	llmRegistry *llm.Registry
	giteaFactory GiteaClientFactory
	sandboxCfg   sandbox.Config
	db           *store.DB
}

// NewRunnerFactory creates a new RunnerFactory.
func NewRunnerFactory(llmRegistry *llm.Registry, giteaFactory GiteaClientFactory, db *store.DB) *RunnerFactory {
	return &RunnerFactory{
		llmRegistry:  llmRegistry,
		giteaFactory: giteaFactory,
		sandboxCfg:   sandbox.DefaultConfig(),
		db:           db,
	}
}

// GetRunner returns the appropriate runner for the task type.
func (f *RunnerFactory) GetRunner(taskType string) Runner {
	switch taskType {
	case "review_pr":
		return NewReviewRunner(f.llmRegistry, f.giteaFactory)
	case "reply_comment":
		return NewInteractionRunner(f.llmRegistry, f.giteaFactory)
	case "analyze_issue", "trigger":
		return NewAnalyzeRunner(f.llmRegistry, f.giteaFactory)
	case "solve_issue":
		return NewDevRunner(f.llmRegistry, f.giteaFactory, f.sandboxCfg, f.db)
	case "fix_bug":
		return NewBugfixRunner(f.llmRegistry, f.giteaFactory, f.sandboxCfg, f.db)
	default:
		return NewAnalyzeRunner(f.llmRegistry, f.giteaFactory)
	}
}

// --- AnalyzeRunner ---

// AnalyzeRunner handles issue analysis tasks.
type AnalyzeRunner struct {
	llmRegistry  *llm.Registry
	giteaFactory GiteaClientFactory
}

// NewAnalyzeRunner creates a new AnalyzeRunner.
func NewAnalyzeRunner(llmRegistry *llm.Registry, giteaFactory GiteaClientFactory) *AnalyzeRunner {
	return &AnalyzeRunner{
		llmRegistry:  llmRegistry,
		giteaFactory: giteaFactory,
	}
}

// Run executes the analyze task.
func (r *AnalyzeRunner) Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error) {
	// Get LLM provider
	provider, err := r.llmRegistry.Get(agent.Provider)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Build messages
	messages := []llm.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: task.Context},
	}

	// Call LLM
	resp, err := provider.ChatCompletion(ctx, &llm.ChatRequest{
		Model:       agent.Model,
		Messages:    messages,
		MaxTokens:   agent.MaxTokens,
		Temperature: agent.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	log.Printf("[INFO] Task %d LLM response: %d tokens used", task.ID, resp.Usage.TotalTokens)

	return &Result{
		Content: resp.Content,
		Action:  "comment",
	}, nil
}

// --- ReviewRunner ---

// ReviewRunner handles PR review tasks.
type ReviewRunner struct {
	llmRegistry  *llm.Registry
	giteaFactory GiteaClientFactory
}

// NewReviewRunner creates a new ReviewRunner.
func NewReviewRunner(llmRegistry *llm.Registry, giteaFactory GiteaClientFactory) *ReviewRunner {
	return &ReviewRunner{
		llmRegistry:  llmRegistry,
		giteaFactory: giteaFactory,
	}
}

// Run executes the review task.
func (r *ReviewRunner) Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error) {
	// Parse repo owner/name
	parts := strings.SplitN(task.Repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", task.Repo)
	}
	owner, repo := parts[0], parts[1]

	// Get Gitea client
	client := r.giteaFactory.GetGiteaClient(agent.GiteaToken)

	// Get PR diff
	diff, err := client.PRDiff(owner, repo, task.IssueID)
	if err != nil {
		return nil, fmt.Errorf("get PR diff: %w", err)
	}

	// Get PR details
	pr, err := client.PRGet(owner, repo, task.IssueID)
	if err != nil {
		return nil, fmt.Errorf("get PR: %w", err)
	}

	// Get PR files
	files, err := client.PRFiles(owner, repo, task.IssueID)
	if err != nil {
		log.Printf("[WARN] Failed to get PR files: %v", err)
	}

	// Build context with diff
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Repository: %s\n", task.Repo))
	sb.WriteString(fmt.Sprintf("PR #%d: %v\n", task.IssueID, pr["title"]))
	sb.WriteString(fmt.Sprintf("Description: %v\n\n", pr["body"]))
	sb.WriteString("## Changed Files\n")
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("- %s (+%d/-%d)\n", f.Filename, f.Additions, f.Deletions))
	}
	sb.WriteString("\n## Diff\n")
	sb.WriteString(diff)

	// Get LLM provider
	provider, err := r.llmRegistry.Get(agent.Provider)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Build messages
	messages := []llm.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: sb.String()},
	}

	// Call LLM
	resp, err := provider.ChatCompletion(ctx, &llm.ChatRequest{
		Model:       agent.Model,
		Messages:    messages,
		MaxTokens:   agent.MaxTokens,
		Temperature: agent.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	log.Printf("[INFO] Task %d LLM response: %d tokens used", task.ID, resp.Usage.TotalTokens)

	return &Result{
		Content: resp.Content,
		Action:  "comment",
	}, nil
}

// --- InteractionRunner ---

// InteractionRunner handles @Mention reply tasks.
type InteractionRunner struct {
	llmRegistry  *llm.Registry
	giteaFactory GiteaClientFactory
}

// NewInteractionRunner creates a new InteractionRunner.
func NewInteractionRunner(llmRegistry *llm.Registry, giteaFactory GiteaClientFactory) *InteractionRunner {
	return &InteractionRunner{
		llmRegistry:  llmRegistry,
		giteaFactory: giteaFactory,
	}
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
	client := r.giteaFactory.GetGiteaClient(agent.GiteaToken)

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
	provider, err := r.llmRegistry.Get(agent.Provider)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Build messages
	messages := []llm.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: sb.String()},
	}

	// Call LLM
	resp, err := provider.ChatCompletion(ctx, &llm.ChatRequest{
		Model:       agent.Model,
		Messages:    messages,
		MaxTokens:   agent.MaxTokens,
		Temperature: agent.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	log.Printf("[INFO] Task %d LLM response: %d tokens used", task.ID, resp.Usage.TotalTokens)

	return &Result{
		Content: resp.Content,
		Action:  "comment",
	}, nil
}

// --- DevRunner ---

// DevRunner handles development tasks (read issue → write code → create PR).
type DevRunner struct {
	llmRegistry  *llm.Registry
	giteaFactory GiteaClientFactory
	sandboxCfg   sandbox.Config
	db           *store.DB
}

// NewDevRunner creates a new DevRunner.
func NewDevRunner(llmRegistry *llm.Registry, giteaFactory GiteaClientFactory, sandboxCfg sandbox.Config, db *store.DB) *DevRunner {
	return &DevRunner{
		llmRegistry:  llmRegistry,
		giteaFactory: giteaFactory,
		sandboxCfg:   sandboxCfg,
		db:           db,
	}
}

// Run executes the development task.
func (r *DevRunner) Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error) {
	return runWriteTask(ctx, task, agent, r.llmRegistry, r.giteaFactory, r.sandboxCfg, "dev", r.db)
}

// --- BugfixRunner ---

// BugfixRunner handles bug fix tasks (read bug → locate → fix → create PR).
type BugfixRunner struct {
	llmRegistry  *llm.Registry
	giteaFactory GiteaClientFactory
	sandboxCfg   sandbox.Config
	db           *store.DB
}

// NewBugfixRunner creates a new BugfixRunner.
func NewBugfixRunner(llmRegistry *llm.Registry, giteaFactory GiteaClientFactory, sandboxCfg sandbox.Config, db *store.DB) *BugfixRunner {
	return &BugfixRunner{
		llmRegistry:  llmRegistry,
		giteaFactory: giteaFactory,
		sandboxCfg:   sandboxCfg,
		db:           db,
	}
}

// Run executes the bugfix task.
func (r *BugfixRunner) Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error) {
	return runWriteTask(ctx, task, agent, r.llmRegistry, r.giteaFactory, r.sandboxCfg, "bugfix", r.db)
}

// runWriteTask is the shared implementation for write-type runners.
func runWriteTask(ctx context.Context, task *store.Task, agentCfg *store.Agent,
	llmRegistry *llm.Registry, giteaFactory GiteaClientFactory, sandboxCfg sandbox.Config, taskSubType string, db *store.DB) (*Result, error) {

	// Parse repo owner/name
	parts := strings.SplitN(task.Repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", task.Repo)
	}
	owner, repo := parts[0], parts[1]

	// Get Gitea client
	client := giteaFactory.GetGiteaClient(agentCfg.GiteaToken)

	// Get repo info for clone URL
	repoInfo, err := client.GetRepo(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("get repo info: %w", err)
	}
	cloneURL := repoInfo.CloneURL

	// Create sandbox
	sb := sandbox.New(sandboxCfg, task.ID)
	if err := sb.Setup(); err != nil {
		return nil, fmt.Errorf("setup sandbox: %w", err)
	}
	defer sb.Cleanup()

	// Create audit logger
	audit := sandbox.NewAuditLogger(db, task.ID, agentCfg.ID)

	// Clone repository
	git := sandbox.NewGit(sb)
	cloneResult := git.Clone(cloneURL)
	audit.LogCommand("git", []string{"clone", cloneURL}, cloneResult)
	if cloneResult.Error != nil {
		return nil, fmt.Errorf("clone repo: %w", cloneResult.Error)
	}

	// Create branch
	branchName := sandbox.GenerateBranchName(taskSubType, task.ID)
	branchResult := git.CreateBranch(branchName)
	audit.LogCommand("git", []string{"checkout", "-b", branchName}, branchResult)
	if branchResult.Error != nil {
		return nil, fmt.Errorf("create branch: %w", branchResult.Error)
	}

	// Get LLM provider
	provider, err := llmRegistry.Get(agentCfg.Provider)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Load code context
	codeCtx, err := agent.LoadCodeContext(sb, 8000)
	if err != nil {
		log.Printf("[WARN] Failed to load code context: %v", err)
	}

	// Build prompt based on task type
	taskCtx := agent.TaskContext{
		IssueTitle: task.Event,
		IssueBody:  task.Context,
		RepoName:   task.Repo,
		TaskType:   taskSubType,
	}

	var systemPrompt string
	if taskSubType == "dev" {
		systemPrompt = agent.BuildDevPrompt(taskCtx, codeCtx)
	} else {
		systemPrompt = agent.BuildBugfixPrompt(taskCtx, codeCtx)
	}

	// Create tool registry
	toolRegistry := agent.DefaultTools(sb)

	// Create agent loop with config priority: Agent.LoopConfig > Default
	maxTokens := agentCfg.MaxTokens * 2
	if agentCfg.LoopConfig != nil && agentCfg.LoopConfig.MaxTokens > 0 {
		maxTokens = agentCfg.LoopConfig.MaxTokens
	}

	loop := agent.NewAgentLoop(provider, toolRegistry, agentCfg.Model, maxTokens, agentCfg.Temperature)

	// Apply agent-specific loop config
	if agentCfg.LoopConfig != nil {
		if agentCfg.LoopConfig.MaxIterations > 0 {
			loop.SetMaxIterations(agentCfg.LoopConfig.MaxIterations)
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
	commitMsg := fmt.Sprintf("feat: AI solution for task %d", task.ID)
	if taskSubType == "bugfix" {
		commitMsg = fmt.Sprintf("fix: AI bugfix for task %d", task.ID)
	}
	commitResult := git.Commit(commitMsg)
	audit.LogCommand("git", []string{"commit"}, commitResult)
	if commitResult.Error != nil {
		return nil, fmt.Errorf("commit: %w", commitResult.Error)
	}

	// Push to remote
	pushResult := git.Push("origin", branchName)
	audit.LogCommand("git", []string{"push", "origin", branchName}, pushResult)
	if pushResult.Error != nil {
		return nil, fmt.Errorf("push: %w", pushResult.Error)
	}

	// Create PR
	prTitle := fmt.Sprintf("AI Solution: %s", task.Event)
	if taskSubType == "bugfix" {
		prTitle = fmt.Sprintf("Bugfix: %s", task.Event)
	}
	contentPreview := result
	if len(contentPreview) > 500 {
		contentPreview = contentPreview[:500] + "..."
	}
	prBody := fmt.Sprintf("## AI Generated Solution\n\n%s\n\n---\n*Task ID: %d*", contentPreview, task.ID)
	pr, err := client.CreatePR(owner, repo, gitea.CreatePRRequest{
		Title: prTitle,
		Body:  prBody,
		Head:  branchName,
		Base:  "main",
	})
	if err != nil {
		return nil, fmt.Errorf("create PR: %w", err)
	}

	log.Printf("[INFO] Task %d PR created: %s", task.ID, pr.HTMLURL)

	return &Result{
		Content: fmt.Sprintf("PR created: %s\n\n%s", pr.HTMLURL, result),
		Action:  "pr",
	}, nil
}
