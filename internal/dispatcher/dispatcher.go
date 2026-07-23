package dispatcher

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/sandbox"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
	"gitea-agent-gateway/internal/workflow"
)

// Dispatcher orchestrates the event processing pipeline:
// WebhookEvent → EventResolver → Gate checks → WorkflowContext → TaskQueue.Enqueue → Executor.execute
type Dispatcher struct {
	queue     *TaskQueue
	executor  *Executor
	db        *store.DB
	giteaCfg  atomic.Pointer[config.GiteaConfig]
	agentsCfg *config.AgentsConfig

	// v2 components
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
	sandboxCfg sandbox.SandboxConfig,
	mcpCfg config.MCPConfig,
) *Dispatcher {
	queue := NewTaskQueue(db, dispatcherCfg.QueueSize)
	agentDefaults := config.DefaultAgentDefaults()
	if agentsCfg != nil {
		agentDefaults = agentsCfg.Defaults
		if agentDefaults.MaxOutputTokens <= 0 {
			agentDefaults.MaxOutputTokens = config.DefaultAgentDefaults().MaxOutputTokens
		}
		if agentDefaults.MaxInputTokens <= 0 {
			agentDefaults.MaxInputTokens = config.DefaultAgentDefaults().MaxInputTokens
		}
		if agentDefaults.Temperature <= 0 {
			agentDefaults.Temperature = config.DefaultAgentDefaults().Temperature
		}
		if agentDefaults.Timeout == "" {
			agentDefaults.Timeout = config.DefaultAgentDefaults().Timeout
		}
	}
	executor := NewExecutor(
		dispatcherCfg.MaxConcurrent,
		dispatcherCfg.TaskRetryCount,
		llmRegistry,
		db,
		agentDefaults,
		resolveDefaultLoop(agentsCfg),
		sandboxCfg,
		mcpCfg,
	)

	d := &Dispatcher{
		queue:     queue,
		executor:  executor,
		db:        db,
		agentsCfg: agentsCfg,
	}
	d.giteaCfg.Store(giteaCfg)

	// Wire up Gitea client factory for result writeback
	var backends *config.AgentBackendsConfig
	if agentsCfg != nil {
		backends = &agentsCfg.Backends
	}
	executor.SetGiteaClientFactory(d, nil, backends)

	return d
}

// SetDebugConfigGetter supplies live debug settings for conversation logging.
func (d *Dispatcher) SetDebugConfigGetter(getter func() config.DebugConfig) {
	if d.executor != nil && d.executor.giteaFactory != nil {
		var backends *config.AgentBackendsConfig
		if d.agentsCfg != nil {
			backends = &d.agentsCfg.Backends
		}
		d.executor.SetGiteaClientFactory(d.executor.giteaFactory, getter, backends)
	}
}

// SetModelMetaProvider sets the model metadata provider for adaptive token limits.
func (d *Dispatcher) SetModelMetaProvider(m agents.ModelMetaProvider) {
	if d.executor != nil {
		d.executor.SetModelMetaProvider(m)
	}
}

// SetGiteaConfig updates Gitea settings used for admin clients / writeback (hot reload).
func (d *Dispatcher) SetGiteaConfig(cfg *config.GiteaConfig) {
	if cfg == nil {
		return
	}
	d.giteaCfg.Store(cfg)
}

func resolveDefaultLoop(agentsCfg *config.AgentsConfig) config.AgentLoopConfig {
	if agentsCfg == nil {
		return config.DefaultAgentLoopConfig()
	}
	loop := agentsCfg.Loop
	if loop.MaxIterations <= 0 && loop.TotalTimeout == "" && loop.IterationInterval <= 0 {
		return config.DefaultAgentLoopConfig()
	}
	defaults := config.DefaultAgentLoopConfig()
	if loop.MaxIterations <= 0 {
		loop.MaxIterations = defaults.MaxIterations
	}
	if loop.TotalTimeout == "" {
		loop.TotalTimeout = defaults.TotalTimeout
	}
	return loop
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
		if err := wfMgr.OnTaskComplete(ctx, task.TaskType, task.PRID, task.SessionID); err != nil {
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
		// L3 notification (if policy configured)
		d.postL3Notification(task)
		d.releaseIssueLock(task.Repo, task.IssueID)
	})

	// Wire task failure callback: rollback workflow stage + release in-flight lock
	d.executor.SetOnFailed(func(task *store.Task) {
		if task == nil {
			return
		}
		if wfMgr != nil {
			ctx, err := d.db.GetWorkflowContext(task.Repo, task.IssueID)
			if err != nil {
				log.Printf("[WARN] Failed to get workflow context for task %d failure: %v", task.ID, err)
			} else if err := wfMgr.OnTaskFailed(ctx, task.TaskType); err != nil {
				log.Printf("[WARN] Failed to rollback workflow context after task %d: %v", task.ID, err)
			}
		}
		d.releaseIssueLock(task.Repo, task.IssueID)
	})
}

