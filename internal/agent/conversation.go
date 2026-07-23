package agent

import "github.com/jeeinn/matea/internal/llm"

// ConversationRecorder persists agent loop messages when debug logging is enabled.
type ConversationRecorder interface {
	RecordIteration(taskID int64, iteration int, messages []llm.Message, finalAssistant *llm.ChatResponse) error
}
