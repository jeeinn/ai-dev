package agent

import "strings"

// LooksLikePseudoToolCall reports whether content looks like a textual dump of
// tool invocations rather than a normal assistant reply. Used to fail closed
// when models put tool calls in message content instead of structured tool_calls.
//
// Known positive patterns (heuristic, not exhaustive):
//   - DeepSeek DSML special-token dumps (dsml + invoke/tool_call)
//   - XML-ish <tool_call>...</tool_call>
//   - invoke name="..." attribute markup
//   - paired <function>...</function> tags
//
// Explicitly NOT recognized today (do not assume coverage):
//   - bare JSON tool payloads in content, e.g. {"name":"read_file","arguments":{...}}
//   - ChatGLM-style tools_call / other vendor-specific schemas without the markers above
//   - Qwen (and similar) dumping OpenAI-shaped tool_call JSON into content only
func LooksLikePseudoToolCall(content string) bool {
	s := strings.TrimSpace(content)
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)

	// DeepSeek DSML / special-token style dumps
	if strings.Contains(lower, "dsml") &&
		(strings.Contains(lower, "invoke") || strings.Contains(lower, "tool_call")) {
		return true
	}
	// Common XML / markup tool-call leaks
	if strings.Contains(lower, "<tool_call") || strings.Contains(lower, "</tool_call>") {
		return true
	}
	if strings.Contains(lower, "invoke name=") {
		return true
	}
	if strings.Contains(lower, "<function") && strings.Contains(lower, "</function>") {
		return true
	}
	return false
}
