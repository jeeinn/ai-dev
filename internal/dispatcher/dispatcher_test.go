package dispatcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
	"gitea-agent-gateway/internal/workflow"
)

// mockLLMProvider returns a fixed response for testing.
type mockLLMProvider struct{}

func (m *mockLLMProvider) ChatCompletion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Content: "This is a mock AI response for testing.",
		Usage: llm.Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}, nil
}

// setupTestDB creates a temporary SQLite database for testing.
func setupTestDB(t *testing.T) (*store.DB, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	db, err := store.Open(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to open database: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}

	return db, cleanup
}

// createTestAgent creates a test agent in the database.
func createTestAgent(t *testing.T, db *store.DB) *store.Agent {
	t.Helper()

	agent := &store.Agent{
		Name:          "test-agent",
		GiteaUsername: "ai-agent",
		GiteaToken:    "test-token",
		Provider:      "mock",
		Model:         "mock-model",
		MaxTokens:     1024,
		Temperature:   0.3,
		SystemPrompt:  "You are a helpful AI assistant.",
		Role:          store.RoleAnalyze,
		Status:        "active",
	}

	if err := db.CreateAgent(agent); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	return agent
}

func TestDispatcherHandleEvent(t *testing.T) {
	// Setup
	db, cleanup := setupTestDB(t)
	defer cleanup()

	agent := createTestAgent(t, db)

	// Create mock Gitea server
	giteaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer giteaServer.Close()

	// Create dispatcher
	giteaCfg := &config.GiteaConfig{
		URL: giteaServer.URL,
	}
	dispatcherCfg := &config.DispatcherConfig{
		MaxConcurrent: 1,
		Timeout:       30,
		RetryCount:    0,
		QueueSize:     10,
	}

	llmRegistry := &llm.Registry{}
	llmRegistry.Register("mock", &mockLLMProvider{})

	agentsCfg := &config.AgentsConfig{}
	d := NewDispatcher(db, giteaCfg, dispatcherCfg, llmRegistry, agentsCfg)

	// Wire v2 components
	registry := agents.NewRegistry()
	registry.Refresh(agent)
	resolver := workflow.NewResolver(registry)
	wfMgr := workflow.NewWorkflowManager(db)
	l1Gate := workflow.NewL1Gate(db)
	sessionSvc := workflow.NewSessionService(db, "")
	d.SetWorkflowComponents(registry, resolver, wfMgr, l1Gate, sessionSvc, nil, nil)

	// Create test event (v2 uses assignee field)
	evt := &webhook.WebhookEvent{
		DeliveryID: "test-delivery-001",
		Event:      "issues",
		Action:     "assigned",
		Repo: webhook.Repository{
			FullName: "admin/test-repo",
		},
		Issue: &webhook.Issue{
			Number: 1,
			Title:  "Test Issue",
			Body:   "This is a test issue",
			User:   webhook.User{Login: "admin"},
		},
		Assignee: &webhook.User{Login: "ai-agent"},
		Sender:   webhook.User{Login: "admin"},
	}

	// Test HandleEvent
	result := d.HandleEvent(evt)
	if !result {
		t.Error("HandleEvent returned false, expected true")
	}

	// Verify task was created
	tasks, err := db.ListPendingTasks()
	if err != nil {
		t.Fatalf("Failed to list tasks: %v", err)
	}

	if len(tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(tasks))
	}

	task := tasks[0]
	if task.AgentID != agent.ID {
		t.Errorf("Expected agent_id=%d, got %d", agent.ID, task.AgentID)
	}
	if task.Repo != "admin/test-repo" {
		t.Errorf("Expected repo=admin/test-repo, got %s", task.Repo)
	}
	if task.IssueID != 1 {
		t.Errorf("Expected issue_id=1, got %d", task.IssueID)
	}

	t.Logf("Task created successfully: id=%d, agent=%d, repo=%s",
		task.ID, task.AgentID, task.Repo)
}

func TestDispatcherDuplicateDelivery(t *testing.T) {
	// Setup
	db, cleanup := setupTestDB(t)
	defer cleanup()

	agent := createTestAgent(t, db)

	giteaCfg := &config.GiteaConfig{URL: "http://localhost:0"}
	dispatcherCfg := &config.DispatcherConfig{
		MaxConcurrent: 1,
		Timeout:       30,
		QueueSize:     10,
	}

	d := NewDispatcher(db, giteaCfg, dispatcherCfg, nil, nil)

	// Wire v2 components
	registry := agents.NewRegistry()
	registry.Refresh(agent)
	resolver := workflow.NewResolver(registry)
	d.SetWorkflowComponents(registry, resolver, nil, nil, nil, nil, nil)

	evt := &webhook.WebhookEvent{
		DeliveryID: "test-delivery-dup",
		Event:      "issues",
		Action:     "assigned",
		Repo:       webhook.Repository{FullName: "admin/test-repo"},
		Issue: &webhook.Issue{
			Number: 1,
			Title:  "Test",
			User:   webhook.User{Login: "admin"},
		},
		Assignee: &webhook.User{Login: "ai-agent"},
		Sender:   webhook.User{Login: "admin"},
	}

	// First call should succeed
	if !d.HandleEvent(evt) {
		t.Error("First HandleEvent should succeed")
	}

	// Second call with same delivery should fail (duplicate)
	if d.HandleEvent(evt) {
		t.Error("Second HandleEvent with same delivery should fail")
	}

	t.Logf("Duplicate delivery correctly rejected")
}

func TestTaskQueuePersistence(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create an agent first (foreign key constraint)
	agent := createTestAgent(t, db)

	queue := NewTaskQueue(db, 10)

	// Enqueue a task
	task := &store.Task{
		Event:    "issues",
		Repo:     "test/repo",
		IssueID:  1,
		AgentID:  agent.ID,
		TaskType: "test",
		Context:  "test context",
		Status:   "pending",
	}

	if err := queue.Enqueue(task); err != nil {
		t.Fatalf("Failed to enqueue task: %v", err)
	}

	// Verify task was persisted
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM tasks WHERE status='pending'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count tasks: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 pending task, got %d", count)
	}

	// Test LoadPending
	queue2 := NewTaskQueue(db, 10)
	if err := queue2.LoadPending(); err != nil {
		t.Fatalf("Failed to load pending tasks: %v", err)
	}

	// Verify task is in the channel
	select {
	case loadedTask := <-queue2.Dequeue():
		if loadedTask.ID != task.ID {
			t.Errorf("Loaded task ID %d doesn't match original %d", loadedTask.ID, task.ID)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for task from queue")
	}

	t.Logf("Task persistence and recovery working correctly")
}
