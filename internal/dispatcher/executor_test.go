package dispatcher

import (
	"errors"
	"strings"
	"testing"

	"gitea-agent-gateway/internal/store"
)

func TestFormatFailureComment(t *testing.T) {
	task := &store.Task{
		ID:       23,
		AgentID:  3,
		TaskType: "analyze_issue",
	}
	err := errors.New(`runner execution: LLM call: API error 404: {"error":{"message":"model is not found"}}`)

	body := formatFailureComment(task, err)

	if !strings.Contains(body, "任务执行失败") {
		t.Fatalf("missing failure title: %s", body)
	}
	if !strings.Contains(body, "model is not found") {
		t.Fatalf("missing error detail: %s", body)
	}
	if !strings.Contains(body, "Task ID: 23") {
		t.Fatalf("missing task metadata: %s", body)
	}
}
