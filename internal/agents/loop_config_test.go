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
		MaxTokens:         4096,
		Timeout:           "5m",
		TotalTimeout:      "30m",
		IterationInterval: 0,
	}
	agent := &store.AgentLoopConfig{
		MaxIterations:     8,
		IterationInterval: 5,
	}

	merged := MergeLoopConfig(agent, defaults)
	assert.Equal(t, 8, merged.MaxIterations)
	assert.Equal(t, 4096, merged.MaxTokens)
	assert.Equal(t, 5, merged.IterationInterval)
}
