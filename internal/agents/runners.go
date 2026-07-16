package agents

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	agentpkg "gitea-agent-gateway/internal/agent"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/mcp"
	"gitea-agent-gateway/internal/sandbox"
	"gitea-agent-gateway/internal/store"
)

const (
	fallbackMaxOutput = config.DefaultMaxOutputTokens
	fallbackMaxInput  = config.DefaultMaxInputTokens
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
	backends         config.AgentBackendsConfig // coding backends (Path A)
	internalBackend  *InternalCodingBackend     // always available, built from this factory
	toolPacks        config.ToolPacksConfig     // built-in + user-defined tool packs
	mcpRegistry      *mcp.Registry              // MCP server registry (nil = no MCP)
	gatewayDir       string                     // gateway root directory for SKILL.md scanning
}

// NewRunnerFactory creates a new RunnerFactory from agent defaults and loop config.
// The backends, toolPacks, and mcpReg configs are optional — nil/empty falls back to defaults.
func NewRunnerFactory(llmRegistry *llm.Registry, giteaFactory GiteaClientFactory, db *store.DB, defaults config.AgentDefaultsConfig, defaultLoop config.AgentLoopConfig, getDebugConfig func() config.DebugConfig, backends *config.AgentBackendsConfig, toolPacks *config.ToolPacksConfig, sandboxCfg sandbox.SandboxConfig, mcpReg *mcp.Registry, gatewayDir string) *RunnerFactory {
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

	beCfg := config.DefaultAgentBackends()
	if backends != nil {
		beCfg = *backends
		config.ApplyBackendDefaults(&beCfg)
	}

	tpCfg := config.DefaultToolPacks()
	if toolPacks != nil {
		tpCfg = *toolPacks
		config.ApplyToolPackDefaults(&tpCfg)
	}

	factory := &RunnerFactory{
		llmRegistry:      llmRegistry,
		giteaFactory:     giteaFactory,
		sandboxCfg:       sandboxCfg,
		db:               db,
		defaultMaxOutput: maxOut,
		defaultMaxInput:  maxIn,
		defaultTemp:      temp,
		defaultTimeout:   timeout,
		defaultLoop:      defaultLoop,
		getDebugConfig:   getDebugConfig,
		backends:         beCfg,
		toolPacks:        tpCfg,
		mcpRegistry:      mcpReg,
		gatewayDir:       gatewayDir,
	}
	factory.internalBackend = NewInternalCodingBackend(factory)
	return factory
}

// SetModelMetaProvider sets the model metadata provider for adaptive token limits.
func (f *RunnerFactory) SetModelMetaProvider(m ModelMetaProvider) {
	f.modelMeta = m
}

// resolveMaxOutputTokens priority: Agent explicit > model max_output > agents.defaults > fallback.
// agentMax == 0 means "use model default" (not agents.defaults).
func (f *RunnerFactory) resolveMaxOutputTokens(agentMax int, provider, model string) int {
	var meta *config.ModelDefinition
	if f.modelMeta != nil {
		meta = f.modelMeta.GetModelMeta(provider, model)
	}

	if agentMax > 0 {
		if meta != nil && meta.MaxOutput > 0 && agentMax > meta.MaxOutput {
			return meta.MaxOutput
		}
		return agentMax
	}

	if meta != nil && meta.MaxOutput > 0 {
		return meta.MaxOutput
	}
	if f.defaultMaxOutput > 0 {
		return f.defaultMaxOutput
	}
	return fallbackMaxOutput
}

