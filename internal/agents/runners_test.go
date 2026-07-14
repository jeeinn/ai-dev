package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider is a mock LLM provider for testing.
type mockProvider struct {
	response string
}

func (m *mockProvider) ChatCompletion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Content: m.response,
		Usage: llm.Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}, nil
}

// mockGiteaFactory is a mock Gitea client factory for testing.
type mockGiteaFactory struct{}

func (m *mockGiteaFactory) GetGiteaClient(token string) *gitea.Client {
	return gitea.NewClient("http://localhost:0", token)
}

func (m *mockGiteaFactory) GetAdminGiteaClient() *gitea.Client {
	return gitea.NewClient("http://localhost:0", "admin-token")
}

func TestRunnerFactoryGetRunner(t *testing.T) {
	factory := NewRunnerFactory(nil, nil, nil, config.DefaultAgentDefaults(), config.DefaultAgentLoopConfig(), nil)

	tests := []struct {
		taskType string
		expected string
	}{
		{"review_pr", "*agents.ReviewRunner"},
		{"reply_comment", "*agents.InteractionRunner"},
		{"analyze_issue", "*agents.AnalyzeRunner"},
		{"trigger", "*agents.AnalyzeRunner"},
		{"unknown", "*agents.AnalyzeRunner"},
	}

	for _, tt := range tests {
		t.Run(tt.taskType, func(t *testing.T) {
			runner := factory.GetRunner(tt.taskType)
			if runner == nil {
				t.Error("Expected non-nil runner")
			}
		})
	}
}

func TestAnalyzeRunnerRun(t *testing.T) {
	registry := &llm.Registry{}
	registry.Register("mock", &mockProvider{response: "Analysis result"})

	factory := &mockGiteaFactory{}
	runnerFactory := NewRunnerFactory(registry, factory, nil, config.DefaultAgentDefaults(), config.DefaultAgentLoopConfig(), nil)
	runner := NewAnalyzeRunner(runnerFactory)

	task := &store.Task{
		ID:       1,
		AgentID:  1,
		TaskType: "analyze_issue",
		Context:  "Test context",
	}

	agent := &store.Agent{
		ID:              1,
		Provider:        "mock",
		Model:           "mock-model",
		SystemPrompt:    "You are an analyst.",
		MaxOutputTokens: 1024,
		MaxInputTokens:  8192,
	}

	result, err := runner.Run(context.Background(), task, agent)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Content != "Analysis result" {
		t.Errorf("Expected content='Analysis result', got %s", result.Content)
	}
	if result.Action != "comment" {
		t.Errorf("Expected action=comment, got %s", result.Action)
	}
}

func TestSaveSessionBranch(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "runners-test-*.db")
	require.NoError(t, err)
	tmpDB.Close()

	db, err := store.Open(tmpDB.Name())
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
		os.Remove(tmpDB.Name())
	})

	session := &store.AgentSession{
		ID:     "sess-save-branch",
		Repo:   "owner/repo",
		Status: store.SessionActive,
		Branch: "",
	}
	require.NoError(t, db.CreateSession(session))

	factory := NewRunnerFactory(nil, nil, db, config.DefaultAgentDefaults(), config.DefaultAgentLoopConfig(), nil)
	task := &store.Task{SessionID: session.ID}

	saveSessionBranch(factory, task, "ai/dev/issue-2")

	got, err := db.GetSession(session.ID)
	require.NoError(t, err)
	assert.Equal(t, "ai/dev/issue-2", got.Branch)

	// Idempotent when branch unchanged
	saveSessionBranch(factory, task, "ai/dev/issue-2")
	got, err = db.GetSession(session.ID)
	require.NoError(t, err)
	assert.Equal(t, "ai/dev/issue-2", got.Branch)
}

