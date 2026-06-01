package dispatcher

import (
	"testing"

	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
)

func TestDetermineTaskType(t *testing.T) {
	tests := []struct {
		name     string
		event    *webhook.WebhookEvent
		expected string
	}{
		{
			name: "issue assigned",
			event: &webhook.WebhookEvent{
				Event:  "issues",
				Action: "assigned",
			},
			expected: "analyze_issue",
		},
		{
			name: "issue labeled",
			event: &webhook.WebhookEvent{
				Event:  "issues",
				Action: "labeled",
			},
			expected: "analyze_issue",
		},
		{
			name: "issue opened",
			event: &webhook.WebhookEvent{
				Event:  "issues",
				Action: "opened",
			},
			expected: "trigger",
		},
		{
			name: "pull request",
			event: &webhook.WebhookEvent{
				Event:  "pull_request",
				Action: "opened",
			},
			expected: "review_pr",
		},
		{
			name: "issue comment",
			event: &webhook.WebhookEvent{
				Event:  "issue_comment",
				Action: "created",
			},
			expected: "reply_comment",
		},
		{
			name: "unknown event",
			event: &webhook.WebhookEvent{
				Event:  "unknown",
				Action: "test",
			},
			expected: "trigger",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineTaskType(tt.event)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestFormatComment(t *testing.T) {
	task := &store.Task{
		ID:       1,
		AgentID:  2,
		TaskType: "analyze_issue",
		Result:   "This is the AI response.",
	}

	comment := formatComment(task)

	if comment == "" {
		t.Error("Expected non-empty comment")
	}
	if !contains(comment, "AI Agent Response") {
		t.Error("Expected comment to contain 'AI Agent Response'")
	}
	if !contains(comment, "This is the AI response.") {
		t.Error("Expected comment to contain the result")
	}
	if !contains(comment, "Task ID: 1") {
		t.Error("Expected comment to contain task ID")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
