package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookIssueAssigned(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent and route
	agent := env.CreateTestAgent(t)
	env.CreateTestRoute(t, agent.ID, "issues", "assigned")

	// Start dispatcher
	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Send webhook event
	payload := map[string]interface{}{
		"action": "assigned",
		"repository": map[string]interface{}{
			"id":          1,
			"name":        "repo",
			"full_name":   "owner/repo",
			"clone_url":   "http://localhost:3000/owner/repo.git",
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
		"sender": map[string]interface{}{"id": 1, "login": "user1"},
	}

	err = env.SendWebhook("issues", "test-delivery-001", payload)
	require.NoError(t, err)

	// Wait for task to complete
	task := env.WaitForTask(t, 1, "success", 10*time.Second)

	// Verify task properties
	assert.Equal(t, "issues", task.Event)
	assert.Equal(t, "owner/repo", task.Repo)
	assert.Equal(t, 1, task.IssueID)
	assert.Equal(t, agent.ID, task.AgentID)
	assert.Equal(t, "analyze_issue", task.TaskType)
	assert.NotEmpty(t, task.Result)
}

func TestWebhookDuplicateDelivery(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent and route
	agent := env.CreateTestAgent(t)
	env.CreateTestRoute(t, agent.ID, "issues", "assigned")

	// Start dispatcher
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
		"sender": map[string]interface{}{"id": 1, "login": "user1"},
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

func TestWebhookNoMatchingRoute(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent but no route for "pull_request"
	agent := env.CreateTestAgent(t)
	env.CreateTestRoute(t, agent.ID, "issues", "assigned")

	// Start dispatcher
	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Send PR event (no matching route)
	payload := map[string]interface{}{
		"action": "opened",
		"repository": map[string]interface{}{
			"id":        1,
			"name":      "repo",
			"full_name": "owner/repo",
		},
		"pull_request": map[string]interface{}{
			"id":     1,
			"number": 1,
			"title":  "Test PR",
			"user":   map[string]interface{}{"id": 1, "login": "user1"},
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
