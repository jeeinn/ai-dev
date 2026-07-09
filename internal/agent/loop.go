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
// Task-level deadline is owned by the Executor context; this loop only
// enforces maxIterations and optional iterationInterval.
type AgentLoop struct {
	provider          llm.Provider
	registry          *ToolRegistry
	model             string
	maxTokens         int // output tokens per completion
	maxInputTokens    int // input budget for messages+tools
	temperature       float64
	maxIterations     int
	iterationInterval time.Duration
}

// NewAgentLoop creates a new AgentLoop with default iteration settings.
func NewAgentLoop(provider llm.Provider, registry *ToolRegistry, model string, maxTokens int, temperature float64) *AgentLoop {
	return &AgentLoop{
		provider:       provider,
		registry:       registry,
		model:          model,
		maxTokens:      maxTokens,
		maxInputTokens: 65536,
		temperature:    temperature,
		maxIterations:  20,
	}
}

// NewAgentLoopWithConfig creates a new AgentLoop with loop configuration.
// maxInputTokens controls TruncateMessages before each ChatCompletion.
func NewAgentLoopWithConfig(provider llm.Provider, registry *ToolRegistry, model string, maxTokens, maxInputTokens int, temperature float64, loopCfg config.AgentLoopConfig) *AgentLoop {
	maxIter := loopCfg.MaxIterations
	if maxIter <= 0 {
		maxIter = 20
	}
	if maxInputTokens <= 0 {
		maxInputTokens = 65536
	}

	iterationInterval := time.Duration(0)
	if loopCfg.IterationInterval > 0 {
		iterationInterval = time.Duration(loopCfg.IterationInterval) * time.Second
	}

	return &AgentLoop{
		provider:          provider,
		registry:          registry,
		model:             model,
		maxTokens:         maxTokens,
		maxInputTokens:    maxInputTokens,
		temperature:       temperature,
		maxIterations:     maxIter,
		iterationInterval: iterationInterval,
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
		if i > 0 && a.iterationInterval > 0 {
			log.Printf("[DEBUG] Agent loop waiting %s before iteration %d/%d",
				a.iterationInterval, i+1, a.maxIterations)
			timer := time.NewTimer(a.iterationInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", fmt.Errorf("agent loop cancelled during iteration delay: %w", ctx.Err())
			case <-timer.C:
			}
		}

		log.Printf("[DEBUG] Agent loop iteration %d/%d", i+1, a.maxIterations)

		trimmed, err := TruncateMessages(messages, tools, a.maxInputTokens)
		if err != nil {
			return "", fmt.Errorf("truncate messages: %w", err)
		}
		messages = trimmed

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

		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, call := range resp.ToolCalls {
			log.Printf("[DEBUG] Executing tool: %s(%s)", call.Function.Name, call.Function.Arguments)

			result, err := a.registry.ExecuteTool(call)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
				log.Printf("[WARN] Tool execution failed: %v", err)
			}

			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: call.ID,
			})
		}
	}

	return "", fmt.Errorf("max iterations (%d) reached", a.maxIterations)
}
