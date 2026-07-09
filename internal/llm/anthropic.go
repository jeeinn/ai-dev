package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicProvider implements the Anthropic Claude chat completion API.
type AnthropicProvider struct {
	APIKey     string
	HTTPClient *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		APIKey: apiKey,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

type anthropicRequest struct {
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
	System    string         `json:"system,omitempty"`
	Messages  []anthropicMsg `json:"messages"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ChatCompletion sends a chat completion request to the Anthropic API.
func (p *AnthropicProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	var systemMsg string
	var messages []anthropicMsg
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemMsg = m.Content
		} else {
			messages = append(messages, anthropicMsg{Role: m.Role, Content: m.Content})
		}
	}

	body := anthropicRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    systemMsg,
		Messages:  messages,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("x-api-key", p.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var resp anthropicResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("API error: %s", resp.Error.Message)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	return &ChatResponse{
		Content: resp.Content[0].Text,
		Usage: Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}, nil
}
