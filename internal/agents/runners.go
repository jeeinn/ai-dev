package agents

import (
	"context"
	"log"

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