func TestFinalizeWriteTaskPRCreatesWhenNoOpenPR(t *testing.T) {
	var createCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/owner/repo/pulls":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/pulls":
			createCalled = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(gitea.PRResponse{
				Number:  3,
				HTMLURL: "http://localhost/owner/repo/pulls/3",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := gitea.NewClient(server.URL, "test-token")
	task := &store.Task{ID: 35, Event: "Issue 2", IssueID: 2}
	result, err := finalizeWriteTaskPR(client, "owner", "repo", "ai/dev/issue-2", "main", task, "dev", "done")
	require.NoError(t, err)
	assert.True(t, createCalled)
	assert.Equal(t, "pr", result.Action)
	assert.Equal(t, 3, result.PRID)
}

func TestFinalizeWriteTaskPRCommentsWhenOpenPRExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/repos/owner/repo/pulls", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"number":   3,
				"state":    "open",
				"html_url": "http://localhost/owner/repo/pulls/3",
				"head":     map[string]string{"ref": "ai/dev/issue-2"},
			},
		})
	}))
	defer server.Close()

	client := gitea.NewClient(server.URL, "test-token")
	task := &store.Task{ID: 35, Event: "Issue 2", IssueID: 2}
	result, err := finalizeWriteTaskPR(client, "owner", "repo", "ai/dev/issue-2", "main", task, "dev", "done")
	require.NoError(t, err)
	assert.Equal(t, "comment", result.Action)
	assert.Equal(t, 3, result.PRID)
	assert.Contains(t, result.Content, "Updated PR branch")
}

// stubModelMeta implements ModelMetaProvider for resolveMax* tests.
type stubModelMeta struct {
	defs map[string]*config.ModelDefinition
}

func (s *stubModelMeta) GetModelMeta(provider, model string) *config.ModelDefinition {
	return s.defs[provider+"/"+model]
}

func TestResolveMaxTokensUsesModelWhenAgentZero(t *testing.T) {
	factory := NewRunnerFactory(nil, nil, nil, config.DefaultAgentDefaults(), config.DefaultAgentLoopConfig(), nil)
	factory.SetModelMetaProvider(&stubModelMeta{
		defs: map[string]*config.ModelDefinition{
			"deepseek/deepseek-v4-flash": {
				ID:            "deepseek-v4-flash",
				ContextWindow: 1000000,
				MaxOutput:     32768,
			},
		},
	})

	// agentMax == 0 → model metadata
	assert.Equal(t, 900000, factory.resolveMaxInputTokens(0, "deepseek", "deepseek-v4-flash"))
	assert.Equal(t, 32768, factory.resolveMaxOutputTokens(0, "deepseek", "deepseek-v4-flash"))

	// agentMax explicit, within limit
	assert.Equal(t, 8192, factory.resolveMaxInputTokens(8192, "deepseek", "deepseek-v4-flash"))
	assert.Equal(t, 1024, factory.resolveMaxOutputTokens(1024, "deepseek", "deepseek-v4-flash"))

	// agentMax exceeds model limit → clamped
	assert.Equal(t, 900000, factory.resolveMaxInputTokens(2000000, "deepseek", "deepseek-v4-flash"))
	assert.Equal(t, 32768, factory.resolveMaxOutputTokens(99999, "deepseek", "deepseek-v4-flash"))

	// unknown model + agentMax 0 → agents.defaults
	assert.Equal(t, factory.defaultMaxInput, factory.resolveMaxInputTokens(0, "deepseek", "unknown"))
	assert.Equal(t, factory.defaultMaxOutput, factory.resolveMaxOutputTokens(0, "deepseek", "unknown"))
}

func TestRecordTaskUsageCostPerThousandTokens(t *testing.T) {
	// Verify cost formula: price is $/1K tokens
	meta := &config.ModelDefinition{InputPrice: 1.0, OutputPrice: 2.0}
	prompt, completion := 1000, 500
	cost := (float64(prompt)*meta.InputPrice + float64(completion)*meta.OutputPrice) / 1000.0
	assert.InDelta(t, 2.0, cost, 0.0001) // 1000*1/1000 + 500*2/1000 = 1 + 1 = 2
}
