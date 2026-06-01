package llm

import (
	"context"
	"testing"
)

func TestMessageStructure(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "Hello, world!",
	}

	if msg.Role != "user" {
		t.Errorf("Expected role=user, got %s", msg.Role)
	}
	if msg.Content != "Hello, world!" {
		t.Errorf("Expected content='Hello, world!', got %s", msg.Content)
	}
}

func TestChatRequestStructure(t *testing.T) {
	req := &ChatRequest{
		Model: "deepseek-chat",
		Messages: []Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello!"},
		},
		MaxTokens:   1024,
		Temperature: 0.7,
	}

	if req.Model != "deepseek-chat" {
		t.Errorf("Expected model=deepseek-chat, got %s", req.Model)
	}
	if len(req.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(req.Messages))
	}
	if req.MaxTokens != 1024 {
		t.Errorf("Expected maxTokens=1024, got %d", req.MaxTokens)
	}
}

func TestChatResponseStructure(t *testing.T) {
	resp := &ChatResponse{
		Content: "Hello! How can I help you?",
		Usage: Usage{
			PromptTokens:     50,
			CompletionTokens: 20,
			TotalTokens:      70,
		},
	}

	if resp.Content != "Hello! How can I help you?" {
		t.Errorf("Unexpected content: %s", resp.Content)
	}
	if resp.Usage.TotalTokens != 70 {
		t.Errorf("Expected totalTokens=70, got %d", resp.Usage.TotalTokens)
	}
}

func TestRegistryRegister(t *testing.T) {
	registry := &Registry{}
	provider := &mockProvider{}

	registry.Register("test", provider)

	p, err := registry.Get("test")
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}
	if p != provider {
		t.Error("Expected to get the same provider instance")
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	registry := &Registry{}

	_, err := registry.Get("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent provider")
	}
}

type mockProvider struct{}

func (m *mockProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{Content: "mock"}, nil
}
