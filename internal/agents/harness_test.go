package agents

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/sandbox"
	"gitea-agent-gateway/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeLoopConfigHarnessFields(t *testing.T) {
	defaults := config.DefaultAgentLoopConfig()
	assert.Equal(t, 3, defaults.NoProgressLimit)

	off := 0
	agent := &store.AgentLoopConfig{
		NoProgressLimit: &off,
		VerifyCommands:  []string{}, // explicit empty disables
	}
	merged := MergeLoopConfig(agent, defaults)
	assert.Equal(t, 0, merged.NoProgressLimit)
	assert.NotNil(t, merged.VerifyCommands)
	assert.Len(t, merged.VerifyCommands, 0)

	on := 5
	agent2 := &store.AgentLoopConfig{
		NoProgressLimit: &on,
		VerifyCommands:  []string{"go test ./..."},
	}
	merged2 := MergeLoopConfig(agent2, defaults)
	assert.Equal(t, 5, merged2.NoProgressLimit)
	assert.Equal(t, []string{"go test ./..."}, merged2.VerifyCommands)
}

func TestRunHarnessVerifyOK(t *testing.T) {
	dir := t.TempDir()
	cfg := sandbox.DefaultSandboxConfig()
	cfg.Mode = sandbox.ModeFixed
	cfg.BaseDir = dir
	sb := sandbox.NewWithPath(cfg, 1, dir)
	require.NoError(t, sb.Setup())

	var cmds []string
	if runtime.GOOS == "windows" {
		cmds = []string{"echo ok"}
	} else {
		cmds = []string{"true"}
	}
	require.NoError(t, runHarnessVerify(sb, cmds))
}

func TestRunHarnessVerifyFails(t *testing.T) {
	dir := t.TempDir()
	cfg := sandbox.DefaultSandboxConfig()
	cfg.Mode = sandbox.ModeFixed
	cfg.BaseDir = dir
	sb := sandbox.NewWithPath(cfg, 2, dir)
	require.NoError(t, sb.Setup())

	var cmds []string
	if runtime.GOOS == "windows" {
		cmds = []string{"cmd /C exit 1"}
	} else {
		cmds = []string{"false"}
	}
	err := runHarnessVerify(sb, cmds)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify gate failed")
}

func TestWorkspaceProgressSnapshotChanges(t *testing.T) {
	dir := t.TempDir()
	cfg := sandbox.DefaultSandboxConfig()
	cfg.Mode = sandbox.ModeFixed
	cfg.BaseDir = dir
	sb := sandbox.NewWithPath(cfg, 3, dir)
	require.NoError(t, sb.Setup())

	// init git repo
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = sb.WorkDir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init")
	run("config", "user.email", "t@t.com")
	run("config", "user.name", "t")
	require.NoError(t, os.WriteFile(filepath.Join(sb.WorkDir, "a.txt"), []byte("a"), 0644))
	run("add", "a.txt")
	run("commit", "-m", "init")

	s1 := workspaceProgressSnapshot(sb)
	require.NoError(t, os.WriteFile(filepath.Join(sb.WorkDir, "b.txt"), []byte("b"), 0644))
	s2 := workspaceProgressSnapshot(sb)
	assert.NotEqual(t, s1, s2)
}
