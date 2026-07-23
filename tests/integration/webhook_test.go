package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeeinn/matea/internal/store"
)

func TestWebhookIssueAssigned(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent with role and enable v2 pipeline
	env.CreateTestAgentWithRole(t, "ai-agent", "ai-agent", store.RoleAnalyze)
	env.EnableWorkflowV2(t)

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Send webhook event (v2 uses assignee field)
	payload := map[string]interface{}{
		"action": "assigned",
		"repository": map[string]interface{}{
			"id":             1,
			"name":           "repo",
			"full_name":      "owner/repo",
			"clone_url":      "http://localhost:3000/owner/repo.git",
			"default_branch": "main",
		},
		"issue": map[string]interface{}{
			"id":     1,
			"number": 1,
			"title":  "Test Issue",
			"body":   "This is a test issue",
			"state":  "open",
			"user":   map[string]interface{}{"id": 1, "login": "user1"},
		},
		"assignee": map[string]interface{}{"id": 100, "login": "ai-agent"},
		"sender":   map[string]interface{}{"id": 1, "login": "user1"},
	}

	err = env.SendWebhook("issues", "test-delivery-001", payload)
	require.NoError(t, err)

	// Wait for task to complete
	task := env.WaitForTask(t, 1, "success", 10*time.Second)

	// Verify task properties
	assert.Equal(t, "issues", task.Event)
	assert.Equal(t, "owner/repo", task.Repo)
	assert.Equal(t, 1, task.IssueID)
	assert.Equal(t, "analyze_issue", task.TaskType)
	assert.NotEmpty(t, task.Result)
}

func TestWebhookDuplicateDelivery(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	env.CreateTestAgentWithRole(t, "ai-agent", "ai-agent", store.RoleAnalyze)
	env.EnableWorkflowV2(t)

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	payload := map[string]interface{}{
		"action": "assigned",
		"repository": map[string]interface{}{
			"id":        1,
			"name":      "repo",
			"full_name": "owner/repo",
		},
		"issue": map[string]interface{}{
			"id":     1,
			"number": 1,
			"title":  "Test",
			"user":   map[string]interface{}{"id": 1, "login": "user1"},
		},
		"assignee": map[string]interface{}{"id": 100, "login": "ai-agent"},
		"sender":   map[string]interface{}{"id": 1, "login": "user1"},
	}

	// Send same delivery twice
	err = env.SendWebhook("issues", "dup-delivery-001", payload)
	require.NoError(t, err)

	// Wait for first task
	env.WaitForTask(t, 1, "success", 10*time.Second)

	// Send duplicate
	err = env.SendWebhook("issues", "dup-delivery-001", payload)
	require.NoError(t, err) // Should not error, but task should not be created

	// Verify only one task exists
	tasks, err := env.DB.ListTasks(10, 0)
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
}

func TestWebhookNoMatchingAgent(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	env.CreateTestAgentWithRole(t, "ai-agent", "ai-agent", store.RoleAnalyze)
	env.EnableWorkflowV2(t)

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Send PR event with unknown reviewer (no match)
	payload := map[string]interface{}{
		"action": "review_requested",
		"repository": map[string]interface{}{
			"id":        1,
			"name":      "repo",
			"full_name": "owner/repo",
		},
		"pull_request": map[string]interface{}{
			"id":     1,
			"number": 1,
			"title":  "Test PR",
			"state":  "open",
			"user":   map[string]interface{}{"id": 1, "login": "user1"},
			"requested_reviewers": []map[string]interface{}{
				{"id": 999, "login": "unknown-reviewer"},
			},
		},
		"sender": map[string]interface{}{"id": 1, "login": "user1"},
	}

	err = env.SendWebhook("pull_request", "pr-delivery-001", payload)
	require.NoError(t, err)

	// Wait a bit for processing
	time.Sleep(1 * time.Second)

	// Verify no tasks created
	tasks, err := env.DB.ListTasks(10, 0)
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
}
