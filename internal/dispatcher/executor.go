package dispatcher

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"
)

// GiteaClientFactory creates Gitea clients for result writeback.
type GiteaClientFactory interface {
	GetGiteaClient(token string) *gitea.Client
}

// Executor runs agent tasks from the queue with concurrency control.
type Executor struct {
	maxConcurrent int
	timeout       int // seconds
	llmRegistry   *llm.Registry
	db            *store.DB
	sem           chan struct{}
	retryCount    int
	giteaFactory  GiteaClientFactory
	runnerFactory *agents.RunnerFactory
}

// NewExecutor creates a new Executor.
func NewExecutor(maxConcurrent, timeout, retryCount int, llmRegistry *llm.Registry, db *store.DB) *Executor {
	return &Executor{
		maxConcurrent: maxConcurrent,
		timeout:       timeout,
		llmRegistry:   llmRegistry,
		db:            db,
		sem:           make(chan struct{}, maxConcurrent),
		retryCount:    retryCount,
	}
}

// SetGiteaClientFactory sets the factory for creating Gitea clients.
func (e *Executor) SetGiteaClientFactory(factory GiteaClientFactory) {
	e.giteaFactory = factory
	e.runnerFactory = agents.NewRunnerFactory(e.llmRegistry, factory, e.db)
}

// Start begins the executor workers.
func (e *Executor) Start(queue *TaskQueue) {
	for i := 0; i < e.maxConcurrent; i++ {
		go e.worker(queue)
	}
	log.Printf("[INFO] Executor started with %d workers", e.maxConcurrent)
}

func (e *Executor) worker(queue *TaskQueue) {
	for task := range queue.Dequeue() {
		e.sem <- struct{}{} // acquire
		e.execute(task)
		<-e.sem // release
	}
}

func (e *Executor) execute(task *store.Task) {
	log.Printf("[INFO] Executing task: id=%d agent=%d type=%s", task.ID, task.AgentID, task.TaskType)

	// Mark as running
	now := time.Now()
	task.Status = "running"
	task.StartedAt = &now
	e.db.UpdateTaskStatus(task.ID, "running", "", "")

	// Execute with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.timeout)*time.Second)
	defer cancel()

	var err error
	for attempt := 0; attempt <= e.retryCount; attempt++ {
		if attempt > 0 {
			log.Printf("[INFO] Retrying task %d (attempt %d/%d)", task.ID, attempt, e.retryCount)
			time.Sleep(5 * time.Second)
		}

		err = e.runTask(ctx, task)
		if err == nil {
			break
		}
	}

	// Update final status
	finished := time.Now()
	task.FinishedAt = &finished
	if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
		e.db.UpdateTaskStatus(task.ID, "failed", "", err.Error())
		log.Printf("[ERROR] Task %d failed: %v", task.ID, err)
	} else {
		task.Status = "success"
		e.db.UpdateTaskStatus(task.ID, "success", task.Result, "")
		log.Printf("[INFO] Task %d completed successfully", task.ID)

		// Write back result to Gitea
		if writeErr := e.writeBackToGitea(task); writeErr != nil {
			log.Printf("[ERROR] Task %d writeback failed: %v", task.ID, writeErr)
		}
	}
}

func (e *Executor) runTask(ctx context.Context, task *store.Task) error {
	// Load agent config
	agent, err := e.db.GetAgent(task.AgentID)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}

	// Get the appropriate runner for this task type
	runner := e.runnerFactory.GetRunner(task.TaskType)

	// Execute the task
	result, err := runner.Run(ctx, task, agent)
	if err != nil {
		return fmt.Errorf("runner execution: %w", err)
	}

	task.Result = result.Content
	log.Printf("[INFO] Task %d completed, action=%s", task.ID, result.Action)
	return nil
}

// writeBackToGitea posts the LLM result as a comment on the Gitea issue/PR.
func (e *Executor) writeBackToGitea(task *store.Task) error {
	if e.giteaFactory == nil {
		log.Printf("[DEBUG] No Gitea factory configured, skipping writeback for task %d", task.ID)
		return nil
	}

	if task.Result == "" {
		log.Printf("[DEBUG] No result to write back for task %d", task.ID)
		return nil
	}

	if task.IssueID == 0 {
		log.Printf("[DEBUG] No issue ID for task %d, skipping writeback", task.ID)
		return nil
	}

	// Load agent to get its Gitea token
	agent, err := e.db.GetAgent(task.AgentID)
	if err != nil {
		return fmt.Errorf("load agent for writeback: %w", err)
	}

	// Create Gitea client with agent's token
	client := e.giteaFactory.GetGiteaClient(agent.GiteaToken)

	// Parse repo owner/name
	parts := strings.SplitN(task.Repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format: %s", task.Repo)
	}
	owner, repo := parts[0], parts[1]

	// Format the comment body
	commentBody := formatComment(task)

	// Post comment
	if err := client.IssueComment(owner, repo, task.IssueID, commentBody); err != nil {
		return fmt.Errorf("post comment: %w", err)
	}

	log.Printf("[INFO] Task %d result written back to %s#%d", task.ID, task.Repo, task.IssueID)
	return nil
}

// formatComment formats the LLM result as a Gitea comment.
func formatComment(task *store.Task) string {
	var sb strings.Builder

	sb.WriteString("🤖 **AI Agent Response**\n\n")
	sb.WriteString(task.Result)
	sb.WriteString("\n\n---\n")
	sb.WriteString(fmt.Sprintf("*Task ID: %d | Agent: %d | Type: %s*", task.ID, task.AgentID, task.TaskType))

	return sb.String()
}
