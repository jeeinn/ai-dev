package dispatcher

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
)

// Dispatcher orchestrates the event processing pipeline:
// WebhookEvent → Router.Match → TaskQueue.Enqueue → Executor.execute
type Dispatcher struct {
	router    *Router
	queue     *TaskQueue
	executor  *Executor
	db        *store.DB
	giteaCfg  *config.GiteaConfig
	agentsCfg *config.AgentsConfig
}

// NewDispatcher creates a new Dispatcher with all components wired together.
func NewDispatcher(
	db *store.DB,
	giteaCfg *config.GiteaConfig,
	dispatcherCfg *config.DispatcherConfig,
	llmRegistry *llm.Registry,
	agentsCfg *config.AgentsConfig,
) *Dispatcher {
	router := NewRouter(db)
	queue := NewTaskQueue(db, dispatcherCfg.QueueSize)
	defaultMaxTokens := 4096
	defaultTemp := 0.3
	if agentsCfg != nil {
		if agentsCfg.Defaults.MaxTokens > 0 {
			defaultMaxTokens = agentsCfg.Defaults.MaxTokens
		}
		if agentsCfg.Defaults.Temperature > 0 {
			defaultTemp = agentsCfg.Defaults.Temperature
		}
	}
	executor := NewExecutor(
		dispatcherCfg.MaxConcurrent,
		dispatcherCfg.Timeout,
		dispatcherCfg.RetryCount,
		llmRegistry,
		db,
		defaultMaxTokens,
		defaultTemp,
	)

	d := &Dispatcher{
		router:    router,
		queue:     queue,
		executor:  executor,
		db:        db,
		giteaCfg:  giteaCfg,
		agentsCfg: agentsCfg,
	}

	// Wire up Gitea client factory for result writeback
	executor.SetGiteaClientFactory(d)

	return d
}

// Start initializes the executor workers, loads pending tasks, and starts the queue scanner.
func (d *Dispatcher) Start() error {
	// Load pending tasks from DB before starting workers
	if err := d.queue.LoadPending(); err != nil {
		return fmt.Errorf("load pending tasks: %w", err)
	}

	// Start executor workers
	d.executor.Start(d.queue)

	// Start queue scanner (scan every 60s, reset stale tasks after 10min)
	d.queue.StartScanner(60*time.Second, 10*time.Minute)

	log.Printf("[INFO] Dispatcher started")
	return nil
}

// HandleEvent processes a webhook event through the pipeline.
// This is the callback function passed to webhook.Handler.
// Returns true if the event was successfully processed (task enqueued).
func (d *Dispatcher) HandleEvent(evt *webhook.WebhookEvent) bool {
	log.Printf("[INFO] Processing event: %s/%s repo=%s sender=%s",
		evt.Event, evt.Action, evt.Repo.FullName, evt.Sender.Login)

	// Match event to an agent via routes
	match := d.router.Match(evt)
	if match == nil {
		log.Printf("[DEBUG] No matching route for event %s/%s in repo %s",
			evt.Event, evt.Action, evt.Repo.FullName)
		return true // Not an error, just no matching route
	}

	log.Printf("[INFO] Matched agent %q (id=%d) via route %d",
		match.Agent.Name, match.Agent.ID, match.Route.ID)

	// Determine task type based on event
	taskType := determineTaskType(evt)

	// Build task context from event (uses templates if configured)
	taskContext := d.buildTaskContext(evt, match.Agent, taskType)

	// Get issue number for the task
	issueID := 0
	if evt.Issue != nil {
		issueID = evt.Issue.Number
	} else if evt.PR != nil {
		issueID = evt.PR.Number
	}

	// Create and enqueue task
	task := &store.Task{
		Event:      evt.Event,
		Repo:       evt.Repo.FullName,
		IssueID:    issueID,
		AgentID:    match.Agent.ID,
		TaskType:   taskType,
		Context:    taskContext,
		Status:     "pending",
		Priority:   match.Route.Priority,
		DeliveryID: evt.DeliveryID,
	}

	if err := d.queue.Enqueue(task); err != nil {
		log.Printf("[ERROR] Failed to enqueue task: %v", err)
		return false
	}

	log.Printf("[INFO] Task %d enqueued for agent %s on %s#%d",
		task.ID, match.Agent.Name, task.Repo, task.IssueID)
	return true
}

