package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAgentBackendDefaultsToInternal(t *testing.T) {
	db := newTestDB(t)

	agent := &Agent{
		Name:          "coder-1",
		GiteaUsername: "coder-1",
		GiteaToken:    "tok",
		Provider:      "deepseek",
		Model:         "deepseek-chat",
		Role:          RoleCoder,
		Status:        "active",
		// Backend intentionally left empty
	}
	require.NoError(t, db.CreateAgent(agent))

	got, err := db.GetAgent(agent.ID)
	require.NoError(t, err)
	assert.Equal(t, "internal", got.Backend)
	assert.Nil(t, got.BackendOptions)
}

func TestCreateAgentWithBackendAndOptions(t *testing.T) {
	db := newTestDB(t)

	agent := &Agent{
		Name:           "coder-oc",
		GiteaUsername:  "coder-oc",
		GiteaToken:     "tok",
		Provider:       "deepseek",
		Model:          "deepseek-chat",
		Role:           RoleCoder,
		Status:         "active",
		Backend:        "opencode-local",
		BackendOptions: map[string]any{"opencode_model": "claude-sonnet", "inject_system_prompt": true},
	}
	require.NoError(t, db.CreateAgent(agent))

	got, err := db.GetAgent(agent.ID)
	require.NoError(t, err)
	assert.Equal(t, "opencode-local", got.Backend)
	assert.Equal(t, "claude-sonnet", got.BackendOptions["opencode_model"])
	assert.Equal(t, true, got.BackendOptions["inject_system_prompt"])
}

func TestUpdateAgentBackend(t *testing.T) {
	db := newTestDB(t)

	agent := &Agent{
		Name:          "coder-2",
		GiteaUsername: "coder-2",
		GiteaToken:   "tok",
		Role:          RoleCoder,
		Status:       "active",
	}
	require.NoError(t, db.CreateAgent(agent))

	// Switch to opencode backend
	agent.Backend = "opencode-local"
	agent.BackendOptions = map[string]any{"opencode_session_id": "abc-123"}
	require.NoError(t, db.UpdateAgent(agent))

	got, err := db.GetAgent(agent.ID)
	require.NoError(t, err)
	assert.Equal(t, "opencode-local", got.Backend)
	assert.Equal(t, "abc-123", got.BackendOptions["opencode_session_id"])

	// Switch back to internal (clear options)
	agent.Backend = "internal"
	agent.BackendOptions = nil
	require.NoError(t, db.UpdateAgent(agent))

	got, err = db.GetAgent(agent.ID)
	require.NoError(t, err)
	assert.Equal(t, "internal", got.Backend)
}

func TestListAgentsIncludesBackend(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.CreateAgent(&Agent{
		Name: "a", GiteaUsername: "u-a", GiteaToken: "t", Role: RoleAnalyze, Status: "active",
	}))
	require.NoError(t, db.CreateAgent(&Agent{
		Name: "c", GiteaUsername: "u-c", GiteaToken: "t", Role: RoleCoder, Status: "active",
		Backend: "opencode-local",
	}))

	agents, err := db.ListAgents()
	require.NoError(t, err)
	require.Len(t, agents, 2)

	byName := map[string]*Agent{}
	for _, a := range agents {
		byName[a.Name] = a
	}
	assert.Equal(t, "internal", byName["a"].Backend)
	assert.Equal(t, "opencode-local", byName["c"].Backend)
}
