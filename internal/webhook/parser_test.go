package webhook

import (
	"testing"
)

func TestParseEventIssues(t *testing.T) {
	payload := []byte(`{
		"action": "assigned",
		"repository": {
			"id": 1,
			"name": "test-repo",
			"full_name": "admin/test-repo",
			"owner": {"id": 1, "login": "admin"}
		},
		"issue": {
			"id": 100,
			"number": 1,
			"title": "Test Issue",
			"body": "Test body",
			"state": "open",
			"user": {"id": 1, "login": "admin"},
			"assignees": [{"id": 2, "login": "ai-agent"}],
			"labels": [{"id": 1, "name": "bug"}]
		},
		"sender": {"id": 1, "login": "admin"}
	}`)

	evt, err := ParseEvent("issues", "delivery-001", payload)
	if err != nil {
		t.Fatalf("ParseEvent failed: %v", err)
	}

	if evt.Event != "issues" {
		t.Errorf("Expected event=issues, got %s", evt.Event)
	}
	if evt.Action != "assigned" {
		t.Errorf("Expected action=assigned, got %s", evt.Action)
	}
	if evt.Repo.FullName != "admin/test-repo" {
		t.Errorf("Expected repo=admin/test-repo, got %s", evt.Repo.FullName)
	}
	if evt.Issue == nil {
		t.Fatal("Expected issue to be non-nil")
	}
	if evt.Issue.Number != 1 {
		t.Errorf("Expected issue number=1, got %d", evt.Issue.Number)
	}
	if evt.DeliveryID != "delivery-001" {
		t.Errorf("Expected deliveryID=delivery-001, got %s", evt.DeliveryID)
	}

	t.Logf("Parsed event: %s/%s repo=%s", evt.Event, evt.Action, evt.Repo.FullName)
}

func TestParseEventPullRequest(t *testing.T) {
	payload := []byte(`{
		"action": "opened",
		"repository": {
			"id": 1,
			"name": "test-repo",
			"full_name": "admin/test-repo",
			"owner": {"id": 1, "login": "admin"}
		},
		"pull_request": {
			"id": 200,
			"number": 10,
			"title": "New Feature",
			"body": "Feature description",
			"state": "open",
			"user": {"id": 1, "login": "admin"},
			"head": {"ref": "feature", "repo": {"full_name": "admin/test-repo"}},
			"base": {"ref": "main", "repo": {"full_name": "admin/test-repo"}}
		},
		"sender": {"id": 1, "login": "admin"}
	}`)

	evt, err := ParseEvent("pull_request", "delivery-002", payload)
	if err != nil {
		t.Fatalf("ParseEvent failed: %v", err)
	}

	if evt.PR == nil {
		t.Fatal("Expected PR to be non-nil")
	}
	if evt.PR.Number != 10 {
		t.Errorf("Expected PR number=10, got %d", evt.PR.Number)
	}

	t.Logf("Parsed PR event: %s/%s", evt.Event, evt.Action)
}

func TestHasLabel(t *testing.T) {
	evt := &WebhookEvent{
		Issue: &Issue{
			Labels: []Label{
				{ID: 1, Name: "bug"},
				{ID: 2, Name: "enhancement"},
			},
		},
	}

	if !evt.HasLabel("bug") {
		t.Error("Expected to find label 'bug'")
	}
	if !evt.HasLabel("enhancement") {
		t.Error("Expected to find label 'enhancement'")
	}
	if evt.HasLabel("nonexistent") {
		t.Error("Should not find label 'nonexistent'")
	}

	// Test with nil issue
	evt2 := &WebhookEvent{}
	if evt2.HasLabel("bug") {
		t.Error("Should return false for nil issue")
	}
}

func TestHasAssignee(t *testing.T) {
	evt := &WebhookEvent{
		Issue: &Issue{
			Assignees: []User{
				{ID: 1, Login: "user1"},
				{ID: 2, Login: "ai-agent"},
			},
		},
	}

	if !evt.HasAssignee("ai-agent") {
		t.Error("Expected to find assignee 'ai-agent'")
	}
	if evt.HasAssignee("nonexistent") {
		t.Error("Should not find assignee 'nonexistent'")
	}
}

func TestHasMention(t *testing.T) {
	evt := &WebhookEvent{
		Comment: &Comment{
			Body: "Hey @ai-agent, please review this",
		},
	}

	if !evt.HasMention("ai-agent") {
		t.Error("Expected to find mention '@ai-agent'")
	}
	if evt.HasMention("nonexistent") {
		t.Error("Should not find mention '@nonexistent'")
	}

	// Test with nil comment
	evt2 := &WebhookEvent{}
	if evt2.HasMention("ai-agent") {
		t.Error("Should return false for nil comment")
	}
}
