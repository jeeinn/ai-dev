package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptCRUD(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent first
	env.CreateTestAgent(t)

	// Create prompt version
	createReq := map[string]interface{}{
		"system_prompt": "You are a test assistant.",
		"user_template": "Test template: {{.Context}}",
		"note":          "Initial version",
	}

	resp, err := env.APIRequest("POST", "/api/agents/1/prompts", createReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var prompt map[string]interface{}
	err = parseJSON(resp, &prompt)
	require.NoError(t, err)

	assert.Equal(t, float64(1), prompt["version"])
	assert.Equal(t, true, prompt["is_active"])

	// List prompts
	resp, err = env.APIRequest("GET", "/api/agents/1/prompts", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var prompts []map[string]interface{}
	err = parseJSON(resp, &prompts)
	require.NoError(t, err)
	assert.Len(t, prompts, 1)

	// Create another version
	createReq2 := map[string]interface{}{
		"system_prompt": "Updated system prompt.",
		"user_template": "Updated template.",
		"note":          "Second version",
	}

	resp, err = env.APIRequest("POST", "/api/agents/1/prompts", createReq2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// List prompts again - should have 2
	resp, err = env.APIRequest("GET", "/api/agents/1/prompts", nil)
	require.NoError(t, err)
	err = parseJSON(resp, &prompts)
	require.NoError(t, err)
	assert.Len(t, prompts, 2)

	// Get active prompt
	resp, err = env.APIRequest("GET", "/api/agents/1/prompts/active", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var activePrompt map[string]interface{}
	err = parseJSON(resp, &activePrompt)
	require.NoError(t, err)
	assert.Equal(t, "Updated system prompt.", activePrompt["system_prompt"])

	// Rollback to first version
	resp, err = env.APIRequest("POST", "/api/prompts/1/activate", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify rollback
	resp, err = env.APIRequest("GET", "/api/agents/1/prompts/active", nil)
	require.NoError(t, err)
	err = parseJSON(resp, &activePrompt)
	require.NoError(t, err)
	assert.Equal(t, "You are a test assistant.", activePrompt["system_prompt"])

	// Delete prompt
	resp, err = env.APIRequest("DELETE", "/api/prompts/2", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify deletion
	resp, err = env.APIRequest("GET", "/api/agents/1/prompts", nil)
	require.NoError(t, err)
	err = parseJSON(resp, &prompts)
	require.NoError(t, err)
	assert.Len(t, prompts, 1)
}

func TestPromptValidation(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent
	env.CreateTestAgent(t)

	// Try to create prompt without system_prompt
	createReq := map[string]interface{}{
		"user_template": "Test template",
	}

	resp, err := env.APIRequest("POST", "/api/agents/1/prompts", createReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Try to get prompts for non-existent agent
	resp, err = env.APIRequest("GET", "/api/agents/999/prompts", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode) // Should return empty list

	// Try to activate non-existent prompt
	resp, err = env.APIRequest("POST", "/api/prompts/999/activate", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestPromptWithAgentCreation(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent
	env.CreateTestAgent(t)

	// Get active prompt - should use agent's system_prompt
	resp, err := env.APIRequest("GET", "/api/agents/1/prompts/active", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var activePrompt map[string]interface{}
	err = parseJSON(resp, &activePrompt)
	require.NoError(t, err)

	// Should have agent's system prompt
	assert.NotEmpty(t, activePrompt["system_prompt"])
}

func TestPromptTemplates(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Get available templates
	resp, err := env.APIRequest("GET", "/api/templates", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Response should be valid JSON (even if empty)
	var templates map[string]interface{}
	err = parseJSON(resp, &templates)
	require.NoError(t, err)
}

func TestPromptVersionOrdering(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup()

	// Create agent
	env.CreateTestAgent(t)

	// Create multiple versions
	for i := 1; i <= 3; i++ {
		createReq := map[string]interface{}{
			"system_prompt": "Version " + string(rune('0'+i)),
			"user_template": "Template " + string(rune('0'+i)),
			"note":          "Version note",
		}

		resp, err := env.APIRequest("POST", "/api/agents/1/prompts", createReq)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// List prompts - should be ordered by version DESC
	resp, err := env.APIRequest("GET", "/api/agents/1/prompts", nil)
	require.NoError(t, err)

	var prompts []map[string]interface{}
	err = parseJSON(resp, &prompts)
	require.NoError(t, err)

	// Should have 3 prompts
	assert.Len(t, prompts, 3)

	// First should be version 3 (newest)
	assert.Equal(t, float64(3), prompts[0]["version"])
	assert.Equal(t, true, prompts[0]["is_active"])

	// Last should be version 1 (oldest)
	assert.Equal(t, float64(1), prompts[2]["version"])
	assert.Equal(t, false, prompts[2]["is_active"])
}
