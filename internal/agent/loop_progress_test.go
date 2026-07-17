package agent

import (
	"context"
	"testing"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stallProvider returns tool calls for the first n rounds, then a final message.
type stallProvider struct {
	toolRounds int
	calls      int
}

func (p *stallProvider) Name() string { return "stall" }

func (p *stallProvider) ChatCompletion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	p.calls++
	if p.calls <= p.toolRounds {
		return &llm.ChatResponse{
			Content: "",
			ToolCalls: []llm.ToolCall{{
				ID:   "c1",
				Type: "function",
				Function: llm.FuncCall{
					Name:      "noop",
					Arguments: `{}`,
				},
			}},
			FinishReason: "tool_calls",
		}, nil
	}
	return &llm.ChatResponse{Content: "done", FinishReason: "stop"}, nil
}

func registerNoop(reg *ToolRegistry) {
	reg.Register(&ToolDef{
		Name:        "noop",
		Description: "noop",
		Parameters: llm.Parameters{
			Type:       "object",
			Properties: map[string]llm.Property{},
		},
		Fn: func(params map[string]interface{}) (string, error) { return "ok", nil },
	})
}

func TestAgentLoopNoProgressExits(t *testing.T) {
	reg := NewToolRegistry()
	registerNoop(reg)

	loop := NewAgentLoopWithConfig(
		&stallProvider{toolRounds: 10},
		reg,
		"m",
		1024,
		8192,
		0.2,
		config.AgentLoopConfig{MaxIterations: 10, TotalTimeout: "30m"},
	)
	// Fingerprint never changes → stall
	loop.SetNoProgressGuard(3, func() string { return "same" })

	_, err := loop.Run(context.Background(), []llm.Message{{Role: "user", Content: "go"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no progress")
}

func TestAgentLoopNoProgressResetsOnChange(t *testing.T) {
	reg := NewToolRegistry()
	registerNoop(reg)

	n := 0
	loop := NewAgentLoopWithConfig(
		&stallProvider{toolRounds: 4},
		reg,
		"m",
		1024,
		8192,
		0.2,
		config.AgentLoopConfig{MaxIterations: 10, TotalTimeout: "30m"},
	)
	loop.SetNoProgressGuard(3, func() string {
		n++
		// Change every round so stall never trips
		return string(rune('a' + n))
	})

	out, err := loop.Run(context.Background(), []llm.Message{{Role: "user", Content: "go"}})
	require.NoError(t, err)
	assert.Equal(t, "done", out)
}

func TestAgentLoopNoProgressDisabled(t *testing.T) {
	reg := NewToolRegistry()
	registerNoop(reg)

	loop := NewAgentLoopWithConfig(
		&stallProvider{toolRounds: 2},
		reg,
		"m",
		1024,
		8192,
		0.2,
		config.AgentLoopConfig{MaxIterations: 10, TotalTimeout: "30m"},
	)
	// limit 0 → disabled even with snap
	loop.SetNoProgressGuard(0, func() string { return "same" })

	out, err := loop.Run(context.Background(), []llm.Message{{Role: "user", Content: "go"}})
	require.NoError(t, err)
	assert.Equal(t, "done", out)
}
