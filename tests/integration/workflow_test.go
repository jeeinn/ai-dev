package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/store"
)

func TestWebhookIssueAssignedAnalyze(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create analyze agent and enable v2 pipeline
	env.CreateTestAgentWithRole(t, "analyze-007", "analyze-007", store.RoleAnalyze)
	registry := env.EnableWorkflowV2(t)
	_ = registry // suppress unused warning

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Send issues.assigned event
	payload := map[string]interface{}{
		"action":   "assigned",
		"issue":    buildIssuePayload(1, "Test Issue", nil),
		"assignee": map[string]interface{}{"id": 100, "login": "analyze-007"},
		"repository": map[string]interface{}{
			"id": 1, "name": "repo", "full_name": "owner/repo",
			"clone_url": "http://localhost:3000/owner/repo.git",
		},
		"sender": map[string]interface{}{"id": 1, "login": "human"},
	}

	err = env.SendWebhook("issues", "wf-analyze-001", payload)
	require.NoError(t, err)

	// Wait for task to complete
	task := env.WaitForTask(t, 1, "success", 10*time.Second)

	assert.Equal(t, "analyze_issue", task.TaskType)
	assert.Equal(t, store.RoleAnalyze, task.Role)
	assert.Equal(t, int64(1), task.AgentID) // analyze-007's ID

	// Verify workflow context stage
	ctx, err := env.DB.GetWorkflowContext("owner/repo", 1)
	require.NoError(t, err)
	assert.Equal(t, store.StageAnalyzed, ctx.Stage)
}

func TestWebhookIssueAssignedCoder(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	env.CreateTestAgentWithRole(t, "coder-ds", "coder-ds", store.RoleCoder)
	env.EnableWorkflowV2(t)

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	payload := map[string]interface{}{
		"action":   "assigned",
		"issue":    buildIssuePayload(2, "Implement feature", nil),
		"assignee": map[string]interface{}{"id": 200, "login": "coder-ds"},
		"repository": map[string]interface{}{
			"id": 1, "name": "repo", "full_name": "owner/repo",
			"clone_url": "http://localhost:3000/owner/repo.git",
		},
		"sender": map[string]interface{}{"id": 1, "login": "human"},
	}

	err = env.SendWebhook("issues", "wf-coder-001", payload)
	require.NoError(t, err)

	// Wait for task to be created (DevRunner needs real git for completion)
	time.Sleep(2 * time.Second)

	// Verify task was created with correct type and role
	tasks, err := env.DB.ListTasks(10, 0)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	task := tasks[0]
	assert.Equal(t, "solve_issue", task.TaskType)
	assert.Equal(t, store.RoleCoder, task.Role)
	assert.Equal(t, "owner/repo", task.Repo)
}

func TestWebhookAssignUnknownUser(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	env.CreateTestAgentWithRole(t, "analyze-007", "analyze-007", store.RoleAnalyze)
	env.EnableWorkflowV2(t)

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Assign to a non-agent user
	payload := map[string]interface{}{
		"action":   "assigned",
		"issue":    buildIssuePayload(3, "Some issue", nil),
		"assignee": map[string]interface{}{"id": 999, "login": "random-user"},
		"repository": map[string]interface{}{
			"id": 1, "name": "repo", "full_name": "owner/repo",
		},
		"sender": map[string]interface{}{"id": 1, "login": "human"},
	}

	err = env.SendWebhook("issues", "wf-unknown-001", payload)
	require.NoError(t, err)

	// Wait a bit for processing
	time.Sleep(1 * time.Second)

	// No tasks should be created
	tasks, err := env.DB.ListTasks(10, 0)
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
}

func TestWebhookAssignSelfTrigger(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	agent := env.CreateTestAgentWithRole(t, "coder-ds", "coder-ds", store.RoleCoder)
	env.EnableWorkflowV2(t)

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Agent assigns itself — sender is the agent itself
	payload := map[string]interface{}{
		"action":   "assigned",
		"issue":    buildIssuePayload(4, "Self-assigned", nil),
		"assignee": map[string]interface{}{"id": int(agent.ID), "login": "coder-ds"},
		"repository": map[string]interface{}{
			"id": 1, "name": "repo", "full_name": "owner/repo",
		},
		"sender": map[string]interface{}{"id": int(agent.ID), "login": "coder-ds"},
	}

	err = env.SendWebhook("issues", "wf-self-001", payload)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(1 * time.Second)

	// No tasks should be created (self-trigger blocked)
	tasks, err := env.DB.ListTasks(10, 0)
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
}

