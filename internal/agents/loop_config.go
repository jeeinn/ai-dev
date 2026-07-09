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
	return merged
}
