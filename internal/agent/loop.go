package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/llm"
)

// AgentLoop manages the multi-turn conversation between LLM and tools.
type AgentLoop struct {
	provider      llm.Provider
	registry      *ToolRegistry
	model         string
	maxTokens     int
	temperature   float64
	maxIterations int
	timeout       time.Duration
	totalTimeout  time.Duration
}

// NewAgentLoop creates a new AgentLoop.
func NewAgentLoop(provider llm.Provider, registry *ToolRegistry, model string, maxTokens int, temperature float64) *AgentLoop {
	return &AgentLoop{
		provider:      provider,
		registry:      registry,
		model:         model,
		maxTokens:     maxTokens,
		temperature:   temperature,
		maxIterations: 20,
		timeout:       5 * time.Minute,
		totalTimeout:  30 * time.Minute,
	}
}

// NewAgentLoopWithConfig creates a new AgentLoop with configuration.
func NewAgentLoopWithConfig(provider llm.Provider, registry *ToolRegistry, model string, maxTokens int, temperature float64, loopCfg config.AgentLoopConfig) *AgentLoop {
	timeout := 5 * time.Minute
	totalTimeout := 30 * time.Minute

	if loopCfg.Timeout != "" {
		if d, err := time.ParseDuration(loopCfg.Timeout); err == nil {
			timeout = d
		}
	}
	if loopCfg.TotalTimeout != "" {
		if d, err := time.ParseDuration(loopCfg.TotalTimeout); err == nil {
			totalTimeout = d
		}
	}

	maxIter := loopCfg.MaxIterations
	if maxIter <= 0 {
		maxIter = 20
	}

	return &AgentLoop{
		provider:      provider,
		registry:      registry,
		model:         model,
		maxTokens:     maxTokens,
		temperature:   temperature,
		maxIterations: maxIter,
		timeout:       timeout,
		totalTimeout:  totalTimeout,
	}
}

// SetMaxIterations sets the maximum number of iterations.
func (a *AgentLoop) SetMaxIterations(n int) {
	a.maxIterations = n
}

// Run executes the agent loop with the given messages.
// Returns the final assistant message content.
func (a *AgentLoop) Run(ctx context.Context, messages []llm.Message) (string, error) {
	tools := a.registry.ToLLMTools()

	for i := 0; i < a.maxIterations; i++ {
		log.Printf("[DEBUG] Agent loop iteration %d/%d", i+1, a.maxIterations)

		// Call LLM
		resp, err := a.provider.ChatCompletion(ctx, &llm.ChatRequest{
			Model:       a.model,
			Messages:    messages,
			Tools:       tools,
			MaxTokens:   a.maxTokens,
			Temperature: a.temperature,
		})
		if err != nil {
			return "", fmt.Errorf("LLM call: %w", err)
		}

		log.Printf("[DEBUG] LLM response: content_len=%d, tool_calls=%d, finish=%s",
			len(resp.Content), len(resp.ToolCalls), resp.FinishReason)

		// If no tool calls, we're done
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// Add assistant message with tool calls
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call
		for _, call := range resp.ToolCalls {
			log.Printf("[DEBUG] Executing tool: %s(%s)", call.Function.Name, call.Function.Arguments)

			result, err := a.registry.ExecuteTool(call)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
				log.Printf("[WARN] Tool execution failed: %v", err)
			}

			// Add tool result message
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: call.ID,
			})
		}
	}

	return "", fmt.Errorf("max iterations (%d) reached", a.maxIterations)
}
