package dispatcher

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
	"gitea-agent-gateway/internal/workflow"
)

// Dispatcher orchestrates the event processing pipeline:
// WebhookEvent → EventResolver → Gate checks → WorkflowContext → TaskQueue.Enqueue → Executor.execute
type Dispatcher struct {
	router    *Router // Legacy router (Phase 16.9 will remove)
	queue     *TaskQueue
	executor  *Executor
	db        *store.DB
	giteaCfg  *config.GiteaConfig
	agentsCfg *config.AgentsConfig

	// v2 components (optional; when set, new pipeline is used)
	registry   *agents.Registry
	resolver   *workflow.Resolver
	wfMgr      *workflow.WorkflowManager
	l1Gate     *workflow.L1Gate
	sessionSvc *workflow.SessionService
	wfPolicy   *workflow.WorkflowPolicy
	lifecycle  *workflow.SessionLifecycle

	// In-flight lock: prevents concurrent tasks on the same (repo, issue)
	inFlight sync.Map // map[string]bool — key is "repo#issueID"
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

// SetWorkflowComponents sets the v2 workflow components.
// When set, the dispatcher uses the new Assign-based pipeline instead of Router.Match.
func (d *Dispatcher) SetWorkflowComponents(registry *agents.Registry, resolver *workflow.Resolver, wfMgr *workflow.WorkflowManager, l1Gate *workflow.L1Gate, sessionSvc *workflow.SessionService, wfPolicy *workflow.WorkflowPolicy, lifecycle *workflow.SessionLifecycle) {
	d.registry = registry
	d.resolver = resolver
	d.wfMgr = wfMgr
	d.l1Gate = l1Gate
	d.sessionSvc = sessionSvc
	d.wfPolicy = wfPolicy
	d.lifecycle = lifecycle

	// Wire task completion callback for WorkflowContext + Session updates
	d.executor.SetOnComplete(func(task *store.Task) {
		if wfMgr == nil || task == nil {
			return
		}
		ctx, err := d.db.GetWorkflowContext(task.Repo, task.IssueID)
		if err != nil {
			log.Printf("[WARN] Failed to get workflow context for task %d completion: %v", task.ID, err)
			return
		}
		if err := wfMgr.OnTaskComplete(ctx, task.TaskType, 0, task.SessionID); err != nil {
			log.Printf("[WARN] Failed to update workflow context after task %d: %v", task.ID, err)
		}
		// Update session state
		if sessionSvc != nil && task.SessionID != "" {
			if session, err := d.db.GetSession(task.SessionID); err == nil {
				// Extract branch from task result (for DevRunner PR creation)
				branch := session.Branch
				if err := sessionSvc.CompleteTask(session, task.ID, branch, ""); err != nil {
					log.Printf("[WARN] Failed to update session after task %d: %v", task.ID, err)
				}
			}
		}
		// Release in-flight lock
		lockKey := fmt.Sprintf("%s#%d", task.Repo, task.IssueID)
		d.inFlight.Delete(lockKey)
	})
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
// Returns true if the event was successfully processed (or intentionally skipped).
func (d *Dispatcher) HandleEvent(evt *webhook.WebhookEvent) bool {
	log.Printf("[INFO] Processing event: %s/%s repo=%s sender=%s",
		evt.Event, evt.Action, evt.Repo.FullName, evt.Sender.Login)

	// v2 pipeline: use Resolver if workflow components are configured
	if d.resolver != nil {
		return d.handleEventV2(evt)
	}

	// Legacy pipeline: Router.Match
	return d.handleEventLegacy(evt)
}

// handleEventV2 processes events through the new Assign-based pipeline.
func (d *Dispatcher) handleEventV2(evt *webhook.WebhookEvent) bool {
	// Step 1: Sender filter — skip if sender is any active agent (self-trigger prevention)
	if d.resolver.IsAgentSender(evt) {
		log.Printf("[INFO] Sender %q is an active agent, ignoring event to prevent self-trigger", evt.Sender.Login)
		return true
	}

	// Step 2: Resolve event via EventResolver
	result := d.resolver.Resolve(evt)
	if result == nil {
		log.Printf("[DEBUG] Event %s/%s not handled by resolver, ignoring", evt.Event, evt.Action)
		return true // Not an error, just not handled
	}

	repo := evt.Repo.FullName
	issueID := result.IssueID

	// Handle lifecycle events (archive, done) — no agent required
	if result.Lifecycle != "" {
		log.Printf("[INFO] Lifecycle event: %s issueID=%d prID=%d merged=%v",
			result.Lifecycle, result.IssueID, result.PRID, result.Merged)
		return d.handleLifecycleEvent(result, repo)
	}

	// Task-creating events require an agent
	if result.Agent == nil {
		log.Printf("[DEBUG] No agent resolved for %s/%s, ignoring", evt.Event, evt.Action)
		return true
	}

	log.Printf("[INFO] Resolved: agent=%q role=%s taskType=%s issueID=%d prID=%d",
		result.Agent.Name, result.Role, result.TaskType, result.IssueID, result.PRID)

	// Step 3: L1 gate check
	if d.l1Gate != nil {
		l1Result := d.l1Gate.CheckL1(evt, result.Role, result.Agent)
		if !l1Result.Allowed {
			log.Printf("[INFO] L1 gate rejected: %s — %s", l1Result.Code, l1Result.Message)
			// Post rejection comment via agent's token
			d.postGateComment(result.Agent, repo, issueID, l1Result.Message)
			return true
		}
	}

	// Step 4: WorkflowContext transition
	var sessionID string
	if d.wfMgr != nil {
		ctx, err := d.db.GetOrCreateWorkflowContext(repo, issueID)
		if err != nil {
			log.Printf("[ERROR] Failed to get workflow context: %v", err)
			return false
		}

		transition := d.wfMgr.Transition(ctx, result.Role)
		if !transition.Allowed {
			log.Printf("[INFO] Transition blocked: %s", transition.Reason)
			d.postGateComment(result.Agent, repo, issueID, "⚠️ "+transition.Reason)
			return true
		}

		// Step 4b: L2 gate evaluation (if policy is configured)
		if d.wfPolicy != nil {
			// Check relevant gates based on the transition
			gatesToCheck := d.gatesForTransition(ctx, result.Role)
			for _, gateID := range gatesToCheck {
				gateResult := workflow.EvaluateGate(d.wfPolicy, gateID, ctx, result.Role)
				if !gateResult.Allowed {
					if result.Force && gateResult.Level == "soft" {
						// /force bypasses soft gates
						log.Printf("[INFO] /force bypassing soft gate: %s", gateID)
						d.postGateComment(result.Agent, repo, issueID, "⚡ /force 已跳过软门禁: "+gateResult.Message)
						continue
					}
					// Hard gate or no /force — block
					log.Printf("[INFO] L2 gate rejected: %s — %s", gateID, gateResult.Message)
					d.postGateComment(result.Agent, repo, issueID, gateResult.Message)
					return true
				}
				if gateResult.Level == "soft" && d.wfPolicy.Notify.OnGateSoft {
					// Soft gate passed — post warning
					d.postGateComment(result.Agent, repo, issueID, gateResult.Message)
				}
			}
		}

		// Step 5: In-flight lock
		lockKey := fmt.Sprintf("%s#%d", repo, issueID)
		if _, loaded := d.inFlight.LoadOrStore(lockKey, true); loaded {
			log.Printf("[INFO] In-flight lock held for %s, skipping", lockKey)
			d.postGateComment(result.Agent, repo, issueID, "⏳ 此 Issue 正在处理中，请稍候。")
			return true
		}

		// Step 6: Check for existing pending/running tasks
		hasPending, err := d.db.HasPendingOrRunningTask(repo, issueID)
		if err != nil {
			d.inFlight.Delete(lockKey)
			log.Printf("[ERROR] Failed to check pending tasks: %v", err)
			return false
		}
		if hasPending {
			d.inFlight.Delete(lockKey)
			log.Printf("[INFO] Pending/running task exists for %s#%d, skipping", repo, issueID)
			d.postGateComment(result.Agent, repo, issueID, "⏳ 已有任务正在处理中。")
			return true
		}

		// Step 6b: Get or create session
		if d.sessionSvc != nil {
			session, err := d.sessionSvc.GetOrCreate(repo, issueID, result.Agent.ID, result.Role)
			if err != nil {
				d.inFlight.Delete(lockKey)
				log.Printf("[ERROR] Failed to get/create session: %v", err)
				return false
			}
			sessionID = session.ID

			// Set base branch from session for coder continuation
			if result.Role == store.RoleCoder && session.Branch != "" {
				// Will be used below for task.BaseBranch
			}
		}

		// Apply transition to DB
		if err := d.wfMgr.ApplyTransition(ctx, transition, result.Agent.ID, result.Role, sessionID); err != nil {
			d.inFlight.Delete(lockKey)
			log.Printf("[ERROR] Failed to apply transition: %v", err)
			return false
		}
	}

	// Step 7: Build task context and enqueue
	taskContext := d.buildTaskContext(evt, result.Agent, result.TaskType)

	task := &store.Task{
		Event:      evt.Event,
		Repo:       repo,
		IssueID:    issueID,
		AgentID:    result.Agent.ID,
		TaskType:   result.TaskType,
		Context:    taskContext,
		Status:     "pending",
		Priority:   10, // Default priority for v2
		Role:       result.Role,
		SessionID:  sessionID,
		DeliveryID: evt.DeliveryID,
	}

	// For PR events, capture the head branch
	if evt.PR != nil && evt.PR.Head.Ref != "" {
		task.BaseBranch = evt.PR.Head.Ref
	}

	if err := d.queue.Enqueue(task); err != nil {
		// Release in-flight lock on error
		lockKey := fmt.Sprintf("%s#%d", repo, issueID)
		d.inFlight.Delete(lockKey)
		log.Printf("[ERROR] Failed to enqueue task: %v", err)
		return false
	}

	log.Printf("[INFO] Task %d enqueued: agent=%s role=%s type=%s on %s#%d",
		task.ID, result.Agent.Name, result.Role, result.TaskType, repo, issueID)

	// Post progress comment
	d.postGateComment(result.Agent, repo, issueID,
		fmt.Sprintf("🔄 %s 已开始处理（task #%d）", result.Agent.Name, task.ID))

	return true
}

// handleLifecycleEvent handles lifecycle events (archive, done) from the Resolver.
func (d *Dispatcher) handleLifecycleEvent(result *workflow.ResolveResult, repo string) bool {
	if d.lifecycle == nil {
		return true
	}

	switch result.Lifecycle {
	case "archive":
		if result.PRID > 0 {
			// PR closed
			if err := d.lifecycle.OnPRClosed(repo, result.PRID, result.IssueID, result.Merged); err != nil {
				log.Printf("[WARN] PR lifecycle error: %v", err)
			}
		} else if result.IssueID > 0 {
			// Issue closed
			if err := d.lifecycle.OnIssueClosed(repo, result.IssueID); err != nil {
				log.Printf("[WARN] Issue lifecycle error: %v", err)
			}
		}
	}
	return true
}

// gatesForTransition returns the L2 gate IDs to check for the given transition.
func (d *Dispatcher) gatesForTransition(ctx *store.WorkflowContext, role string) []string {
	var gates []string

	switch role {
	case store.RoleCoder:
		if ctx.Stage == store.StageIdle || ctx.Stage == store.StageDone {
			gates = append(gates, workflow.GateCoderRequiresAnalyzed)
		}
		if ctx.Stage == store.StageDeveloping {
			gates = append(gates, workflow.GateRerunSameStage)
		}
	case store.RoleAnalyze:
		if ctx.Stage == store.StageDeveloping {
			gates = append(gates, workflow.GateReanalyzeWhileDev)
		}
		if ctx.Stage == store.StageAnalyzing {
			gates = append(gates, workflow.GateRerunSameStage)
		}
	}

	return gates
}

// postGateComment posts a comment on the issue/PR using the agent's Gitea token.
func (d *Dispatcher) postGateComment(agent *store.Agent, repo string, issueID int, body string) {
	if d.giteaCfg == nil || agent.GiteaToken == "" || issueID == 0 {
		return
	}
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return
	}
	client := gitea.NewClient(d.giteaCfg.URL, agent.GiteaToken)
	commentBody := workflow.FormatAgentComment(body)
	if err := client.IssueComment(parts[0], parts[1], issueID, commentBody); err != nil {
		log.Printf("[WARN] Failed to post gate comment on %s#%d: %v", repo, issueID, err)
	}
}

// handleEventLegacy processes events through the old Router.Match pipeline (pre-v2).
func (d *Dispatcher) handleEventLegacy(evt *webhook.WebhookEvent) bool {
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

	// For PR events, capture the head branch so runners can update the existing PR
	if evt.PR != nil && evt.PR.Head.Ref != "" {
		task.BaseBranch = evt.PR.Head.Ref
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

// determineTaskType returns the task type based on the event (legacy pipeline).
// v2: Label-based task type override removed. Task type is determined by Agent.role + event.
func determineTaskType(evt *webhook.WebhookEvent) string {
	switch evt.Event {
	case "issues":
		if evt.Action == "assigned" {
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

// GetAdminGiteaClient creates a Gitea client using admin token.
func (d *Dispatcher) GetAdminGiteaClient() *gitea.Client {
	return gitea.NewClient(d.giteaCfg.URL, d.giteaCfg.AdminToken)
}
