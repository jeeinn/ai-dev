package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitea-agent-gateway/internal/store"
)

func TestPresetFree(t *testing.T) {
	p := PresetFree()
	assert.Equal(t, "free", p.Preset)
	assert.Equal(t, "off", p.Gates[GateCoderRequiresAnalyzed])
	assert.Equal(t, "off", p.Gates[GateReanalyzeWhileDev])
	assert.Equal(t, "off", p.Gates[GateRerunSameStage])
}

func TestPresetStandard(t *testing.T) {
	p := PresetStandard()
	assert.Equal(t, "standard", p.Preset)
	assert.Equal(t, "off", p.Gates[GateCoderRequiresAnalyzed])
	assert.Equal(t, "soft", p.Gates[GateReanalyzeWhileDev])
	assert.Equal(t, "soft", p.Gates[GateRerunSameStage])
	assert.Equal(t, "soft", p.Gates[GateCoderSwitchAgent])
}

func TestPresetStrict(t *testing.T) {
	p := PresetStrict()
	assert.Equal(t, "strict", p.Preset)
	assert.Equal(t, "hard", p.Gates[GateCoderRequiresAnalyzed])
	assert.Equal(t, "hard", p.Gates[GateReanalyzeWhileDev])
	assert.Equal(t, "hard", p.Gates[GateRerunSameStage])
}

func TestGetPreset(t *testing.T) {
	assert.Equal(t, "free", GetPreset("free").Preset)
	assert.Equal(t, "standard", GetPreset("standard").Preset)
	assert.Equal(t, "strict", GetPreset("strict").Preset)
	assert.Equal(t, "standard", GetPreset("unknown").Preset) // Default
	assert.Equal(t, "standard", GetPreset("").Preset)
}

func TestGetGateLevel(t *testing.T) {
	p := PresetStandard()

	assert.Equal(t, GateOff, p.GetGateLevel(GateCoderRequiresAnalyzed))
	assert.Equal(t, GateSoft, p.GetGateLevel(GateReanalyzeWhileDev))
	assert.Equal(t, GateOff, p.GetGateLevel("nonexistent")) // Default off for unknown keys
}

func TestGetGateLevelNilPolicy(t *testing.T) {
	var p *WorkflowPolicy
	assert.Equal(t, GateOff, p.GetGateLevel(GateCoderRequiresAnalyzed))
}

func TestEvaluateGateOff(t *testing.T) {
	p := PresetFree()
	ctx := &store.WorkflowContext{Stage: store.StageIdle}

	result := EvaluateGate(p, GateCoderRequiresAnalyzed, ctx, store.RoleCoder, 0, false)
	assert.True(t, result.Allowed)
	assert.Equal(t, "pass", result.Level)
}

func TestEvaluateGateCoderRequiresAnalyzedIdle(t *testing.T) {
	p := PresetStrict()
	ctx := &store.WorkflowContext{Stage: store.StageIdle}

	result := EvaluateGate(p, GateCoderRequiresAnalyzed, ctx, store.RoleCoder, 0, false)
	assert.False(t, result.Allowed)
	assert.Equal(t, "hard", result.Level)
	assert.Contains(t, result.Message, "需求分析")
}

func TestEvaluateGateCoderRequiresAnalyzedAnalyzed(t *testing.T) {
	p := PresetStrict()
	ctx := &store.WorkflowContext{Stage: store.StageAnalyzed}

	result := EvaluateGate(p, GateCoderRequiresAnalyzed, ctx, store.RoleCoder, 0, false)
	assert.True(t, result.Allowed)
	assert.Equal(t, "pass", result.Level)
}

func TestEvaluateGateReanalyzeWhileDev(t *testing.T) {
	p := PresetStandard()
	ctx := &store.WorkflowContext{Stage: store.StageDeveloping}

	result := EvaluateGate(p, GateReanalyzeWhileDev, ctx, store.RoleAnalyze, 0, false)
	assert.True(t, result.Allowed) // Soft allows
	assert.Equal(t, "soft", result.Level)
	assert.Contains(t, result.Message, "开发阶段")
}

