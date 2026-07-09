package agents

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/store"
)

func newTestManager(t *testing.T, handler http.HandlerFunc) *Manager {
	t.Helper()
	tmpDB, err := os.CreateTemp("", "agent-mgr-test-*.db")
	require.NoError(t, err)
	tmpDB.Close()

	db, err := store.Open(tmpDB.Name())
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
		os.Remove(tmpDB.Name())
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	cfg := &config.GiteaConfig{URL: server.URL, AdminToken: "admin-token"}
	return NewManager(db, cfg)
}

func TestEnsureGiteaAccountCreatesMissingUser(t *testing.T) {
	var created bool
	mgr := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/agent-bot":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/users":
			created = true
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(gitea.UserResponse{ID: 1, Login: "agent-bot"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/tokens"):
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(gitea.TokenResponse{SHA1: "new-token-sha1"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	token, userCreated, err := mgr.EnsureGiteaAccount("agent-bot", "")
	require.NoError(t, err)
	assert.True(t, created)
	assert.True(t, userCreated)
	assert.Equal(t, "new-token-sha1", token)
}

func TestEnsureGiteaAccountKeepsValidToken(t *testing.T) {
	mgr := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/agent-bot":
			json.NewEncoder(w).Encode(gitea.UserResponse{ID: 1, Login: "agent-bot"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/user":
			if r.Header.Get("Authorization") == "token valid-token" {
				json.NewEncoder(w).Encode(gitea.CurrentUser{ID: 1, Login: "agent-bot"})
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	token, userCreated, err := mgr.EnsureGiteaAccount("agent-bot", "valid-token")
	require.NoError(t, err)
	assert.False(t, userCreated)
	assert.Equal(t, "valid-token", token)
}

func TestEnsureGiteaAccountRefreshesInvalidToken(t *testing.T) {
	var passwordReset bool
	mgr := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/agent-bot":
			json.NewEncoder(w).Encode(gitea.UserResponse{ID: 1, Login: "agent-bot"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/user":
			w.WriteHeader(http.StatusUnauthorized)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/admin/users/agent-bot":
			passwordReset = true
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(gitea.UserResponse{ID: 1, Login: "agent-bot"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/tokens"):
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(gitea.TokenResponse{SHA1: "refreshed-token"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	token, userCreated, err := mgr.EnsureGiteaAccount("agent-bot", "stale-localhost-token")
	require.NoError(t, err)
	assert.True(t, passwordReset)
	assert.False(t, userCreated)
	assert.Equal(t, "refreshed-token", token)
}

func TestUpdateAgentProvisionsGiteaUser(t *testing.T) {
	mgr := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/issue-analyze":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/users":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(gitea.UserResponse{ID: 2, Login: "issue-analyze"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/tokens"):
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(gitea.TokenResponse{SHA1: "remote-token"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	agent := &store.Agent{
		Name:          "issue分析",
		GiteaUsername: "issue-analyze",
		GiteaToken:    "old-localhost-token",
		Provider:      "deepseek",
		Model:         "deepseek-v4-flash",
		MaxTokens:     2048,
		Role:          store.RoleAnalyze,
		Status:        "active",
	}
	require.NoError(t, mgr.db.CreateAgent(agent))

	require.NoError(t, mgr.UpdateAgent(agent))

	got, err := mgr.db.GetAgent(agent.ID)
	require.NoError(t, err)
	assert.Equal(t, "remote-token", got.GiteaToken)
}