// SetWorkflowPolicy replaces the live L2 policy (e.g. after WebUI config hot-reload).
func (d *Dispatcher) SetWorkflowPolicy(wfPolicy *workflow.WorkflowPolicy) {
	d.wfPolicy = wfPolicy
}

// getEffectivePolicy returns the effective workflow policy for a repo.
// If a per-repo policy exists in DB, it overrides the global policy.
func (d *Dispatcher) getEffectivePolicy(repo string) *workflow.WorkflowPolicy {
	if d.db == nil {
		return d.wfPolicy
	}

	dbPolicy, err := d.db.GetWorkflowPolicy(repo)
	if err != nil || dbPolicy == nil {
		return d.wfPolicy
	}

	var gateOverrides map[string]string
	if dbPolicy.GatesJSON != "" {
		_ = json.Unmarshal([]byte(dbPolicy.GatesJSON), &gateOverrides)
	}

	return workflow.BuildPolicy(dbPolicy.Preset, gateOverrides)
}

// unassignPreviousAgentOnTransition removes the previous agent from the issue's assignee
// when the stage transitions to a different role. This is controlled by the
// stage_transition_unassign gate policy.
func (d *Dispatcher) unassignPreviousAgentOnTransition(repo string, issueID int, prevAgentID int64, newAgentID int64) {
	giteaCfg := d.giteaCfg.Load()
	if d.wfPolicy == nil || giteaCfg == nil {
		return
	}

	policy := d.getEffectivePolicy(repo)
	if policy == nil {
		policy = d.wfPolicy
	}

	level := policy.GetGateLevel(workflow.GateStageTransitionUnassign)
	if level == workflow.GateOff {
		return
	}

	if prevAgentID == 0 || prevAgentID == newAgentID {
		return
	}

	prevAgent, err := d.db.GetAgent(prevAgentID)
	if err != nil {
		log.Printf("[WARN] Failed to get previous agent %d: %v", prevAgentID, err)
		return
	}

	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return
	}
	owner, repoName := parts[0], parts[1]

	if prevAgent.GiteaUsername == "" {
		log.Printf("[WARN] Previous agent %d has empty gitea_username, skip unassign", prevAgentID)
		return
	}

	giteaClient := gitea.NewClient(giteaCfg.URL, giteaCfg.AdminToken)
	if err := giteaClient.IssueUnassign(owner, repoName, issueID, prevAgent.GiteaUsername); err != nil {
		log.Printf("[WARN] Failed to unassign agent %s from issue %s#%d: %v",
			prevAgent.GiteaUsername, repo, issueID, err)
		if level == workflow.GateHard {
			d.postGateComment(prevAgent, repo, issueID,
				fmt.Sprintf("⚠️ 未能从 Issue 移除前一 Agent [%s] 的分配，已按配置继续执行", prevAgent.GiteaUsername))
		}
	} else {
		log.Printf("[INFO] Unassigned agent %s from issue %s#%d on stage transition",
			prevAgent.GiteaUsername, repo, issueID)
	}
}

// Shutdown cancels in-flight executor work so agent loops can exit on process stop.
func (d *Dispatcher) Shutdown() {
	if d.executor != nil {
		d.executor.Shutdown()
	}
	if d.queue != nil {
		d.queue.StopScanner()
	}
}

