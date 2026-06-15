package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/api"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/dispatcher"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"

	_ "modernc.org/sqlite"
)

// TestEnv contains the test environment.
type TestEnv struct {
	DB           *store.DB
	Config       *config.Config
	Dispatcher   *dispatcher.Dispatcher
	Mux          *http.ServeMux
	Server       *httptest.Server
	GiteaMock    *httptest.Server
	CleanupFuncs []func()
}

// NewTestEnv creates a new test environment.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	// Create temp database
	tmpDB, err := os.CreateTemp("", "test-*.db")
	require.NoError(t, err)
	tmpDB.Close()

	db, err := store.Open(tmpDB.Name())
	require.NoError(t, err)

	// Create mock Gitea server
	giteaMock := newGiteaMock()

	// Create config
	cfg := &config.Config{
		Gitea: config.GiteaConfig{
			URL:        giteaMock.URL,
			AdminToken: "test-admin-token",
		},
		Dispatcher: config.DispatcherConfig{
			MaxConcurrent: 2,
			Timeout:       30,
			RetryCount:    0,
			QueueSize:     10,
		},
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaultsConfig{
				Provider:    "mock",
				Model:       "mock-model",
				MaxTokens:   1024,
				Temperature: 0.3,
			},
		},
		API: config.APIConfig{
			AuthToken: "test-api-token",
		},
	}

	// Create LLM registry with mock provider
	llmRegistry := &llm.Registry{}
	llmRegistry.Register("mock", &mockLLMProvider{response: "Mock AI response"})

	// Create dispatcher
	d := dispatcher.NewDispatcher(db, &cfg.Gitea, &cfg.Dispatcher, llmRegistry, &cfg.Agents)

	// Create API handler
	manager := agents.NewManager(db, &cfg.Gitea)
	apiHandler := api.NewHandler(db, manager, cfg, nil, nil, nil)

	// Create mux
	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux)

	// Create webhook handler
	webhookHandler := webhook.NewHandler(&cfg.Gitea, db.DB, d.HandleEvent)
	mux.Handle("POST /webhook/gitea", webhookHandler)

	// Create test server
	server := httptest.NewServer(mux)

	env := &TestEnv{
		DB:         db,
		Config:     cfg,
		Dispatcher: d,
		Mux:        mux,
		Server:     server,
		GiteaMock:  giteaMock,
		CleanupFuncs: []func(){
			func() { db.Close() },
			func() { os.Remove(tmpDB.Name()) },
			func() { server.Close() },
			func() { giteaMock.Close() },
		},
	}

	return env
}

// Cleanup cleans up the test environment.
func (e *TestEnv) Cleanup() {
	for _, fn := range e.CleanupFuncs {
		fn()
	}
}

// APIURL returns the full API URL for the given path.
func (e *TestEnv) APIURL(path string) string {
	return e.Server.URL + path
}

// APIRequest makes an authenticated API request.
func (e *TestEnv) APIRequest(method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, e.APIURL(path), reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer test-api-token")
	req.Header.Set("Content-Type", "application/json")

	return http.DefaultClient.Do(req)
}

// WebhookURL returns the webhook endpoint URL.
func (e *TestEnv) WebhookURL() string {
	return e.Server.URL + "/webhook/gitea"
}

// SendWebhook sends a webhook event to the test server.
func (e *TestEnv) SendWebhook(event, deliveryID string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", e.WebhookURL(), bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("X-Gitea-Event", event)
	req.Header.Set("X-Gitea-Delivery", deliveryID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CreateTestAgent creates a test agent in the database.
func (e *TestEnv) CreateTestAgent(t *testing.T) *store.Agent {
	t.Helper()

	agent := &store.Agent{
		Name:          "test-agent",
		GiteaUsername: "ai-agent",
		GiteaToken:    "test-gitea-token",
		Provider:      "mock",
		Model:         "mock-model",
		MaxTokens:     1024,
		Temperature:   0.3,
		SystemPrompt:  "You are a helpful AI assistant.",
		Status:        "active",
	}

	err := e.DB.CreateAgent(agent)
	require.NoError(t, err)

	return agent
}

// CreateTestRoute creates a test route in the database.
func (e *TestEnv) CreateTestRoute(t *testing.T, agentID int64, event, action string) {
	t.Helper()

	route := &store.Route{
		Event:    event,
		Action:   action,
		AgentID:  agentID,
		Priority: 10,
	}

	err := e.DB.CreateRoute(route)
	require.NoError(t, err)
}

// WaitForTask waits for a task to reach the given status.
func (e *TestEnv) WaitForTask(t *testing.T, taskID int64, status string, timeout time.Duration) *store.Task {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, err := e.DB.GetTask(taskID)
		if err == nil && task.Status == status {
			return task
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("Task %d did not reach status '%s' within %v", taskID, status, timeout)
	return nil
}

// newGiteaMock creates a mock Gitea API server.
func newGiteaMock() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/v1/version":
			json.NewEncoder(w).Encode(map[string]string{"version": "1.26.0"})

		case r.URL.Path == "/api/v1/repos/owner/repo" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             1,
				"name":           "repo",
				"full_name":      "owner/repo",
				"clone_url":      "http://localhost:3000/owner/repo.git",
				"default_branch": "main",
			})

		case r.URL.Path == "/api/v1/repos/owner/repo/pulls" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":       1,
				"number":   1,
				"title":    "AI PR",
				"html_url": "http://localhost:3000/owner/repo/pulls/1",
			})

		case r.URL.Path == "/api/v1/repos/owner/repo/issues" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     1,
				"number": 1,
				"title":  "Comment",
			})

		default:
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}
	}))
}

// mockLLMProvider is a mock LLM provider for testing.
type mockLLMProvider struct {
	response string
}

func (m *mockLLMProvider) ChatCompletion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Content: m.response,
		Usage: llm.Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}, nil
}

// parseJSON parses a JSON response body.
func parseJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}
