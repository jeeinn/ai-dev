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

	result := EvaluateGate(p, GateCoderRequiresAnalyzed, ctx, store.RoleCoder)
	assert.True(t, result.Allowed)
	assert.Equal(t, "pass", result.Level)
}

func TestEvaluateGateCoderRequiresAnalyzedIdle(t *testing.T) {
	p := PresetStrict()
	ctx := &store.WorkflowContext{Stage: store.StageIdle}

	result := EvaluateGate(p, GateCoderRequiresAnalyzed, ctx, store.RoleCoder)
	assert.False(t, result.Allowed)
	assert.Equal(t, "hard", result.Level)
	assert.Contains(t, result.Message, "需求分析")
}

func TestEvaluateGateCoderRequiresAnalyzedAnalyzed(t *testing.T) {
	p := PresetStrict()
	ctx := &store.WorkflowContext{Stage: store.StageAnalyzed}

	result := EvaluateGate(p, GateCoderRequiresAnalyzed, ctx, store.RoleCoder)
	assert.True(t, result.Allowed)
	assert.Equal(t, "pass", result.Level)
}

func TestEvaluateGateReanalyzeWhileDev(t *testing.T) {
	p := PresetStandard()
	ctx := &store.WorkflowContext{Stage: store.StageDeveloping}

	result := EvaluateGate(p, GateReanalyzeWhileDev, ctx, store.RoleAnalyze)
	assert.True(t, result.Allowed) // Soft allows
	assert.Equal(t, "soft", result.Level)
	assert.Contains(t, result.Message, "开发阶段")
}

func TestEvaluateGateReanalyzeWhileDevStrict(t *testing.T) {
	p := PresetStrict()
	ctx := &store.WorkflowContext{Stage: store.StageDeveloping}

	result := EvaluateGate(p, GateReanalyzeWhileDev, ctx, store.RoleAnalyze)
	assert.False(t, result.Allowed) // Hard blocks
	assert.Equal(t, "hard", result.Level)
}

func TestEvaluateGateRerunSameStage(t *testing.T) {
	p := PresetStandard()
	ctx := &store.WorkflowContext{Stage: store.StageDeveloping}

	result := EvaluateGate(p, GateRerunSameStage, ctx, store.RoleCoder)
	assert.True(t, result.Allowed) // Soft allows
	assert.Equal(t, "soft", result.Level)
}

func TestEvaluateGateCoderSwitchAgent(t *testing.T) {
	p := PresetStandard()
	ctx := &store.WorkflowContext{Stage: store.StageDeveloping}

	result := EvaluateGate(p, GateCoderSwitchAgent, ctx, store.RoleCoder)
	assert.True(t, result.Allowed) // Soft allows
	assert.Equal(t, "soft", result.Level)
	assert.Contains(t, result.Message, "切换 Agent")
}

func TestGateEvaluateResultMessages(t *testing.T) {
	// Verify all gate results have meaningful messages
	p := PresetStrict()

	tests := []struct {
		gateID string
		stage  string
	}{
		{GateCoderRequiresAnalyzed, store.StageIdle},
		{GateReanalyzeWhileDev, store.StageDeveloping},
		{GateRerunSameStage, store.StageDeveloping},
		{GateCoderSwitchAgent, store.StageDeveloping},
	}

	for _, tt := range tests {
		ctx := &store.WorkflowContext{Stage: tt.stage}
		result := EvaluateGate(p, tt.gateID, ctx, store.RoleCoder)
		assert.NotEmpty(t, result.Message, "gate %s should have message", tt.gateID)
	}
}
