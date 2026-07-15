package agents

import (
	"context"
	"fmt"
	"log"
	"time"

	agentpkg "gitea-agent-gateway/internal/agent"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/sandbox"
	"gitea-agent-gateway/internal/store"
)

// CodingBackend executes the coding phase of a write task on a prepared workspace.
//
// Contract (see server-runtime-design-v4.md §3):
//   - Run is called AFTER prepareWriteWorkspace and BEFORE finalizeWriteChanges.
//   - The backend must not clone, push, or open PRs; it only modifies files in
//     the prepared WorkDir. finalize decides whether to commit based on
//     git.HasChanges().
//   - The returned Provider is reused by finalizeWriteChanges for the
//     commit-message LLM call so the same provider instance is shared across
//     the write task (matches pre-A3 behavior).
type CodingBackend interface {
	Name() string
	Run(ctx context.Context, req CodingRequest) (*CodingResult, error)
	// Abort cancels a running coding session. For internal backend this is a
	// no-op (cancellation is via the ctx passed to Run). For opencode it
	// issues POST /session/:id/abort.
	Abort(ctx context.Context, handle string) error
}

// HealthCheckableBackend is an optional interface for backends that support
// an up-front health probe. When implemented, runWriteTask calls HealthCheck
// before Run and converts failures into a user-friendly comment instead of
// hard-failing the task.
type HealthCheckableBackend interface {
	HealthCheck(ctx context.Context) error
}

// CodingRequest is the input to CodingBackend.Run.
//
// Prompts (Prompt / SystemPrompt) are pre-built by runWriteTask so that all
// backends share the same prompt pipeline (BuildDevPrompt/BuildBugfixPrompt +
// MergeAgentSystemPrompt + code context). Backends just consume them as
// user/system messages (internal) or as the message body (opencode).
type CodingRequest struct {
	// Workspace
	WorkDir string           // absolute path to the prepared repo working tree
	Sandbox *sandbox.Sandbox // sandbox for tool execution / audit (internal backend)

	// Task context
	Task        *store.Task
	Agent       *store.Agent
	TaskSubType string // "dev" | "bugfix"

	// Prompts (pre-built by runWriteTask; backend just consumes)
	Prompt       string // user message: raw task.Context (issue body)
	SystemPrompt string // system message: BuildDevPrompt / BuildBugfixPrompt + MergeAgentSystemPrompt

	// Session
	SessionID string // Gateway session id (for continue semantics)
	Continue  bool   // true if continuing an existing session

	// Limits
	Timeout time.Duration

	// Backend-specific options (Agent.BackendOptions)
	BackendOptions map[string]any
}

// CodingResult is the output of CodingBackend.Run.
type CodingResult struct {
	Summary         string       // coder summary, used as PR body / comment content
	Success         bool         // false → finalize returns a comment with the error
	RemoteSessionID string       // opencode session id (empty for internal)
	Provider        llm.Provider // LLM provider used (reused by finalize for commit message)
}

// InternalCodingBackend wraps the existing AgentLoop + DefaultTools as the
// default coding backend. Used by all non-write tasks (forced) and by write
// tasks whose agent.backend resolves to "internal".
type InternalCodingBackend struct {
	factory *RunnerFactory
}

// NewInternalCodingBackend constructs an InternalCodingBackend bound to a
// RunnerFactory (for LLM registry, token resolution, usage recording, debug).
func NewInternalCodingBackend(factory *RunnerFactory) *InternalCodingBackend {
	return &InternalCodingBackend{factory: factory}
}

// Name returns "internal".
func (b *InternalCodingBackend) Name() string { return "internal" }

// Run executes the AgentLoop with DefaultTools on the prepared workspace.
//
// The prompts (Prompt / SystemPrompt) are pre-built by runWriteTask, so this
// method only handles provider resolution, loop configuration, tool registry,
// and the LLM message loop. Behavior matches the pre-A3.1 inline coding phase:
// identical provider lookup, token resolution, tool registry, loop config merge,
// and recorder wiring.
func (b *InternalCodingBackend) Run(ctx context.Context, req CodingRequest) (*CodingResult, error) {
	factory := b.factory
	agentCfg := req.Agent
	task := req.Task
	sb := req.Sandbox

	provider, err := factory.llmRegistry.Get(agentCfg.Provider)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	maxInput := factory.resolveMaxInputTokens(agentCfg.MaxInputTokens, agentCfg.Provider, agentCfg.Model)
	maxOutput := factory.resolveMaxOutputTokens(agentCfg.MaxOutputTokens, agentCfg.Provider, agentCfg.Model)
	temperature := factory.resolveTemperature(agentCfg.Temperature, agentCfg.Provider, agentCfg.Model)
	mergedLoop := MergeLoopConfig(agentCfg.LoopConfig, factory.defaultLoop)

	toolRegistry := agentpkg.DefaultTools(sb)

	loop := agentpkg.NewAgentLoopWithConfig(
		provider,
		toolRegistry,
		agentCfg.Model,
		maxOutput,
		maxInput,
		temperature,
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

	messages := []llm.Message{
		{Role: "system", Content: req.SystemPrompt},
		{Role: "user", Content: req.Prompt},
	}

	result, err := loop.Run(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}

	log.Printf("[INFO] Task %d internal coding backend completed", task.ID)

	return &CodingResult{
		Summary:  result,
		Success:  true,
		Provider: provider,
	}, nil
}

// Abort is a no-op for the internal backend; cancellation is done via the
// context passed to Run. The handle argument is unused.
func (b *InternalCodingBackend) Abort(ctx context.Context, handle string) error {
	_ = ctx
	_ = handle
	return nil
}

// --- Backend resolution ----------------------------------------------------

// ResolveCodingBackend determines which CodingBackend to use for a write task.
//
// Resolution order (server-runtime-design-v4.md §3.2):
//  1. Non-write tasks → always "internal" (enforced by caller, not here)
//  2. Write tasks: agent.Backend != "" → use that name
//  3. Otherwise → agents.backends.default (default: "internal")
//  4. If the named backend is not found in config → error
//  5. If the backend type is unknown → error
//
// The returned backend is ready to call Run. For opencode_http backends, the
// instance is constructed fresh each call (they hold no state between calls);
// the internal backend is reused (it has no state).
func (f *RunnerFactory) ResolveCodingBackend(agent *store.Agent) (CodingBackend, error) {
	name := agent.Backend
	if name == "" {
		name = f.backends.Default
	}
	if name == "" {
		name = "internal"
	}
	// "internal" is always available, even if missing from config
	if name == "internal" {
		return f.internalBackend, nil
	}

	cfg, ok := f.backends.Backends[name]
	if !ok {
		return nil, fmt.Errorf("coding backend %q not found in agents.backends config", name)
	}

	switch cfg.Type {
	case config.BackendTypeBuiltin:
		return f.internalBackend, nil
	case config.BackendTypeOpenCodeHTTP:
		backend, err := NewOpenCodeHTTPBackend(name, cfg)
		if err != nil {
			return nil, fmt.Errorf("create opencode backend %q: %w", name, err)
		}
		return backend, nil
	default:
		return nil, fmt.Errorf("unsupported coding backend type %q for %q", cfg.Type, name)
	}
}
