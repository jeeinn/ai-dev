package agents

import (
	"context"
	"testing"
	"time"

	"github.com/jeeinn/matea/internal/config"
	"github.com/jeeinn/matea/internal/llm"
	"github.com/jeeinn/matea/internal/sandbox"
	"github.com/jeeinn/matea/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface compliance check.
var _ CodingBackend = (*InternalCodingBackend)(nil)

// mockLLMProvider is a minimal llm.Provider used by InternalCodingBackend tests.
// It returns a fixed assistant message with no tool calls, so AgentLoop.Run
// terminates after a single iteration.
type mockLLMProvider struct {
	content string
	usage   llm.Usage
}

func (m *mockLLMProvider) ChatCompletion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Content:      m.content,
		FinishReason: "stop",
		Usage:        m.usage,
	}, nil
}

func newInternalTestFactory(t *testing.T, providerName string, provider llm.Provider) *RunnerFactory {
	t.Helper()
	registry := llm.NewRegistry(nil)
	if provider != nil {
		registry.Register(providerName, provider)
	}
	factory := NewRunnerFactory(registry, nil, nil, config.DefaultAgentDefaults(), config.DefaultAgentLoopConfig(), nil, nil, nil, sandbox.DefaultConfig(), nil, "")
	return factory
}

func TestInternalCodingBackendName(t *testing.T) {
	b := NewInternalCodingBackend(newInternalTestFactory(t, "mock", nil))
	assert.Equal(t, "internal", b.Name())
}

func TestInternalCodingBackendAbort(t *testing.T) {
	b := NewInternalCodingBackend(newInternalTestFactory(t, "mock", nil))
	// Abort is a no-op for the internal backend; must not error and must not
	// depend on the handle argument.
	err := b.Abort(context.Background(), "any-handle")
	require.NoError(t, err)
}

// TestInternalCodingBackendRunNoProvider verifies that Run surfaces a
// provider-not-found error from the registry rather than panicking.
func TestInternalCodingBackendRunNoProvider(t *testing.T) {
	factory := newInternalTestFactory(t, "mock", nil) // no provider registered under "missing"
	b := NewInternalCodingBackend(factory)

	sb := newMinimalSandbox(t)
	_, err := b.Run(context.Background(), CodingRequest{
		WorkDir:      sb.WorkDir,
		Sandbox:      sb,
		Task:         &store.Task{ID: 1, Repo: "owner/repo"},
		Agent:        &store.Agent{Provider: "missing", Model: "m"},
		Prompt:       "user prompt",
		SystemPrompt: "system prompt",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
}

// TestInternalCodingBackendRunSuccess exercises the happy path end-to-end:
// a registered mock provider returns a non-empty content with no tool calls,
// AgentLoop.Run terminates after one iteration, and Run returns a CodingResult
// carrying the summary and the provider instance for reuse by finalize.
func TestInternalCodingBackendRunSuccess(t *testing.T) {
	mock := &mockLLMProvider{
		content: "Implemented the requested change.",
		usage:   llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
	factory := newInternalTestFactory(t, "mock", mock)
	b := NewInternalCodingBackend(factory)

	sb := newMinimalSandbox(t)
	task := &store.Task{ID: 42, Repo: "owner/repo"}
	agent := &store.Agent{Provider: "mock", Model: "mock-model"}

	result, err := b.Run(context.Background(), CodingRequest{
		WorkDir:      sb.WorkDir,
		Sandbox:      sb,
		Task:         task,
		Agent:        agent,
		Prompt:       "Fix the bug described in the issue body.",
		SystemPrompt: "You are a senior software engineer.",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "Implemented the requested change.", result.Summary)
	assert.Empty(t, result.RemoteSessionID, "internal backend must not set a remote session id")
	assert.NotNil(t, result.Provider, "Provider must be returned for reuse by finalize")
}

// newMinimalSandbox builds a throwaway sandbox with a tiny initial commit so
// that LoadCodeContext / DefaultTools have a valid working tree to reference.
// The sandbox is cleaned up automatically via t.Cleanup.
func newMinimalSandbox(t *testing.T) *sandbox.Sandbox {
	t.Helper()
	cfg := sandbox.SandboxConfig{
		Mode:           sandbox.ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 30 * time.Second,
		MaxOutput:      4096,
		MaxFileSize:    4096,
	}
	s := sandbox.New(cfg, 9901)
	require.NoError(t, s.Setup())
	t.Cleanup(func() { s.Cleanup() })

	// Initialise an empty git repo so DefaultTools has a valid work tree.
	s.Execute("git", "init")
	s.Execute("git", "config", "user.email", "test@test.com")
	s.Execute("git", "config", "user.name", "Test")
	require.NoError(t, s.WriteFile("README.md", []byte("hello")))
	git := sandbox.NewGit(s)
	require.NoError(t, git.Add().Error)
	require.NoError(t, git.Commit("initial").Error)
	return s
}