// Start initializes the executor workers, loads pending tasks, and starts the queue scanner.
func (d *Dispatcher) Start() error {
	// Mark orphaned running tasks as failed (e.g. previous process killed with Ctrl+C)
	if n, err := d.db.FailOrphanedRunningTasks("gateway restarted; interrupted running task"); err != nil {
		log.Printf("[WARN] Failed to clear orphaned running tasks: %v", err)
	} else if n > 0 {
		log.Printf("[INFO] Marked %d orphaned running task(s) as failed after restart", n)
	}

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

// HandleEvent processes a webhook event through the v2 pipeline.
// This is the callback function passed to webhook.Handler.
// Returns true for terminal outcomes (enqueued or intentionally skipped);
// false for transient failures that should remain accepted for ReplayAccepted.
func (d *Dispatcher) HandleEvent(evt *webhook.WebhookEvent) bool {
	log.Printf("[INFO] Processing event: %s/%s repo=%s sender=%s",
		evt.Event, evt.Action, evt.Repo.FullName, evt.Sender.Login)

	if d.resolver == nil {
		log.Printf("[WARN] No resolver configured, ignoring event")
		return true
	}

	return d.handleEventV2(evt)
}

// handleEventV2 processes events through the new Assign-based pipeline.
func (d *Dispatcher) handleEventV2(evt *webhook.WebhookEvent) bool {
	// Step 1: Sender filter — skip if sender is any active agent (self-trigger prevention)
	if d.resolver.IsAgentSender(evt) {
		log.Printf("[INFO] Sender %q is an active agent, ignoring event to prevent self-trigger", evt.Sender.Login)
		return true
	}

	// Step 1b: Check for /gateway reset command in comments
	if evt.Comment != nil && strings.Contains(evt.Comment.Body, "/gateway reset") {
		issueID := 0
		if evt.Issue != nil {
			issueID = evt.Issue.Number
		}
		if issueID > 0 {
			log.Printf("[INFO] /gateway reset command detected for %s#%d", evt.Repo.FullName, issueID)
			d.resetIssue(evt.Repo.FullName, issueID)
			return true
		}
	}

	// Step 2: Resolve event via EventResolver
	result := d.resolver.Resolve(evt)
	if result == nil {
		if evt.Event == "pull_request" && evt.Action == "review_requested" {
			var reviewers []string
			prNum := 0
			if evt.PR != nil {
				prNum = evt.PR.Number
				for _, r := range evt.PR.RequestedReviewers {
					reviewers = append(reviewers, r.Login)
				}
			}
			log.Printf("[INFO] review_requested not handled: repo=%s pr=%d reviewers=%v sender=%q",
				evt.Repo.FullName, prNum, reviewers, evt.Sender.Login)
		} else {
			log.Printf("[DEBUG] Event %s/%s not handled by resolver, ignoring", evt.Event, evt.Action)
		}
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
	var sessionBranch string
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
			policy := d.getEffectivePolicy(repo)
			if policy == nil {
				policy = d.wfPolicy
			}
			// Check relevant gates based on the transition
			gatesToCheck := d.gatesForTransition(ctx, result.Role)
			// Determine if PR is a draft (for review_warn_if_draft gate)
			isDraftPR := evt.PR != nil && evt.PR.Draft
			for _, gateID := range gatesToCheck {
				gateResult := workflow.EvaluateGate(policy, gateID, ctx, result.Role, result.Agent.ID, isDraftPR)
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
				if gateResult.Level == "soft" && policy.Notify.OnGateSoft {
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
			if result.Role == store.RoleCoder && session.Branch != "" {
				sessionBranch = session.Branch
			}
		}

		// Save previous agent ID before ApplyTransition overwrites it
		prevAgentID := ctx.ActiveAgentID

		// Apply transition to DB
		if err := d.wfMgr.ApplyTransition(ctx, transition, result.Agent.ID, result.Role, sessionID); err != nil {
			d.inFlight.Delete(lockKey)
			log.Printf("[ERROR] Failed to apply transition: %v", err)
			return false
		}

		// Step 6c: Unassign previous agent on stage transition (if configured)
		d.unassignPreviousAgentOnTransition(repo, issueID, prevAgentID, result.Agent.ID)
	}

	// Step 7: Build task context and enqueue
	taskContext := d.buildTaskContext(evt, result.Agent, result.TaskType)

	task := &store.Task{
		Event:      evt.Event,
		Repo:       repo,
		IssueID:    issueID,
		PRID:       result.PRID,
		AgentID:    result.Agent.ID,
		TaskType:   result.TaskType,
		Context:    taskContext,
		Status:     "pending",
		Priority:   10, // Default priority for v2
		Role:       result.Role,
		SessionID:  sessionID,
		DeliveryID: evt.DeliveryID,
	}

	// PR head branch, or session branch for coder continuation when webhook omits pull_request.
	if evt.PR != nil && evt.PR.Head.Ref != "" {
		task.BaseBranch = strings.TrimSpace(evt.PR.Head.Ref)
	}
	if task.BaseBranch == "" && sessionBranch != "" {
		task.BaseBranch = sessionBranch
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

// resetIssue archives all sessions and resets the workflow context to idle.
func (d *Dispatcher) resetIssue(repo string, issueID int) {
	// Archive sessions
	if d.sessionSvc != nil {
		d.sessionSvc.ArchiveByIssue(repo, issueID)
	}

	// Reset context
	ctx, err := d.db.GetWorkflowContext(repo, issueID)
	if err == nil {
		ctx.Stage = store.StageIdle
		ctx.PreviousStage = ""
		ctx.ActiveAgentID = 0
		ctx.ActiveRole = ""
		ctx.SessionID = ""
		d.db.UpdateWorkflowContext(ctx)
	}

	// Release in-flight lock
	d.releaseIssueLock(repo, issueID)

	log.Printf("[INFO] Reset %s#%d: sessions archived, context=idle", repo, issueID)
}

func (d *Dispatcher) releaseIssueLock(repo string, issueID int) {
	lockKey := fmt.Sprintf("%s#%d", repo, issueID)
	d.inFlight.Delete(lockKey)
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
		// Check if switching to a different coder agent
		if ctx.ActiveAgentID != 0 {
			gates = append(gates, workflow.GateCoderSwitchAgent)
		}
	case store.RoleAnalyze:
		if ctx.Stage == store.StageDeveloping {
			gates = append(gates, workflow.GateReanalyzeWhileDev)
		}
		if ctx.Stage == store.StageAnalyzing {
			gates = append(gates, workflow.GateRerunSameStage)
		}
	case store.RoleReview:
		// Draft PR warning for review tasks
		gates = append(gates, workflow.GateReviewWarnIfDraft)
	}

	return gates
}

// postGateComment posts a comment on the issue/PR using the agent's Gitea token.
func (d *Dispatcher) postGateComment(agent *store.Agent, repo string, issueID int, body string) {
	giteaCfg := d.giteaCfg.Load()
	if giteaCfg == nil || agent.GiteaToken == "" || issueID == 0 {
		return
	}
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return
	}
	client := gitea.NewClient(giteaCfg.URL, agent.GiteaToken)
	commentBody := workflow.FormatAgentComment(body)
	if err := client.IssueComment(parts[0], parts[1], issueID, commentBody); err != nil {
		log.Printf("[WARN] Failed to post gate comment on %s#%d: %v", repo, issueID, err)
	}
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

// GetGiteaClient creates a Gitea client using agent's token for writeback.
func (d *Dispatcher) GetGiteaClient(agentToken string) *gitea.Client {
	cfg := d.giteaCfg.Load()
	if cfg == nil {
		return gitea.NewClient("", agentToken)
	}
	return gitea.NewClient(cfg.URL, agentToken)
}

// GetAdminGiteaClient creates a Gitea client using admin token.
func (d *Dispatcher) GetAdminGiteaClient() *gitea.Client {
	cfg := d.giteaCfg.Load()
	if cfg == nil {
		return gitea.NewClient("", "")
	}
	return gitea.NewClient(cfg.URL, cfg.AdminToken)
}

// prURLPattern matches PR URLs in task results (e.g. "PR created: http://...").
var prURLPattern = regexp.MustCompile(`PR created: (https?://\S+)`)

// postL3Notification posts an L3 comment notification after task completion,
// if the workflow policy has the corresponding notification enabled.
func (d *Dispatcher) postL3Notification(task *store.Task) {
	if d.wfPolicy == nil || d.giteaCfg.Load() == nil {
		return
	}

	// Load agent for token and name
	agent, err := d.db.GetAgent(task.AgentID)
	if err != nil || agent.GiteaToken == "" {
		return
	}

	switch task.TaskType {
	case "analyze_issue":
		if !d.wfPolicy.Notify.OnAnalyzeDone {
			return
		}
		body := workflow.FormatL3Comment(workflow.L3AnalyzeDone, map[string]string{
			"task_id":    fmt.Sprintf("%d", task.ID),
			"agent_name": agent.GiteaUsername,
		})
		d.postGateComment(agent, task.Repo, task.IssueID, body)

	case "solve_issue", "fix_bug", "solve_comment":
		if !d.wfPolicy.Notify.OnCoderPROpened {
			return
		}
		// Only notify when a PR was actually created
		if task.PRID == 0 {
			return
		}
		// Extract PR URL from result
		prURL := ""
		if matches := prURLPattern.FindStringSubmatch(task.Result); len(matches) >= 2 {
			prURL = matches[1]
		}
		if prURL == "" {
			return
		}
		body := workflow.FormatL3Comment(workflow.L3CoderPROpened, map[string]string{
			"pr_url": prURL,
		})
		d.postGateComment(agent, task.Repo, task.IssueID, body)
	}
}
