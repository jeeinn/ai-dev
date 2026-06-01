package dispatcher

import (
	"context"
	"fmt"
	"log"
	"time"

	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"
)

// Executor runs agent tasks from the queue with concurrency control.
type Executor struct {
	maxConcurrent int
	timeout       int // seconds
	llmRegistry   *llm.Registry
	db            *store.DB
	sem           chan struct{}
	retryCount    int
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
	}
}

func (e *Executor) runTask(ctx context.Context, task *store.Task) error {
	// Load agent config
	agent, err := e.db.GetAgent(task.AgentID)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}

	// Get LLM provider
	provider, err := e.llmRegistry.Get(agent.Provider)
	if err != nil {
		return fmt.Errorf("get provider: %w", err)
	}

	// Build messages from template
	messages := []llm.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: task.Context},
	}

	// Call LLM
	resp, err := provider.ChatCompletion(ctx, &llm.ChatRequest{
		Model:       agent.Model,
		Messages:    messages,
		MaxTokens:   agent.MaxTokens,
		Temperature: agent.Temperature,
	})
	if err != nil {
		return fmt.Errorf("LLM call: %w", err)
	}

	task.Result = resp.Content
	log.Printf("[INFO] Task %d LLM response: %d tokens used", task.ID, resp.Usage.TotalTokens)
	return nil
}
