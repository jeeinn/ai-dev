package agents

import (
	agentpkg "github.com/jeeinn/matea/internal/agent"
	"github.com/jeeinn/matea/internal/llm"
	"github.com/jeeinn/matea/internal/store"
)

type conversationRecorder struct {
	db              *store.DB
	maxContentChars int
}

func newConversationRecorder(db *store.DB, maxContentChars int) agentpkg.ConversationRecorder {
	return &conversationRecorder{db: db, maxContentChars: maxContentChars}
}

func (r *conversationRecorder) RecordIteration(taskID int64, iteration int, messages []llm.Message, finalAssistant *llm.ChatResponse) error {
	toStore := make([]llm.Message, 0, len(messages)+1)
	toStore = append(toStore, messages...)
	if finalAssistant != nil {
		toStore = append(toStore, llm.Message{
			Role:      "assistant",
			Content:   finalAssistant.Content,
			ToolCalls: finalAssistant.ToolCalls,
		})
	}
	return r.db.AppendConversationMessages(taskID, iteration, toStore, r.maxContentChars)
}