// buildTaskContext constructs the context string for the task from the event.
// If the agent has a user_template, it renders it with the event data.
// Otherwise, it falls back to the default context builder.
func (d *Dispatcher) buildTaskContext(evt *webhook.WebhookEvent, agent *store.Agent, taskType string) string {
	// Try to use agent's user_template first
	if agent.UserTemplate != "" {
		rendered, err := RenderTemplate(agent.UserTemplate, BuildTemplateData(evt))
		if err != nil {
			log.Printf("[WARN] Failed to render user_template: %v, using default", err)
		} else if rendered != "" {
			return rendered
		}
	}

	// Try to use template from config based on task type
	if d.agentsCfg != nil {
		if tmpl, ok := d.agentsCfg.Templates[taskType]; ok && tmpl.UserTemplate != "" {
			data := BuildTemplateData(evt)
			data.Task = &TaskData{TaskType: taskType}
			rendered, err := RenderTemplate(tmpl.UserTemplate, data)
			if err != nil {
				log.Printf("[WARN] Failed to render config template: %v, using default", err)
			} else if rendered != "" {
				return rendered
			}
		}
	}

	// Fallback to default context builder
	return d.buildDefaultContext(evt)
}

// buildDefaultContext builds the default context string without templates.
func (d *Dispatcher) buildDefaultContext(evt *webhook.WebhookEvent) string {
	var sb strings.Builder

	// Add repository info
	sb.WriteString(fmt.Sprintf("Repository: %s\n", evt.Repo.FullName))

	// Add issue/PR info
	if evt.Issue != nil {
		sb.WriteString(fmt.Sprintf("Issue #%d: %s\n", evt.Issue.Number, evt.Issue.Title))
		sb.WriteString(fmt.Sprintf("State: %s\n", evt.Issue.State))
		sb.WriteString(fmt.Sprintf("Author: %s\n", evt.Issue.User.Login))
		if evt.Issue.Body != "" {
			sb.WriteString(fmt.Sprintf("\nBody:\n%s\n", evt.Issue.Body))
		}
		if len(evt.Issue.Labels) > 0 {
			labels := make([]string, len(evt.Issue.Labels))
			for i, l := range evt.Issue.Labels {
				labels[i] = l.Name
			}
			sb.WriteString(fmt.Sprintf("Labels: %s\n", strings.Join(labels, ", ")))
		}
	}

	if evt.PR != nil {
		sb.WriteString(fmt.Sprintf("PR #%d: %s\n", evt.PR.Number, evt.PR.Title))
		sb.WriteString(fmt.Sprintf("State: %s\n", evt.PR.State))
		sb.WriteString(fmt.Sprintf("Author: %s\n", evt.PR.User.Login))
		sb.WriteString(fmt.Sprintf("Head: %s → Base: %s\n", evt.PR.Head.Ref, evt.PR.Base.Ref))
		if evt.PR.Body != "" {
			sb.WriteString(fmt.Sprintf("\nBody:\n%s\n", evt.PR.Body))
		}
	}

	if evt.Comment != nil {
		sb.WriteString(fmt.Sprintf("\nComment by %s:\n%s\n", evt.Comment.User.Login, evt.Comment.Body))
	}

	sb.WriteString(fmt.Sprintf("\nEvent: %s/%s\n", evt.Event, evt.Action))
	sb.WriteString(fmt.Sprintf("Sender: %s\n", evt.Sender.Login))

	return sb.String()
}

// determineTaskType returns the task type based on the event.
// Supports label-based task type override: "ai:solve" → solve_issue, "ai:fix" → fix_bug
func determineTaskType(evt *webhook.WebhookEvent) string {
	switch evt.Event {
	case "issues":
		// Check for label-based task type override
		if evt.Issue != nil {
			for _, label := range evt.Issue.Labels {
				switch label.Name {
				case "ai:solve":
					return "solve_issue"
				case "ai:fix":
					return "fix_bug"
				}
			}
		}
		if evt.Action == "assigned" || evt.Action == "labeled" {
			return "analyze_issue"
		}
		return "trigger"
	case "pull_request":
		return "review_pr"
	case "issue_comment", "pull_request_comment":
		return "reply_comment"
	default:
		return "trigger"
	}
}

// GetGiteaClient creates a Gitea client using agent's token for writeback.
func (d *Dispatcher) GetGiteaClient(agentToken string) *gitea.Client {
	return gitea.NewClient(d.giteaCfg.URL, agentToken)
}
