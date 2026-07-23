package agents

import (
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/store"
)

// MergeLoopConfig merges per-agent loop overrides onto system defaults.
func MergeLoopConfig(agentLoop *store.AgentLoopConfig, defaults config.AgentLoopConfig) config.AgentLoopConfig {
	merged := defaults
	if agentLoop == nil {
		return merged
	}
	if agentLoop.MaxIterations > 0 {
		merged.MaxIterations = agentLoop.MaxIterations
	}
	if agentLoop.TotalTimeout != "" {
		merged.TotalTimeout = agentLoop.TotalTimeout
	}
	if agentLoop.IterationInterval > 0 {
		merged.IterationInterval = agentLoop.IterationInterval
	}
	if agentLoop.NoProgressLimit != nil {
		merged.NoProgressLimit = *agentLoop.NoProgressLimit
	}
	// Non-nil slice (including empty) overrides defaults so agents can disable verify.
	if agentLoop.VerifyCommands != nil {
		merged.VerifyCommands = append([]string{}, agentLoop.VerifyCommands...)
	}
	if agentLoop.IndependentChecker != nil {
		merged.IndependentChecker = *agentLoop.IndependentChecker
	}
	return merged
}
