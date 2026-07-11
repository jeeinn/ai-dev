package dispatcher

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"time"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/workflow"
)

// GiteaClientFactory creates Gitea clients for result writeback.
type GiteaClientFactory interface {
	GetGiteaClient(token string) *gitea.Client
	GetAdminGiteaClient() *gitea.Client
}

// TaskCompleteCallback is called after a task completes successfully.
type TaskCompleteCallback func(task *store.Task)

// TaskFailedCallback is called after a task fails (all retries exhausted).
type TaskFailedCallback func(task *store.Task)

// Executor runs agent tasks from the queue with concurrency control.
type Executor struct {
	maxConcurrent int
	llmRegistry   *llm.Registry
	db            *store.DB
	sem           chan struct{}
	retryCount    int // task_retry_count: whole-task retries after runner failure
	giteaFactory  GiteaClientFactory
	runnerFactory *agents.RunnerFactory
	agentDefaults config.AgentDefaultsConfig
	defaultLoop   config.AgentLoopConfig
	onComplete    TaskCompleteCallback
	onFailed      TaskFailedCallback

	// rootCtx is cancelled on Shutdown so in-flight agent loops abort promptly.
	rootCtx    context.Context
	rootCancel context.CancelFunc
}

// NewExecutor creates a new Executor.
func NewExecutor(maxConcurrent, retryCount int, llmRegistry *llm.Registry, db *store.DB, agentDefaults config.AgentDefaultsConfig, defaultLoop config.AgentLoopConfig) *Executor {
	if defaultLoop.MaxIterations <= 0 {
		defaultLoop = config.DefaultAgentLoopConfig()
	}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	return &Executor{
		maxConcurrent: maxConcurrent,
		llmRegistry:   llmRegistry,
		db:            db,
		sem:           make(chan struct{}, maxConcurrent),
		retryCount:    retryCount,
		agentDefaults: agentDefaults,
		defaultLoop:   defaultLoop,
		rootCtx:       rootCtx,
		rootCancel:    rootCancel,
	}
}

// Shutdown cancels in-flight task contexts (agent loops / LLM calls observe ctx.Done()).
func (e *Executor) Shutdown() {
	if e.rootCancel != nil {
		e.rootCancel()
	}
}

// SetOnComplete sets the callback for successful task completion.
func (e *Executor) SetOnComplete(cb TaskCompleteCallback) {
	e.onComplete = cb
}

// SetOnFailed sets the callback for failed task completion.
func (e *Executor) SetOnFailed(cb TaskFailedCallback) {
	e.onFailed = cb
}

// SetGiteaClientFactory sets the factory for creating Gitea clients.
func (e *Executor) SetGiteaClientFactory(factory GiteaClientFactory, getDebugConfig func() config.DebugConfig) {
	e.giteaFactory = factory
	e.runnerFactory = agents.NewRunnerFactory(e.llmRegistry, factory, e.db, e.agentDefaults, e.defaultLoop, getDebugConfig)
}

// SetModelMetaProvider sets the model metadata provider for adaptive token limits.
func (e *Executor) SetModelMetaProvider(m agents.ModelMetaProvider) {
	if e.runnerFactory != nil {
		e.runnerFactory.SetModelMetaProvider(m)
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
		e.executeSafely(task)
		<-e.sem // release
	}
}

// executeSafely runs execute and converts panics into a failed task without crashing the worker.
func (e *Executor) executeSafely(task *store.Task) {
	defer func() {
		if r := recover(); r != nil {
			e.handleTaskPanic(task, r)
		}
	}()
	e.execute(task)
}

func (e *Executor) handleTaskPanic(task *store.Task, recovered any) {
	log.Printf("[ERROR] Task %d panicked: %v\n%s", task.ID, recovered, debug.Stack())

	err := fmt.Errorf("task panicked: %v", recovered)
	finished := time.Now()
	task.FinishedAt = &finished
	task.Status = "failed"
	task.Error = err.Error()
	e.db.UpdateTaskStatus(task.ID, "failed", "", task.Error)

	if writeErr := e.writeFailureToGitea(task, err); writeErr != nil {
		log.Printf("[ERROR] Task %d failure writeback failed: %v", task.ID, writeErr)
	}
	if e.onFailed != nil {
		e.onFailed(task)
	}
}

