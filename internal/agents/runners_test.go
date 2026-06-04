package agents

import (
	"context"
	"testing"

	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"
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

func TestRunnerFactoryGetRunner(t *testing.T) {
	factory := NewRunnerFactory(nil, nil, nil, 4096, 0.3)

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
	runnerFactory := NewRunnerFactory(registry, factory, nil, 4096, 0.3)
	runner := NewAnalyzeRunner(runnerFactory)

	task := &store.Task{
		ID:       1,
		AgentID:  1,
		TaskType: "analyze_issue",
		Context:  "Test context",
	}

	agent := &store.Agent{
		ID:           1,
		Provider:     "mock",
		Model:        "mock-model",
		SystemPrompt: "You are an analyst.",
		MaxTokens:    1024,
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
