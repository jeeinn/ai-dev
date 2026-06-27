package workflow

import (
	"fmt"
	"strings"

	"gitea-agent-gateway/internal/store"
)

// GateLevel defines the enforcement level of a gate.
type GateLevel string

const (
	GateOff  GateLevel = "off"  // No check
	GateSoft GateLevel = "soft" // Warn but allow (can /force)
	GateHard GateLevel = "hard" // Block
)

// WorkflowPolicy defines the configurable workflow gates.
type WorkflowPolicy struct {
	Preset string            `yaml:"preset" json:"preset"` // free | standard | strict
	Gates  map[string]string `yaml:"gates" json:"gates"`   // gate_id → off|soft|hard
	Notify NotifyPolicy      `yaml:"notify" json:"notify"`
}

// NotifyPolicy controls L3 comment notifications.
type NotifyPolicy struct {
	OnAnalyzeDone   bool `yaml:"on_analyze_done" json:"on_analyze_done"`
	OnCoderPROpened bool `yaml:"on_coder_pr_opened" json:"on_coder_pr_opened"`
	OnGateSoft      bool `yaml:"on_gate_soft" json:"on_gate_soft"`
	OnGateHard      bool `yaml:"on_gate_hard" json:"on_gate_hard"`
}

// Gate IDs
const (
	GateCoderRequiresAnalyzed = "coder_requires_analyzed"
	GateAllowSkipAnalyze      = "allow_skip_analyze"
	GateReanalyzeWhileDev     = "reanalyze_while_developing"
	GateRerunSameStage        = "rerun_same_stage"
	GateReviewWarnIfDraft     = "review_warn_if_draft"
	GateCoderSwitchAgent      = "coder_switch_agent"
)

// PresetFree returns the free preset (minimal gates).
func PresetFree() *WorkflowPolicy {
	return &WorkflowPolicy{
		Preset: "free",
		Gates: map[string]string{
			GateCoderRequiresAnalyzed: "off",
			GateAllowSkipAnalyze:      "true",
			GateReanalyzeWhileDev:     "off",
			GateRerunSameStage:        "off",
			GateReviewWarnIfDraft:     "off",
			GateCoderSwitchAgent:      "off",
		},
		Notify: NotifyPolicy{
			OnAnalyzeDone:   true,
			OnCoderPROpened: true,
			OnGateSoft:      false,
			OnGateHard:      true,
		},
	}
}

// PresetStandard returns the standard preset (balanced).
func PresetStandard() *WorkflowPolicy {
	return &WorkflowPolicy{
		Preset: "standard",
		Gates: map[string]string{
			GateCoderRequiresAnalyzed: "off",
			GateAllowSkipAnalyze:      "true",
			GateReanalyzeWhileDev:     "soft",
			GateRerunSameStage:        "soft",
			GateReviewWarnIfDraft:     "off",
			GateCoderSwitchAgent:      "soft",
		},
		Notify: NotifyPolicy{
			OnAnalyzeDone:   true,
			OnCoderPROpened: true,
			OnGateSoft:      true,
			OnGateHard:      true,
		},
	}
}

// PresetStrict returns the strict preset (maximum gates).
func PresetStrict() *WorkflowPolicy {
	return &WorkflowPolicy{
		Preset: "strict",
		Gates: map[string]string{
			GateCoderRequiresAnalyzed: "hard",
			GateAllowSkipAnalyze:      "false",
			GateReanalyzeWhileDev:     "hard",
			GateRerunSameStage:        "hard",
			GateReviewWarnIfDraft:     "soft",
			GateCoderSwitchAgent:      "hard",
		},
		Notify: NotifyPolicy{
			OnAnalyzeDone:   true,
			OnCoderPROpened: true,
			OnGateSoft:      true,
			OnGateHard:      true,
		},
	}
}

// GetPreset returns the policy for the given preset name.
func GetPreset(name string) *WorkflowPolicy {
	switch name {
	case "free":
		return PresetFree()
	case "strict":
		return PresetStrict()
	default:
		return PresetStandard()
	}
}

// GetGateLevel returns the enforcement level for a gate.
func (p *WorkflowPolicy) GetGateLevel(gateID string) GateLevel {
	if p == nil || p.Gates == nil {
		return GateOff
	}
	level, ok := p.Gates[gateID]
	if !ok {
		return GateOff
	}
	switch GateLevel(level) {
	case GateSoft:
		return GateSoft
	case GateHard:
		return GateHard
	default:
		return GateOff
	}
}

// GateEvaluateResult is the outcome of an L2 gate evaluation.
type GateEvaluateResult struct {
	Allowed bool   // Whether the action is allowed
	Level   string // "pass", "soft", "hard"
	Code    string // Gate ID
	Message string // Human-readable message for comments
	Hint    string // Suggested next action
}