func (e *Executor) execute(task *store.Task) {
	log.Printf("[INFO] Executing task: id=%d agent=%d type=%s", task.ID, task.AgentID, task.TaskType)

	// Mark as running
	now := time.Now()
	task.Status = "running"
	task.StartedAt = &now
	e.db.UpdateTaskStatus(task.ID, "running", "", "")

	// Load agent first so timeout can be resolved per agent/task type
	agent, err := e.db.GetAgent(task.AgentID)
	if err != nil {
		finished := time.Now()
		task.FinishedAt = &finished
		task.Status = "failed"
		task.Error = fmt.Sprintf("load agent: %v", err)
		e.db.UpdateTaskStatus(task.ID, "failed", "", task.Error)
		log.Printf("[ERROR] Task %d failed: %v", task.ID, err)
		if e.onFailed != nil {
			e.onFailed(task)
		}
		return
	}

	timeout := e.resolveTaskTimeout(task.TaskType, agent)
	// Shared across task retries; cancelled on Executor.Shutdown (Ctrl+C / SIGTERM).
	parent := e.rootCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	for attempt := 0; attempt <= e.retryCount; attempt++ {
		if attempt > 0 {
			if ctx.Err() != nil {
				err = fmt.Errorf("task cancelled before retry: %w", ctx.Err())
				break
			}
			log.Printf("[INFO] Retrying whole task %d (task_retry %d/%d)", task.ID, attempt, e.retryCount)
			timer := time.NewTimer(5 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				err = fmt.Errorf("task cancelled during retry backoff: %w", ctx.Err())
			case <-timer.C:
			}
			if ctx.Err() != nil {
				break
			}
		}

		err = e.runTask(ctx, task, agent)
		if err == nil {
			break
		}
		// Do not burn task retries on intentional cancellation / deadline.
		if ctx.Err() != nil {
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
		if writeErr := e.writeFailureToGitea(task, err); writeErr != nil {
			log.Printf("[ERROR] Task %d failure writeback failed: %v", task.ID, writeErr)
		}
		if e.onFailed != nil {
			e.onFailed(task)
		}
	} else {
		task.Status = "success"
		e.db.UpdateTaskStatus(task.ID, "success", task.Result, "")
		log.Printf("[INFO] Task %d completed successfully", task.ID)

		if e.onComplete != nil {
			e.onComplete(task)
		}

		if writeErr := e.writeBackToGitea(task); writeErr != nil {
			log.Printf("[ERROR] Task %d writeback failed: %v", task.ID, writeErr)
		}
	}
}

func (e *Executor) resolveTaskTimeout(taskType string, agent *store.Agent) time.Duration {
	if isLoopTask(taskType) {
		merged := agents.MergeLoopConfig(agent.LoopConfig, e.defaultLoop)
		if d, err := time.ParseDuration(merged.TotalTimeout); err == nil && d > 0 {
			return d
		}
		if d, err := time.ParseDuration(e.defaultLoop.TotalTimeout); err == nil && d > 0 {
			return d
		}
		return 30 * time.Minute
	}

	timeoutStr := agent.Timeout
	if timeoutStr == "" {
		timeoutStr = e.agentDefaults.Timeout
	}
	if d, err := time.ParseDuration(timeoutStr); err == nil && d > 0 {
		return d
	}
	return 5 * time.Minute
}

func isLoopTask(taskType string) bool {
	switch taskType {
	case "solve_issue", "solve_comment", "fix_bug":
		return true
	default:
		return false
	}
}

func (e *Executor) runTask(ctx context.Context, task *store.Task, agent *store.Agent) error {
	runner := e.runnerFactory.GetRunner(task.TaskType)

	result, err := runner.Run(ctx, task, agent)
	if err != nil {
		return fmt.Errorf("runner execution: %w", err)
	}

	task.Result = result.Content
	if result.PRID > 0 {
		task.PRID = result.PRID
		log.Printf("[INFO] Task %d created PR #%d", task.ID, result.PRID)
	}
	log.Printf("[INFO] Task %d completed, action=%s", task.ID, result.Action)
	return nil
}

// writebackTargetID returns the Gitea issue/PR index to post comments on.
// For review_pr with PRID set, comments go on the PR (Gitea uses issue API index).
func writebackTargetID(task *store.Task) (targetID int, ok bool) {
	if task == nil {
		return 0, false
	}
	if task.TaskType == "review_pr" && task.PRID > 0 {
		return task.PRID, true
	}
	if task.IssueID > 0 {
		return task.IssueID, true
	}
	return 0, false
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

	targetID, ok := writebackTargetID(task)
	if !ok {
		log.Printf("[DEBUG] No issue/PR target for task %d, skipping writeback", task.ID)
		return nil
	}

	agent, err := e.db.GetAgent(task.AgentID)
	if err != nil {
		return fmt.Errorf("load agent for writeback: %w", err)
	}

	client := e.giteaFactory.GetGiteaClient(agent.GiteaToken)

	parts := strings.SplitN(task.Repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format: %s", task.Repo)
	}
	owner, repo := parts[0], parts[1]

	commentBody := formatComment(task)

	if err := client.IssueComment(owner, repo, targetID, commentBody); err != nil {
		return fmt.Errorf("post comment: %w", err)
	}

	log.Printf("[INFO] Task %d result written back to %s (target #%d)", task.ID, task.Repo, targetID)
	return nil
}

// writeFailureToGitea posts task failure details to the linked Gitea issue/PR.
func (e *Executor) writeFailureToGitea(task *store.Task, taskErr error) error {
	if e.giteaFactory == nil {
		log.Printf("[DEBUG] No Gitea factory configured, skipping failure writeback for task %d", task.ID)
		return nil
	}
	if taskErr == nil {
		return nil
	}

	targetID, ok := writebackTargetID(task)
	if !ok {
		log.Printf("[DEBUG] No issue/PR target for task %d, skipping failure writeback", task.ID)
		return nil
	}

	agent, err := e.db.GetAgent(task.AgentID)
	if err != nil {
		return fmt.Errorf("load agent for failure writeback: %w", err)
	}

	client := e.giteaFactory.GetGiteaClient(agent.GiteaToken)

	parts := strings.SplitN(task.Repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format: %s", task.Repo)
	}
	owner, repo := parts[0], parts[1]

	commentBody := workflow.FormatAgentComment(formatFailureComment(task, taskErr))
	if err := client.IssueComment(owner, repo, targetID, commentBody); err != nil {
		return fmt.Errorf("post failure comment: %w", err)
	}

	log.Printf("[INFO] Task %d failure written back to %s#%d", task.ID, task.Repo, targetID)
	return nil
}

func formatFailureComment(task *store.Task, taskErr error) string {
	var sb strings.Builder
	sb.WriteString("❌ **任务执行失败**\n\n")
	sb.WriteString("**错误原因：**\n")
	sb.WriteString("```\n")
	sb.WriteString(strings.TrimSpace(taskErr.Error()))
	sb.WriteString("\n```\n\n")
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("*Task ID: %d | Agent: %d | Type: %s*", task.ID, task.AgentID, task.TaskType))
	return sb.String()
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
