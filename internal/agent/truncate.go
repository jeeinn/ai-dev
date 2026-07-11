package agent

import (
	"encoding/json"
	"fmt"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/llm"
)

// EstimateTokens estimates the token count for a string.
// Rough approximation: 1 token ≈ 4 characters.
func EstimateTokens(s string) int {
	return estimateTokens(s)
}

func messageTokens(m llm.Message) int {
	n := EstimateTokens(m.Content) + EstimateTokens(m.Role) + EstimateTokens(m.ToolCallID)
	for _, tc := range m.ToolCalls {
		n += EstimateTokens(tc.ID) + EstimateTokens(tc.Type)
		n += EstimateTokens(tc.Function.Name) + EstimateTokens(tc.Function.Arguments)
	}
	return n
}

func toolsTokens(tools []llm.Tool) int {
	if len(tools) == 0 {
		return 0
	}
	data, err := json.Marshal(tools)
	if err != nil {
		// Fallback: sum names/descriptions
		n := 0
		for _, t := range tools {
			n += EstimateTokens(t.Type) + EstimateTokens(t.Function.Name) + EstimateTokens(t.Function.Description)
		}
		return n
	}
	return EstimateTokens(string(data))
}

func totalMessageTokens(messages []llm.Message) int {
	n := 0
	for _, m := range messages {
		n += messageTokens(m)
	}
	return n
}

// TruncateMessages trims messages (and accounts for tools) to fit maxInputTokens.
// Trim order: oldest tool results / middle assistant+tool pairs first; then truncate
// last user content; system last. If tools+system alone exceed budget, returns error.
// If modelMeta is provided and the model does not support tools, tool_calls and
// tool messages are stripped from the output, and tools budget is zeroed.
func TruncateMessages(messages []llm.Message, tools []llm.Tool, maxInputTokens int, modelMeta *config.ModelDefinition) ([]llm.Message, error) {
	if maxInputTokens <= 0 {
		return messages, nil
	}

	out := make([]llm.Message, len(messages))
	copy(out, messages)

	supportsTools := true
	if modelMeta != nil {
		supportsTools = modelMeta.SupportsTools
	}

	// If model does not support tools, strip all tool-related content
	if !supportsTools {
		tools = nil
		stripped := make([]llm.Message, 0, len(out))
		for _, m := range out {
			if m.Role == "tool" {
				continue
			}
			if len(m.ToolCalls) > 0 {
				m.ToolCalls = nil
			}
			stripped = append(stripped, m)
		}
		out = stripped
	}

	toolBudget := toolsTokens(tools)
	if toolBudget >= maxInputTokens {
		return nil, fmt.Errorf("tools alone require ~%d tokens, exceed max_input_tokens %d", toolBudget, maxInputTokens)
	}

	budget := maxInputTokens - toolBudget
	if totalMessageTokens(out) <= budget {
		return out, nil
	}

	// 1) Drop oldest middle turns (assistant with tool_calls + following tool results),
	//    keeping the first system message and the trailing user/assistant tail.
	for totalMessageTokens(out) > budget {
		idx := findOldestDroppableTurn(out)
		if idx < 0 {
			break
		}
		out = append(out[:idx], out[idx+1:]...)
	}

	if totalMessageTokens(out) <= budget {
		return out, nil
	}

	// 2) Truncate the last user message content from the end.
	for i := len(out) - 1; i >= 0; i-- {
		if out[i].Role == "user" && out[i].Content != "" {
			out[i].Content = truncateContentToBudget(out, i, budget)
			break
		}
	}

	if totalMessageTokens(out) <= budget {
		return out, nil
	}

	// 3) Truncate system message(s) as last resort.
	for i := range out {
		if out[i].Role == "system" && out[i].Content != "" {
			out[i].Content = truncateContentToBudget(out, i, budget)
		}
	}

	if totalMessageTokens(out) > budget {
		sysTokens := 0
		for _, m := range out {
			if m.Role == "system" {
				sysTokens += messageTokens(m)
			}
		}
		if toolBudget+sysTokens > budget {
			return nil, fmt.Errorf("tools+system require ~%d tokens, exceed remaining budget %d (max_input_tokens %d)",
				toolBudget+sysTokens, budget, maxInputTokens)
		}
		return nil, fmt.Errorf("unable to fit messages into max_input_tokens %d", maxInputTokens)
	}

	return out, nil
}

// findOldestDroppableTurn finds the earliest assistant(tool_calls) or tool message
// that is not the final trailing exchange, preferring to drop from the middle.
func findOldestDroppableTurn(messages []llm.Message) int {
	if len(messages) <= 2 {
		return -1
	}
	// Keep index 0 if system; keep the last message.
	start := 0
	if messages[0].Role == "system" {
		start = 1
	}
	end := len(messages) - 1 // exclusive of last message
	if end <= start {
		return -1
	}

	for i := start; i < end; i++ {
		m := messages[i]
		if m.Role == "tool" {
			return i
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			return i
		}
	}
	// Fall back: drop oldest non-system, non-last message
	for i := start; i < end; i++ {
		return i
	}
	return -1
}

func truncateContentToBudget(messages []llm.Message, idx int, budget int) string {
	other := 0
	for i, m := range messages {
		if i == idx {
			continue
		}
		other += messageTokens(m)
	}
	// Reserve a few tokens for role overhead on this message
	overhead := EstimateTokens(messages[idx].Role) + EstimateTokens(messages[idx].ToolCallID) + 4
	remain := budget - other - overhead
	if remain <= 0 {
		return ""
	}
	// content tokens ≈ len/4 → max chars = remain * 4
	maxChars := remain * 4
	content := messages[idx].Content
	if maxChars <= 0 || len(content) <= maxChars {
		return content
	}
	if maxChars < 20 {
		return content[:maxChars]
	}
	return content[:maxChars-14] + "\n...[truncated]"
}
