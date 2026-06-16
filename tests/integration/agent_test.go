package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/store"
)

func TestAgentCRUD(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent with role
	createReq := map[string]interface{}{
		"name":           "test-agent",
		"gitea_username": "ai-test",
		"provider":       "mock",
		"model":          "mock-model",
		"system_prompt":  "You are a test agent.",
		"role":           "analyze",
	}

	resp, err := env.APIRequest("POST", "/api/agents", createReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]interface{}
	err = parseJSON(resp, &result)
	require.NoError(t, err)

	agent, ok := result["agent"].(map[string]interface{})
	require.True(t, ok, "response should contain 'agent' field")
	assert.Equal(t, "test-agent", agent["name"])
	assert.Equal(t, "ai-test", agent["gitea_username"])
	assert.Equal(t, "analyze", agent["role"])

	// Get agent
	resp, err = env.APIRequest("GET", "/api/agents/1", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// List agents
	resp, err = env.APIRequest("GET", "/api/agents", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var agents []map[string]interface{}
	err = parseJSON(resp, &agents)
	require.NoError(t, err)
	assert.Len(t, agents, 1)

	// Update agent
	updateReq := map[string]interface{}{
		"name":          "updated-agent",
		"system_prompt": "Updated prompt.",
		"role":          "coder",
	}
	resp, err = env.APIRequest("PUT", "/api/agents/1", updateReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Delete agent
	resp, err = env.APIRequest("DELETE", "/api/agents/1", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify deleted
	resp, err = env.APIRequest("GET", "/api/agents/1", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAgentTokenHidden(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent with token
	env.CreateTestAgent(t)

	// Get agent via API
	resp, err := env.APIRequest("GET", "/api/agents/1", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var agent map[string]interface{}
	err = parseJSON(resp, &agent)
	require.NoError(t, err)

	// Token should not be in response
	_, hasToken := agent["gitea_token"]
	assert.False(t, hasToken, "gitea_token should not be in API response")
}

func TestTaskList(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent with role and enable v2
	env.CreateTestAgentWithRole(t, "ai-agent", "ai-agent", store.RoleAnalyze)
	env.EnableWorkflowV2(t)

	// Start dispatcher
	err := env.Dispatcher.Start()
	require.NoError(t, err)

	// Send webhook to create a task (v2 uses assignee)
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

	err = env.SendWebhook("issues", "task-list-test", payload)
	require.NoError(t, err)

	// Wait for task
	env.WaitForTask(t, 1, "success", 10*time.Second)

	// List tasks via API
	resp, err := env.APIRequest("GET", "/api/tasks", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = parseJSON(resp, &result)
	require.NoError(t, err)
	tasks, ok := result["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, tasks, 1)
	task := tasks[0].(map[string]interface{})
	assert.Equal(t, "success", task["status"])
}

func TestTemplatesEndpoint(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Get templates
	resp, err := env.APIRequest("GET", "/api/templates", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Templates endpoint should return valid JSON
	var templates map[string]interface{}
	err = parseJSON(resp, &templates)
	require.NoError(t, err)
	// Templates may be empty map if not configured
	_ = templates
}

func TestStatsEndpoint(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create some data
	env.CreateTestAgent(t)

	// Get stats
	resp, err := env.APIRequest("GET", "/api/stats", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var stats map[string]interface{}
	err = parseJSON(resp, &stats)
	require.NoError(t, err)
	assert.Equal(t, float64(1), stats["total_agents"])
}
