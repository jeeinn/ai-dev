package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAgentLoopConfig(t *testing.T) {
	assert.NoError(t, ValidateAgentLoopConfig(AgentLoopConfig{MaxIterations: 20, TotalTimeout: "30m"}))
	assert.NoError(t, ValidateAgentLoopConfig(AgentLoopConfig{MaxIterations: 1, TotalTimeout: "1m"}))
	assert.NoError(t, ValidateAgentLoopConfig(AgentLoopConfig{MaxIterations: 100, TotalTimeout: "1h"}))

	assert.Error(t, ValidateAgentLoopConfig(AgentLoopConfig{MaxIterations: 0, TotalTimeout: "30m"}))
	assert.Error(t, ValidateAgentLoopConfig(AgentLoopConfig{MaxIterations: 101, TotalTimeout: "30m"}))
	assert.Error(t, ValidateAgentLoopConfig(AgentLoopConfig{MaxIterations: 20, TotalTimeout: "30s"}))
	assert.Error(t, ValidateAgentLoopConfig(AgentLoopConfig{MaxIterations: 20, TotalTimeout: "2h"}))
	assert.Error(t, ValidateAgentLoopConfig(AgentLoopConfig{MaxIterations: 20, TotalTimeout: "not-a-duration"}))
}

func TestAlignWorkspacePathsInheritsSandboxBaseDir(t *testing.T) {
	cfg := &Config{
		Workspace: WorkspaceConfig{BaseDir: "./data/work"},
		Sandbox:   SandboxConfig{BaseDir: "./workspace"}, // legacy default
	}
	applySandboxDefaults(&cfg.Sandbox)
	alignWorkspacePaths(cfg)
	assert.Equal(t, "./data/work", cfg.Sandbox.BaseDir)
}

func TestAlignWorkspacePathsPreservesExplicitSandbox(t *testing.T) {
	cfg := &Config{
		Workspace: WorkspaceConfig{BaseDir: "./data/work"},
		Sandbox:   SandboxConfig{BaseDir: "./data/sandbox-custom"},
	}
	alignWorkspacePaths(cfg)
	assert.Equal(t, "./data/sandbox-custom", cfg.Sandbox.BaseDir)
}

func TestLoadRejectsInvalidLoopConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
agents:
  loop:
    max_iterations: 999
    total_timeout: "30m"
`), 0644))
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_iterations")
}

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
