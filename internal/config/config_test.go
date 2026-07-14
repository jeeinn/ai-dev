package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyBackendDefaultsSetsInternal(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	assert.Equal(t, "internal", cfg.Agents.Backends.Default)
	assert.Contains(t, cfg.Agents.Backends.Backends, "internal")
	assert.Equal(t, BackendTypeBuiltin, cfg.Agents.Backends.Backends["internal"].Type)
}

func TestApplyBackendDefaultsPreservesExplicitDefault(t *testing.T) {
	cfg := &Config{
		Agents: AgentsConfig{
			Backends: AgentBackendsConfig{
				Default: "opencode-local",
				Backends: map[string]BackendConfig{
					"opencode-local": {Type: BackendTypeOpenCodeHTTP, BaseURL: "http://127.0.0.1:4096"},
				},
			},
		},
	}
	applyDefaults(cfg)

	// Explicit default preserved; internal still ensured
	assert.Equal(t, "opencode-local", cfg.Agents.Backends.Default)
	assert.Contains(t, cfg.Agents.Backends.Backends, "internal")
	assert.Contains(t, cfg.Agents.Backends.Backends, "opencode-local")
}

func TestApplyBackendDefaultsBackfillsInternalType(t *testing.T) {
	cfg := &Config{
		Agents: AgentsConfig{
			Backends: AgentBackendsConfig{
				Backends: map[string]BackendConfig{
					"internal": {}, // type empty
				},
			},
		},
	}
	applyDefaults(cfg)

	assert.Equal(t, "internal", cfg.Agents.Backends.Default)
	assert.Equal(t, BackendTypeBuiltin, cfg.Agents.Backends.Backends["internal"].Type)
}
