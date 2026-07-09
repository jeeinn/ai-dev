package agent

import (
	"context"
	"testing"
	"time"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/llm"
)

type countingProvider struct {
	calls int
}

func (p *countingProvider) ChatCompletion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	p.calls++
	if p.calls < 3 {
		return &llm.ChatResponse{
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.FuncCall{
					Name:      "read_file",
					Arguments: `{"path":"README.md"}`,
				},
			}},
			FinishReason: "tool_calls",
		}, nil
	}
	return &llm.ChatResponse{Content: "done", FinishReason: "stop"}, nil
}

func TestAgentLoopIterationInterval(t *testing.T) {
	provider := &countingProvider{}
	registry := NewToolRegistry()
	loop := NewAgentLoopWithConfig(provider, registry, "test-model", 1024, 8192, 0.3, config.AgentLoopConfig{
		MaxIterations:     3,
		IterationInterval: 1,
	})

	start := time.Now()
	_, err := loop.Run(context.Background(), []llm.Message{{Role: "user", Content: "go"}})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if provider.calls != 3 {
		t.Fatalf("expected 3 provider calls, got %d", provider.calls)
	}
	if elapsed < 2*time.Second {
		t.Fatalf("expected at least 2s delay between iterations, got %v", elapsed)
	}
}

type recordingRecorder struct {
	calls []recordCall
}

type recordCall struct {
	iteration int
	messages  int
	hasFinal  bool
}

func (r *recordingRecorder) RecordIteration(taskID int64, iteration int, messages []llm.Message, finalAssistant *llm.ChatResponse) error {
	r.calls = append(r.calls, recordCall{
		iteration: iteration,
		messages:  len(messages),
		hasFinal:  finalAssistant != nil,
	})
	return nil
}

func TestAgentLoopPersistsIterations(t *testing.T) {
	provider := &countingProvider{}
	registry := NewToolRegistry()
	recorder := &recordingRecorder{}
	loop := NewAgentLoopWithConfig(provider, registry, "test-model", 1024, 8192, 0.3, config.AgentLoopConfig{
		MaxIterations: 3,
	})
	loop.SetConversationRecorder(recorder, 42)

	_, err := loop.Run(context.Background(), []llm.Message{{Role: "user", Content: "go"}})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(recorder.calls) != 3 {
		t.Fatalf("expected 3 recorded iterations, got %d", len(recorder.calls))
	}
	if !recorder.calls[2].hasFinal {
		t.Fatalf("expected final iteration to include assistant response")
	}
}
