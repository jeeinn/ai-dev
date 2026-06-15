package workflow

import (
	"fmt"
	"log"

	"gitea-agent-gateway/internal/store"
)

// WorkflowManager manages WorkflowContext state transitions.
type WorkflowManager struct {
	db *store.DB
}

// NewWorkflowManager creates a new WorkflowManager.
func NewWorkflowManager(db *store.DB) *WorkflowManager {
	return &WorkflowManager{db: db}
}

// TransitionResult holds the result of a stage transition attempt.
type TransitionResult struct {
	Allowed  bool   // Whether the transition is allowed
	NewStage string // The stage to transition to (if Allowed)
	Reason   string // Human-readable reason (for comments)
	SkipTask bool   // If true, the task should not be enqueued
}

// Transition evaluates whether a stage transition is allowed and returns the result.
// This does NOT modify the database — the caller is responsible for applying the transition
// after successful L2 gate check and task enqueue.
func (m *WorkflowManager) Transition(ctx *store.WorkflowContext, role string) TransitionResult {
	currentStage := ctx.Stage

	switch role {
	case store.RoleAnalyze:
		return m.transitionAnalyze(currentStage)
	case store.RoleCoder:
		return m.transitionCoder(currentStage)
	case store.RoleReview:
		return m.transitionReview(currentStage)
	default:
		return TransitionResult{Allowed: false, Reason: fmt.Sprintf("unknown role: %s", role)}
	}
}

// transitionAnalyze handles analyze role transitions.
func (m *WorkflowManager) transitionAnalyze(currentStage string) TransitionResult {
	switch currentStage {
	case store.StageIdle, store.StageAnalyzed, store.StageDone:
		return TransitionResult{Allowed: true, NewStage: store.StageAnalyzing}
	case store.StageAnalyzing:
		// Already analyzing — in-flight check should catch this
		return TransitionResult{Allowed: false, SkipTask: true, Reason: "分析任务正在进行中"}
	case store.StageDeveloping:
		// Re-analyze while developing — allowed with soft warning (L2 handles the warning)
		return TransitionResult{Allowed: true, NewStage: store.StageAnalyzing, Reason: "开发阶段中重新分析，可能中断当前开发"}
	case store.StageReviewing:
		// Re-analyze while reviewing — allowed with soft warning
		return TransitionResult{Allowed: true, NewStage: store.StageAnalyzing, Reason: "审查阶段中重新分析"}
	default:
		return TransitionResult{Allowed: false, Reason: fmt.Sprintf("无法从阶段 %s 转换到 analyzing", currentStage)}
	}
}

// transitionCoder handles coder role transitions.
func (m *WorkflowManager) transitionCoder(currentStage string) TransitionResult {
	switch currentStage {
	case store.StageIdle:
		// Idle → developing (allowed if allow_skip_analyze is true, which is the default)
		return TransitionResult{Allowed: true, NewStage: store.StageDeveloping}
	case store.StageAnalyzed:
		return TransitionResult{Allowed: true, NewStage: store.StageDeveloping}
	case store.StageDeveloping:
		// Already developing — in-flight check should catch same-task; re-run same stage handled by L2
		return TransitionResult{Allowed: true, NewStage: store.StageDeveloping, Reason: "开发阶段重新执行"}
	case store.StageReviewing:
		// @coder continuation from review — allowed
		return TransitionResult{Allowed: true, NewStage: store.StageDeveloping, Reason: "从审查阶段回到开发"}
	case store.StageDone:
		return TransitionResult{Allowed: true, NewStage: store.StageDeveloping, Reason: "从完成状态重新开发"}
	case store.StageAnalyzing:
		return TransitionResult{Allowed: false, Reason: "分析进行中，请等待分析完成后再开始开发"}
	default:
		return TransitionResult{Allowed: false, Reason: fmt.Sprintf("无法从阶段 %s 转换到 developing", currentStage)}
	}
}

// transitionReview handles review role transitions.
func (m *WorkflowManager) transitionReview(currentStage string) TransitionResult {
	// Review is always allowed (structural gate L1 checks for open PR)
	return TransitionResult{Allowed: true, NewStage: store.StageReviewing}
}

// ApplyTransition updates the WorkflowContext in the database after a successful transition.
func (m *WorkflowManager) ApplyTransition(ctx *store.WorkflowContext, result TransitionResult, agentID int64, role, sessionID string) error {
	if !result.Allowed {
		return fmt.Errorf("transition not allowed: %s", result.Reason)
	}
	return m.db.TransitionStage(ctx, result.NewStage, agentID, role, sessionID)
}

// OnTaskComplete updates the WorkflowContext stage after a task completes successfully.
func (m *WorkflowManager) OnTaskComplete(ctx *store.WorkflowContext, taskType string, prID int, sessionID string) error {
	switch taskType {
	case "analyze_issue":
		ctx.Stage = store.StageAnalyzed
	case "solve_issue", "fix_bug":
		// Stay in developing; write PR ID if available
		if prID > 0 {
			ctx.PRID = prID
		}
	case "review_pr":
		// Stay in reviewing
	case "reply_comment", "solve_comment":
		// No stage change
	default:
		log.Printf("[WARN] Unknown task type %s for stage update", taskType)
	}
	return m.db.UpdateWorkflowContext(ctx)
}
