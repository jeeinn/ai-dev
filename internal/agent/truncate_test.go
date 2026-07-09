package agent

import (
	"strings"
	"testing"

	"gitea-agent-gateway/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEstimateTokens(t *testing.T) {
	assert.Equal(t, 0, EstimateTokens(""))
	assert.Equal(t, 2, EstimateTokens("abcdabcd")) // 8/4
}

func TestTruncateMessagesNoOpWhenUnderBudget(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
	}
	out, err := TruncateMessages(msgs, nil, 10000)
	require.NoError(t, err)
	assert.Equal(t, msgs, out)
}

func TestTruncateMessagesDropsOldestToolTurns(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "start"},
		{Role: "assistant", Content: "thinking", ToolCalls: []llm.ToolCall{{
			ID: "c1", Type: "function", Function: llm.FuncCall{Name: "read_file", Arguments: `{"path":"a"}`},
		}}},
		{Role: "tool", Content: strings.Repeat("x", 4000), ToolCallID: "c1"},
		{Role: "assistant", Content: "more", ToolCalls: []llm.ToolCall{{
			ID: "c2", Type: "function", Function: llm.FuncCall{Name: "read_file", Arguments: `{"path":"b"}`},
		}}},
		{Role: "tool", Content: strings.Repeat("y", 4000), ToolCallID: "c2"},
		{Role: "user", Content: "continue"},
	}

	// Tight budget forces dropping middle tool turns
	out, err := TruncateMessages(msgs, nil, 200)
	require.NoError(t, err)
	assert.Equal(t, "system", out[0].Role)
	assert.Equal(t, "user", out[len(out)-1].Role)
	for _, m := range out {
		assert.NotEqual(t, "tool", m.Role, "expected tool messages to be dropped first")
	}
}

func TestTruncateMessagesToolsPlusSystemExceedBudget(t *testing.T) {
	tools := []llm.Tool{{
		Type: "function",
		Function: llm.Function{
			Name:        "read_file",
			Description: strings.Repeat("d", 400),
			Parameters:  llm.Parameters{Type: "object"},
		},
	}}
	msgs := []llm.Message{
		{Role: "system", Content: strings.Repeat("s", 400)},
		{Role: "user", Content: "hi"},
	}
	_, err := TruncateMessages(msgs, tools, 50)
	require.Error(t, err)
}

func TestTruncateMessagesTruncatesUserContent(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: strings.Repeat("u", 4000)},
	}
	out, err := TruncateMessages(msgs, nil, 100)
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Less(t, len(out[1].Content), 4000)
	assert.Contains(t, out[1].Content, "[truncated]")
}
