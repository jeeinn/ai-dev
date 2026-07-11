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

func TestIsOllamaURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected bool
	}{
		{"localhost:11434", "http://localhost:11434/v1", true},
		{"127.0.0.1:11434", "http://127.0.0.1:11434/v1", true},
		{"ollama in hostname", "http://ollama.local:11434/v1", true},
		{"deepseek api", "https://api.deepseek.com/v1", false},
		{"openai api", "https://api.openai.com/v1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOllamaURL(tt.baseURL)
			if result != tt.expected {
				t.Errorf("isOllamaURL(%q) = %v, want %v", tt.baseURL, result, tt.expected)
			}
		})
	}
}

func TestDiscoverModelsOpenAICompatible(t *testing.T) {
	_, err := DiscoverModels("test", "", "sk-test", "openai_compatible")
	if err == nil {
		t.Error("Expected error for empty base_url")
	}
	if err.Error() != "base_url not configured" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestDiscoverModelsUnsupportedType(t *testing.T) {
	_, err := DiscoverModels("test", "http://example.com", "sk-test", "anthropic")
	if err == nil {
		t.Error("Expected error for unsupported provider type")
	}
}

type mockProvider struct{}

func (m *mockProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{Content: "mock"}, nil
}
