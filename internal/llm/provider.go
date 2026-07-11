package llm

import (
	"context"
	"fmt"
	"time"
)

// Provider is the interface for LLM API providers.
type Provider interface {
	ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}

// DiscoverModels discovers available models for a provider by type.
// Returns model IDs and an error.
func DiscoverModels(providerName, baseURL, apiKey, providerType string) ([]string, error) {
	switch providerType {
	case "openai_compatible":
		if baseURL == "" {
			return nil, fmt.Errorf("base_url not configured")
		}
		p := NewOpenAICompatibleProvider(baseURL, apiKey)
		// Use a shorter timeout for discovery
		p.HTTPClient.Timeout = 10 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return p.ListModels(ctx)
	default:
		return nil, fmt.Errorf("dynamic discovery not supported for provider type: %s", providerType)
	}
}

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Tool represents a function tool that the LLM can call.
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function represents a function definition.
type Function struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  Parameters `json:"parameters"`
}

// Parameters represents the parameters schema for a function.
type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property represents a single parameter property.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolCall represents a tool call from the LLM.
type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function FuncCall `json:"function"`
}

// FuncCall represents a function call with name and arguments.
type FuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatRequest is the request payload for chat completion.
type ChatRequest struct {
	Model            string    `json:"model"`
	Messages         []Message `json:"messages"`
	Tools            []Tool    `json:"tools,omitempty"`
	MaxTokens        int       `json:"max_tokens"`
	Temperature      float64   `json:"temperature"`
	TopP             float64   `json:"top_p,omitempty"`
	FrequencyPenalty float64   `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64   `json:"presence_penalty,omitempty"`
}

// ChatResponse is the response from chat completion.
type ChatResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Usage        Usage      `json:"usage"`
}

// Usage represents token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