func TestWebhookReviewRequested(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	env.CreateTestAgentWithRole(t, "reviewer-gpt", "reviewer-gpt", store.RoleReview)
	env.EnableWorkflowV2(t)

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	payload := map[string]interface{}{
		"action": "review_requested",
		"pull_request": map[string]interface{}{
			"id":     10,
			"number": 10,
			"title":  "Fix bug",
			"state":  "open",
			"body":   "Fixes #5",
			"user":   map[string]interface{}{"id": 1, "login": "coder-ds"},
			"head":   map[string]interface{}{"ref": "fix-10", "repo": map[string]interface{}{"full_name": "owner/repo"}},
			"base":   map[string]interface{}{"ref": "main", "repo": map[string]interface{}{"full_name": "owner/repo"}},
			"requested_reviewers": []map[string]interface{}{
				{"id": 400, "login": "reviewer-gpt"},
			},
		},
		"repository": map[string]interface{}{
			"id": 1, "name": "repo", "full_name": "owner/repo",
			"clone_url": "http://localhost:3000/owner/repo.git",
		},
		"sender": map[string]interface{}{"id": 1, "login": "coder-ds"},
	}

	err = env.SendWebhook("pull_request", "wf-review-001", payload)
	require.NoError(t, err)

	task := env.WaitForTask(t, 1, "success", 10*time.Second)

	assert.Equal(t, "review_pr", task.TaskType)
	assert.Equal(t, store.RoleReview, task.Role)
}

func TestL1ReviewNoPR(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	env.CreateTestAgentWithRole(t, "reviewer-gpt", "reviewer-gpt", store.RoleReview)
	env.EnableWorkflowV2(t)

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Assign review agent to an issue (no PR exists)
	payload := map[string]interface{}{
		"action":   "assigned",
		"issue":    buildIssuePayload(20, "Review this", nil),
		"assignee": map[string]interface{}{"id": 400, "login": "reviewer-gpt"},
		"repository": map[string]interface{}{
			"id": 1, "name": "repo", "full_name": "owner/repo",
		},
		"sender": map[string]interface{}{"id": 1, "login": "human"},
	}

	err = env.SendWebhook("issues", "wf-l1-nopr-001", payload)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(2 * time.Second)

	// No task should be created (L1 gate rejects)
	tasks, err := env.DB.ListTasks(10, 0)
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
}

func TestLabeledNoTask(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	env.CreateTestAgentWithRole(t, "analyze-007", "analyze-007", store.RoleAnalyze)
	env.EnableWorkflowV2(t)

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Send labeled event — should be ignored by v2 resolver
	payload := map[string]interface{}{
		"action": "labeled",
		"issue":  buildIssuePayload(30, "Labeled issue", nil),
		"repository": map[string]interface{}{
			"id": 1, "name": "repo", "full_name": "owner/repo",
		},
		"sender": map[string]interface{}{"id": 1, "login": "human"},
	}

	err = env.SendWebhook("issues", "wf-labeled-001", payload)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(1 * time.Second)

	// No tasks should be created
	tasks, err := env.DB.ListTasks(10, 0)
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
}

func TestBugLabelFixBug(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	env.CreateTestAgentWithRole(t, "coder-ds", "coder-ds", store.RoleCoder)
	env.EnableWorkflowV2(t)

	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Assign coder to an issue with "bug" label
	payload := map[string]interface{}{
		"action":   "assigned",
		"issue":    buildIssuePayload(40, "Fix the bug", []map[string]interface{}{{"id": 1, "name": "bug"}}),
		"assignee": map[string]interface{}{"id": 200, "login": "coder-ds"},
		"repository": map[string]interface{}{
			"id": 1, "name": "repo", "full_name": "owner/repo",
			"clone_url": "http://localhost:3000/owner/repo.git",
		},
		"sender": map[string]interface{}{"id": 1, "login": "human"},
	}

	err = env.SendWebhook("issues", "wf-bug-001", payload)
	require.NoError(t, err)

	// Wait for task to be created (don't wait for completion — DevRunner needs real git)
	time.Sleep(2 * time.Second)

	// Verify task was created with fix_bug type
	tasks, err := env.DB.ListTasks(10, 0)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	task := tasks[0]
	assert.Equal(t, "fix_bug", task.TaskType)
	assert.Equal(t, store.RoleCoder, task.Role)
	assert.Equal(t, "owner/repo", task.Repo)
	assert.Equal(t, 40, task.IssueID)
}

// buildIssuePayload is a helper to build issue webhook payloads.
func buildIssuePayload(number int, title string, labels []map[string]interface{}) map[string]interface{} {
	if labels == nil {
		labels = []map[string]interface{}{}
	}
	return map[string]interface{}{
		"id":     number * 100,
		"number": number,
		"title":  title,
		"state":  "open",
		"user":   map[string]interface{}{"id": 1, "login": "admin"},
		"labels": labels,
	}
}