// resolveMaxInputTokens priority: Agent explicit > model context_window*90% > agents.defaults > fallback.
// agentMax == 0 means "use model default" (not agents.defaults).
func (f *RunnerFactory) resolveMaxInputTokens(agentMax int, provider, model string) int {
	var meta *config.ModelDefinition
	if f.modelMeta != nil {
		meta = f.modelMeta.GetModelMeta(provider, model)
	}

	modelLimit := 0
	if meta != nil && meta.ContextWindow > 0 {
		modelLimit = int(float64(meta.ContextWindow) * 0.9)
	}

	if agentMax > 0 {
		if modelLimit > 0 && agentMax > modelLimit {
			return modelLimit
		}
		return agentMax
	}

	if modelLimit > 0 {
		return modelLimit
	}
	if f.defaultMaxInput > 0 {
		return f.defaultMaxInput
	}
	return fallbackMaxInput
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
		cost := 0.0
		if f.modelMeta != nil {
			if meta := f.modelMeta.GetModelMeta(provider, model); meta != nil {
				// InputPrice/OutputPrice are $/1K tokens
				cost = (float64(usage.PromptTokens)*meta.InputPrice + float64(usage.CompletionTokens)*meta.OutputPrice) / 1000.0
			}
		}
		if err := f.db.CreateTaskUsage(&store.TaskUsage{
			TaskID:           taskID,
			Provider:         provider,
			Model:            model,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			Cost:             cost,
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

// resolveToolPack returns the tool pack ID for a task based on task type.
// Role-based defaults (agent-level override can be added later when the
// Agent schema gains a tool_pack column):
//   - write tasks (dev/bugfix) → "coder-default"
//   - analyze tasks → "analyze-readonly"
func (f *RunnerFactory) resolveToolPack(taskType string) string {
	switch taskType {
	case "analyze_issue", "trigger", "review_pr", "reply_comment":
		return "analyze-readonly"
	default:
		return "coder-default"
	}
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
//
// It first attempts a shallow clone + short read-only AgentLoop for richer
// analysis. If clone fails it falls back to single-shot LLM (legacy behavior).
func (r *AnalyzeRunner) Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error) {
	provider, err := r.factory.llmRegistry.Get(agent.Provider)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Try read-only workspace + short loop for richer analysis
	wwc, err := prepareAnalyzeWorkspace(ctx, task, agent, r.factory)
	if err != nil {
		log.Printf("[WARN] Task %d analyze workspace failed (%v); falling back to single-shot", task.ID, err)
		return r.runSingleShot(ctx, task, agent, provider)
	}
	defer wwc.Sandbox.Cleanup()

	return r.runAnalyzeLoop(ctx, task, agent, provider, wwc.Sandbox)
}

// runSingleShot is the legacy single-shot LLM analysis (no workspace).
func (r *AnalyzeRunner) runSingleShot(ctx context.Context, task *store.Task, agent *store.Agent, provider llm.Provider) (*Result, error) {
	messages := []llm.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: task.Context},
	}

	messages, err := agentpkg.TruncateMessages(messages, nil, r.factory.resolveMaxInputTokens(agent.MaxInputTokens, agent.Provider, agent.Model), r.factory.getModelMeta(agent.Provider, agent.Model))
	if err != nil {
		return nil, fmt.Errorf("truncate messages: %w", err)
	}

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

// runAnalyzeLoop runs a short read-only AgentLoop on a prepared workspace.
func (r *AnalyzeRunner) runAnalyzeLoop(ctx context.Context, task *store.Task, agent *store.Agent, provider llm.Provider, sb *sandbox.Sandbox) (*Result, error) {
	maxInput := r.factory.resolveMaxInputTokens(agent.MaxInputTokens, agent.Provider, agent.Model)
	maxOutput := r.factory.resolveMaxOutputTokens(agent.MaxOutputTokens, agent.Provider, agent.Model)
	temperature := r.factory.resolveTemperature(agent.Temperature, agent.Provider, agent.Model)

	// Load code context (best-effort)
	codeCtx, err := agentpkg.LoadCodeContext(sb, maxInput)
	if err != nil {
		log.Printf("[WARN] Failed to load code context: %v", err)
	}

	// Build analyze prompt
	taskCtx := agentpkg.TaskContext{
		IssueTitle: task.Event,
		IssueBody:  task.Context,
		RepoName:   task.Repo,
		TaskType:   "analyze",
	}
	basePrompt := agentpkg.BuildAnalyzePrompt(taskCtx, codeCtx)
	systemPrompt := agentpkg.MergeAgentSystemPrompt(basePrompt, agent.SystemPrompt)

	// Assemble analyze-readonly tool pack
	packID := r.factory.resolveToolPack(task.TaskType)
	packCfg, ok := r.factory.toolPacks.Packs[packID]
	if !ok {
		return nil, fmt.Errorf("tool pack %q not found", packID)
	}
	toolRegistry, err := agentpkg.AssembleToolRegistry(packCfg.Tools, sb)
	if err != nil {
		return nil, fmt.Errorf("assemble tool registry for pack %q: %w", packID, err)
	}

	// Register MCP tools if enabled for this agent
	if len(agent.McpServers) > 0 && r.factory.mcpRegistry != nil {
		if err := toolRegistry.RegisterMCPTools(ctx, r.factory.mcpRegistry, agent.McpServers); err != nil {
			return nil, fmt.Errorf("register mcp tools: %w", err)
		}
	}

	// Register skill discovery tools (Analyze: no arbitrary skill scripts)
	skillReg := agentpkg.NewSkillRegistry(sb, r.factory.gatewayDir)
	toolRegistry.Register(agentpkg.NewListSkillsTool(skillReg))
	toolRegistry.Register(agentpkg.NewLoadSkillTool(skillReg, toolRegistry, false))

	// Short loop: max 5 iterations for read-only analysis
	loop := agentpkg.NewAgentLoopWithConfig(
		provider,
		toolRegistry,
		agent.Model,
		maxOutput,
		maxInput,
		temperature,
		r.factory.defaultLoop,
	)
	loop.SetMaxIterations(5)
	loop.SetModelMeta(r.factory.getModelMeta(agent.Provider, agent.Model))
	loop.SetProviderName(agent.Provider)
	loop.SetUsageRecorder(func(p, m string, usage llm.Usage) {
		r.factory.recordTaskUsage(task.ID, p, m, usage)
	})

	if r.factory.getDebugConfig != nil {
		debugCfg := r.factory.getDebugConfig()
		if debugCfg.ConversationLog.Enabled && r.factory.db != nil {
			loop.SetConversationRecorder(
				newConversationRecorder(r.factory.db, debugCfg.ConversationLog.MaxContentChars),
				task.ID,
			)
		}
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task.Context},
	}

	messages, err = agentpkg.TruncateMessages(messages, toolRegistry.ToLLMTools(), maxInput, r.factory.getModelMeta(agent.Provider, agent.Model))
	if err != nil {
		return nil, fmt.Errorf("truncate messages: %w", err)
	}

	result, err := loop.Run(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("analyze agent loop: %w", err)
	}

	log.Printf("[INFO] Task %d analyze loop completed", task.ID)

	return &Result{
		Content: result,
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
//
// Structure (after A3):
//
//	prepareWriteWorkspace → CodingBackend.Run → finalizeWriteChanges
//
// The coding backend is resolved from agent.Backend (or agents.backends.default).
// Non-write runners (Analyze/Review/Reply) never call this function — they
// always use internal LLM directly, which matches the "Analyze forced internal"
// constraint from server-runtime-design-v4.md §3.2.
func runWriteTask(ctx context.Context, task *store.Task, agentCfg *store.Agent,
	factory *RunnerFactory, taskSubType string) (*Result, error) {

	// Phase 0: resolve backend + optional health probe BEFORE preparing the
	// workspace. Sidecar-down must not leave session clones / branches behind.
	backend, err := factory.ResolveCodingBackend(agentCfg)
	if err != nil {
		return nil, fmt.Errorf("resolve coding backend: %w", err)
	}
	log.Printf("[INFO] Task %d using coding backend: %s", task.ID, backend.Name())

	if hc, ok := backend.(HealthCheckableBackend); ok {
		hcCtx, hcCancel := context.WithTimeout(ctx, 5*time.Second)
		hcErr := hc.HealthCheck(hcCtx)
		hcCancel()
		if hcErr != nil {
			if allowsInternalFallback(backend) {
				log.Printf("[WARN] Task %d coding backend %s unhealthy (%v); allow_fallback_internal=true → switching to internal",
					task.ID, backend.Name(), hcErr)
				backend = factory.internalBackend
			} else {
				// Return error so Executor marks failed (not success) and posts
				// a failure comment via writeFailureToGitea.
				return nil, fmt.Errorf(
					"coding backend %q is not reachable (health check failed): %w",
					backend.Name(), hcErr,
				)
			}
		}
	}

	// Phase 1: prepare workspace (sandbox / clone / branch)
	wwc, err := prepareWriteWorkspace(ctx, task, agentCfg, factory, taskSubType)
	if err != nil {
		return nil, err
	}
	// Only cleanup for non-session workspaces (session workspaces persist)
	if !wwc.UseSession {
		defer wwc.Sandbox.Cleanup()
	}

	sb := wwc.Sandbox

	// Phase 2: coding
	// Build prompts (shared by all backends)
	maxInput := factory.resolveMaxInputTokens(agentCfg.MaxInputTokens, agentCfg.Provider, agentCfg.Model)

	// Load code context for the prompt (best-effort; warn on failure)
	codeCtx, err := agentpkg.LoadCodeContext(sb, maxInput)
	if err != nil {
		log.Printf("[WARN] Failed to load code context: %v", err)
	}

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

	codingReq := CodingRequest{
		WorkDir:        sb.WorkDir,
		Sandbox:        sb,
		Task:           task,
		Agent:          agentCfg,
		TaskSubType:    taskSubType,
		Prompt:         task.Context,
		SystemPrompt:   systemPrompt,
		SessionID:      task.SessionID,
		Continue:       task.SessionID != "",
		BackendOptions: agentCfg.BackendOptions,
		ToolPack:       factory.resolveToolPack(task.TaskType),
	}

	codingResult, err := backend.Run(ctx, codingReq)
	if err != nil {
		return nil, fmt.Errorf("coding backend %s: %w", backend.Name(), err)
	}

	// Phase 3: finalize (commit / push / PR)
	//
	// For the internal backend, codingResult.Provider is the LLM provider
	// used during coding, which we reuse for the commit message LLM call.
	// For opencode backend, Provider is nil (LLM runs server-side), so
	// finalize will look up the provider again from the registry — a minor
	// overhead but keeps the contract simple.
	provider := codingResult.Provider
	if provider == nil {
		provider, _ = factory.llmRegistry.Get(agentCfg.Provider)
	}

	return finalizeWriteChanges(ctx, wwc, task, agentCfg, factory, provider, taskSubType, codingResult.Summary)
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
