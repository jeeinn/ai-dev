package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/logging"
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
	recorder          ConversationRecorder
	taskID            int64
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

// SetConversationRecorder enables per-iteration conversation persistence for a task.
func (a *AgentLoop) SetConversationRecorder(recorder ConversationRecorder, taskID int64) {
	a.recorder = recorder
	a.taskID = taskID
}

// Run executes the agent loop with the given messages.
// Returns the final assistant message content.
func (a *AgentLoop) Run(ctx context.Context, messages []llm.Message) (string, error) {
	tools := a.registry.ToLLMTools()

	for i := 0; i < a.maxIterations; i++ {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("agent loop cancelled: %w", err)
		}
		if i > 0 && a.iterationInterval > 0 {
			logging.Debugf("Agent loop waiting %s before iteration %d/%d",
				a.iterationInterval, i+1, a.maxIterations)
			timer := time.NewTimer(a.iterationInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", fmt.Errorf("agent loop cancelled during iteration delay: %w", ctx.Err())
			case <-timer.C:
			}
		}

		logging.Debugf("Agent loop iteration %d/%d", i+1, a.maxIterations)

		msgStart := len(messages)

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

		logging.Debugf("LLM response: content_len=%d, tool_calls=%d, finish=%s",
			len(resp.Content), len(resp.ToolCalls), resp.FinishReason)
		if resp.Content != "" {
			logging.Debugf("LLM content preview: %s", truncateForLog(resp.Content, 800))
		}
		for _, call := range resp.ToolCalls {
			logging.Debugf("LLM tool_call: %s(%s)", call.Function.Name, truncateForLog(call.Function.Arguments, 500))
		}

		if len(resp.ToolCalls) == 0 {
			a.persistIteration(i+1, messages[msgStart:], resp)
			return resp.Content, nil
		}

		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, call := range resp.ToolCalls {
			logging.Debugf("Executing tool: %s(%s)", call.Function.Name, truncateForLog(call.Function.Arguments, 500))

			result, err := a.registry.ExecuteTool(call)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
				logging.Warnf("Tool execution failed: %v", err)
			}
			logging.Debugf("Tool result %s: %s", call.Function.Name, truncateForLog(result, 1200))

			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: call.ID,
			})
		}

		a.persistIteration(i+1, messages[msgStart:], nil)
	}

	return "", fmt.Errorf("max iterations (%d) reached", a.maxIterations)
}

func (a *AgentLoop) persistIteration(iteration int, delta []llm.Message, finalAssistant *llm.ChatResponse) {
	if a.recorder == nil || a.taskID <= 0 {
		return
	}
	if len(delta) == 0 && finalAssistant == nil {
		return
	}
	if err := a.recorder.RecordIteration(a.taskID, iteration, delta, finalAssistant); err != nil {
		logging.Warnf("Failed to persist conversation log (task=%d iter=%d): %v", a.taskID, iteration, err)
	}
}

func truncateForLog(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…(truncated)"
}