// EvaluateGate evaluates an L2 workflow gate.
// Returns GateEvaluateResult with Allowed=true if the transition should proceed.
// agentID is the incoming agent's ID (used by coder_switch_agent to detect agent changes).
// isDraftPR indicates the PR is a draft (used by review_warn_if_draft).
func EvaluateGate(policy *WorkflowPolicy, gateID string, ctx *store.WorkflowContext, agentRole string, agentID int64, isDraftPR bool) GateEvaluateResult {
	level := policy.GetGateLevel(gateID)

	if level == GateOff {
		return GateEvaluateResult{Allowed: true, Level: "pass", Code: gateID}
	}

	// Evaluate specific gate logic
	switch gateID {
	case GateCoderRequiresAnalyzed:
		return evalCoderRequiresAnalyzed(level, ctx)
	case GateReanalyzeWhileDev:
		return evalReanalyzeWhileDev(level, ctx)
	case GateRerunSameStage:
		return evalRerunSameStage(level, ctx)
	case GateCoderSwitchAgent:
		return evalCoderSwitchAgent(level, ctx, agentID)
	case GateReviewWarnIfDraft:
		return evalReviewWarnIfDraft(level, isDraftPR)
	default:
		return GateEvaluateResult{Allowed: true, Level: "pass", Code: gateID}
	}
}

func evalCoderRequiresAnalyzed(level GateLevel, ctx *store.WorkflowContext) GateEvaluateResult {
	// Only relevant if current stage is idle (never analyzed)
	if ctx.Stage == "idle" {
		msg := "此 Issue 尚未进行需求分析。"
		hint := "建议先 Assign analyze Agent 进行需求分析。"
		if level == GateHard {
			return GateEvaluateResult{
				Allowed: false, Level: "hard", Code: GateCoderRequiresAnalyzed,
				Message: "❌ " + msg, Hint: hint,
			}
		}
		return GateEvaluateResult{
			Allowed: true, Level: "soft", Code: GateCoderRequiresAnalyzed,
			Message: "⚠️ " + msg + " 已按配置继续执行。", Hint: hint,
		}
	}
	return GateEvaluateResult{Allowed: true, Level: "pass", Code: GateCoderRequiresAnalyzed}
}

func evalReanalyzeWhileDev(level GateLevel, ctx *store.WorkflowContext) GateEvaluateResult {
	if ctx.Stage == "developing" {
		msg := "开发阶段中重新分析，可能中断当前开发工作。"
		if level == GateHard {
			return GateEvaluateResult{
				Allowed: false, Level: "hard", Code: GateReanalyzeWhileDev,
				Message: "❌ " + msg,
			}
		}
		return GateEvaluateResult{
			Allowed: true, Level: "soft", Code: GateReanalyzeWhileDev,
			Message: "⚠️ " + msg,
		}
	}
	return GateEvaluateResult{Allowed: true, Level: "pass", Code: GateReanalyzeWhileDev}
}

func evalRerunSameStage(level GateLevel, ctx *store.WorkflowContext) GateEvaluateResult {
	// This is checked when the same stage would be re-entered
	msg := fmt.Sprintf("重复执行 %s 阶段。", ctx.Stage)
	if level == GateHard {
		return GateEvaluateResult{
			Allowed: false, Level: "hard", Code: GateRerunSameStage,
			Message: "❌ " + msg,
		}
	}
	return GateEvaluateResult{
		Allowed: true, Level: "soft", Code: GateRerunSameStage,
		Message: "⚠️ " + msg,
	}
}

func evalCoderSwitchAgent(level GateLevel, ctx *store.WorkflowContext, incomingAgentID int64) GateEvaluateResult {
	// Only fire when there's an active agent and it's a different one
	if ctx.ActiveAgentID == 0 || ctx.ActiveAgentID == incomingAgentID {
		return GateEvaluateResult{Allowed: true, Level: "pass", Code: GateCoderSwitchAgent}
	}
	msg := "切换 Agent 可能导致上下文丢失。"
	if level == GateHard {
		return GateEvaluateResult{
			Allowed: false, Level: "hard", Code: GateCoderSwitchAgent,
			Message: "❌ " + msg,
		}
	}
	return GateEvaluateResult{
		Allowed: true, Level: "soft", Code: GateCoderSwitchAgent,
		Message: "⚠️ " + msg,
	}
}

func evalReviewWarnIfDraft(level GateLevel, isDraftPR bool) GateEvaluateResult {
	if !isDraftPR {
		return GateEvaluateResult{Allowed: true, Level: "pass", Code: GateReviewWarnIfDraft}
	}
	msg := "此 PR 为 Draft 状态，审查可能尚未准备好。"
	if level == GateHard {
		return GateEvaluateResult{
			Allowed: false, Level: "hard", Code: GateReviewWarnIfDraft,
			Message: "❌ " + msg,
		}
	}
	return GateEvaluateResult{
		Allowed: true, Level: "soft", Code: GateReviewWarnIfDraft,
		Message: "⚠️ " + msg,
	}
}

// FormatL3Comment formats an L3 notification comment by replacing {{key}} placeholders.
func FormatL3Comment(template string, data map[string]string) string {
	result := template
	for k, v := range data {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

// L3 comment templates
const (
	L3AnalyzeDone = "✅ 分析完成（task #{{task_id}}）。\n\n建议：确认无误后 Assign coder Agent 开始实现。\n若需调整方案，请 @{{agent_name}} 继续讨论。"

	L3CoderPROpened = "✅ PR 已创建：{{pr_url}}\n\n建议：Request reviewer Agent 进行代码审查。"

	L3GateSoft = "⚠️ {{message}}\n\n已按配置继续执行。若希望强制跳过此检查，请在评论中使用 /force。"

	L3GateHard = "❌ {{message}}\n\n{{hint}}"
)
