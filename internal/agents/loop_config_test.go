package agents

import (
	"testing"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/store"

	"github.com/stretchr/testify/assert"
)

func TestMergeLoopConfigUsesAgentOverrides(t *testing.T) {
	defaults := config.AgentLoopConfig{
		MaxIterations:     20,
		TotalTimeout:      "30m",
		IterationInterval: 0,
	}
	agent := &store.AgentLoopConfig{
		MaxIterations:     8,
		TotalTimeout:      "45m",
		IterationInterval: 5,
	}

	merged := MergeLoopConfig(agent, defaults)
	assert.Equal(t, 8, merged.MaxIterations)
	assert.Equal(t, "45m", merged.TotalTimeout)
	assert.Equal(t, 5, merged.IterationInterval)
}

func TestMergeLoopConfigNilAgent(t *testing.T) {
	defaults := config.DefaultAgentLoopConfig()
	merged := MergeLoopConfig(nil, defaults)
	assert.Equal(t, defaults, merged)
}
