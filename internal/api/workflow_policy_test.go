package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jeeinn/matea/internal/agents"
	"github.com/jeeinn/matea/internal/config"
	"github.com/jeeinn/matea/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWorkflowPolicyAPI(t *testing.T) (*http.ServeMux, *store.DB) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(tmp)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	cfg := &config.Config{
		API:    config.APIConfig{AuthToken: "test-api-token"},
		Agents: config.AgentsConfig{Defaults: config.DefaultAgentDefaults()},
		Gitea:  config.GiteaConfig{URL: "http://localhost", AdminToken: "t"},
	}
	manager := agents.NewManager(db, &cfg.Gitea)
	h := NewHandler(db, manager, cfg, nil, nil, nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, db
}

func apiAuthReq(method, path string, body any) *http.Request {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("Authorization", "Bearer test-api-token")
	return r
}

func TestWorkflowPolicyCRUDWithSlashedRepo(t *testing.T) {
	mux, _ := setupWorkflowPolicyAPI(t)
	repo := "owner/my-repo"

	// PUT with owner/name path (requires {repo...})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, apiAuthReq(http.MethodPut, "/api/workflow-policies/"+repo, map[string]any{
		"preset": "strict",
		"gates":  map[string]string{"stage_transition_unassign": "hard"},
	}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var putResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &putResp))
	assert.Equal(t, repo, putResp["repo"])
	assert.Equal(t, "strict", putResp["preset"])

	// GET single
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, apiAuthReq(http.MethodGet, "/api/workflow-policies/"+repo, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got store.WorkflowPolicyDB
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, repo, got.Repo)
	assert.Equal(t, "strict", got.Preset)
	assert.Contains(t, got.GatesJSON, "stage_transition_unassign")

	// LIST
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, apiAuthReq(http.MethodGet, "/api/workflow-policies", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var list []*store.WorkflowPolicyDB
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, repo, list[0].Repo)

	// DELETE
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, apiAuthReq(http.MethodDelete, "/api/workflow-policies/"+repo, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// GET after delete → 404
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, apiAuthReq(http.MethodGet, "/api/workflow-policies/"+repo, nil))
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestWorkflowPolicyAcceptsAPITokenOnWrite(t *testing.T) {
	mux, _ := setupWorkflowPolicyAPI(t)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, apiAuthReq(http.MethodPut, "/api/workflow-policies/acme/app", map[string]any{
		"preset": "free",
	}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestWorkflowPolicyRejectsUnauthorized(t *testing.T) {
	mux, _ := setupWorkflowPolicyAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/workflow-policies", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWorkflowPolicyGetNotFound(t *testing.T) {
	mux, _ := setupWorkflowPolicyAPI(t)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, apiAuthReq(http.MethodGet, "/api/workflow-policies/missing/repo", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}