func TestEvaluateGateReanalyzeWhileDevStrict(t *testing.T) {
	p := PresetStrict()
	ctx := &store.WorkflowContext{Stage: store.StageDeveloping}

	result := EvaluateGate(p, GateReanalyzeWhileDev, ctx, store.RoleAnalyze, 0, false)
	assert.False(t, result.Allowed) // Hard blocks
	assert.Equal(t, "hard", result.Level)
}

func TestEvaluateGateRerunSameStage(t *testing.T) {
	p := PresetStandard()
	ctx := &store.WorkflowContext{Stage: store.StageDeveloping}

	result := EvaluateGate(p, GateRerunSameStage, ctx, store.RoleCoder, 0, false)
	assert.True(t, result.Allowed) // Soft allows
	assert.Equal(t, "soft", result.Level)
}

func TestEvaluateGateCoderSwitchAgentDifferentAgent(t *testing.T) {
	p := PresetStandard()
	ctx := &store.WorkflowContext{Stage: store.StageDeveloping, ActiveAgentID: 1}

	// Different agent (ID=2) triggers the gate
	result := EvaluateGate(p, GateCoderSwitchAgent, ctx, store.RoleCoder, 2, false)
	assert.True(t, result.Allowed) // Soft allows
	assert.Equal(t, "soft", result.Level)
	assert.Contains(t, result.Message, "切换 Agent")
}

func TestEvaluateGateCoderSwitchAgentSameAgent(t *testing.T) {
	p := PresetStandard()
	ctx := &store.WorkflowContext{Stage: store.StageDeveloping, ActiveAgentID: 1}

	// Same agent (ID=1) does NOT trigger the gate
	result := EvaluateGate(p, GateCoderSwitchAgent, ctx, store.RoleCoder, 1, false)
	assert.True(t, result.Allowed)
	assert.Equal(t, "pass", result.Level)
}

func TestEvaluateGateCoderSwitchAgentNoActiveAgent(t *testing.T) {
	p := PresetStandard()
	ctx := &store.WorkflowContext{Stage: store.StageDeveloping, ActiveAgentID: 0}

	// No active agent — gate does not trigger
	result := EvaluateGate(p, GateCoderSwitchAgent, ctx, store.RoleCoder, 1, false)
	assert.True(t, result.Allowed)
	assert.Equal(t, "pass", result.Level)
}

func TestEvaluateGateReviewWarnIfDraft(t *testing.T) {
	p := PresetStrict() // strict has review_warn_if_draft = soft
	ctx := &store.WorkflowContext{Stage: store.StageReviewing}

	// Draft PR triggers warning
	result := EvaluateGate(p, GateReviewWarnIfDraft, ctx, store.RoleReview, 0, true)
	assert.True(t, result.Allowed) // Soft allows
	assert.Equal(t, "soft", result.Level)
	assert.Contains(t, result.Message, "Draft")
}

func TestEvaluateGateReviewWarnIfNotDraft(t *testing.T) {
	p := PresetStrict()
	ctx := &store.WorkflowContext{Stage: store.StageReviewing}

	// Non-draft PR — gate passes
	result := EvaluateGate(p, GateReviewWarnIfDraft, ctx, store.RoleReview, 0, false)
	assert.True(t, result.Allowed)
	assert.Equal(t, "pass", result.Level)
}

func TestGateEvaluateResultMessages(t *testing.T) {
	// Verify all gate results have meaningful messages
	p := PresetStrict()

	tests := []struct {
		gateID     string
		stage      string
		agentID    int64
		isDraftPR  bool
	}{
		{GateCoderRequiresAnalyzed, store.StageIdle, 0, false},
		{GateReanalyzeWhileDev, store.StageDeveloping, 0, false},
		{GateRerunSameStage, store.StageDeveloping, 0, false},
		{GateCoderSwitchAgent, store.StageDeveloping, 2, false},   // agentID=2, active=1 → triggers
		{GateReviewWarnIfDraft, store.StageReviewing, 0, true},    // draft → triggers
	}

	for _, tt := range tests {
		ctx := &store.WorkflowContext{Stage: tt.stage, ActiveAgentID: 1}
		result := EvaluateGate(p, tt.gateID, ctx, store.RoleCoder, tt.agentID, tt.isDraftPR)
		assert.NotEmpty(t, result.Message, "gate %s should have message", tt.gateID)
	}
}
