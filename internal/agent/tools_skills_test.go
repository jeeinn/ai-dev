package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/sandbox"
)

func TestLoadSkillBlocksScriptsWhenDisallowed(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	content := `# demo-skill
A demo skill for tests.

## Tools
- ` + "`echo_hi`" + `
  - description: echo hello
  - script: echo hello
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	sb := sandbox.New(sandbox.Config{Mode: sandbox.ModeTemp, BaseDir: t.TempDir()}, 0)
	defer sb.Cleanup()

	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	tools := NewToolRegistry()
	err := reg.LoadSkill("demo-skill", tools, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
	_, ok := tools.Get("echo_hi")
	assert.False(t, ok)

	skill, found := reg.GetSkill("demo-skill")
	require.True(t, found)
	assert.True(t, skill.Loaded)
}

func TestLoadSkillAllowsScriptsWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	content := `# demo-skill
A demo skill for tests.

## Tools
- ` + "`echo_hi`" + `
  - description: echo hello
  - script: echo hello
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	sb := sandbox.New(sandbox.Config{Mode: sandbox.ModeTemp, BaseDir: t.TempDir()}, 0)
	defer sb.Cleanup()

	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	tools := NewToolRegistry()
	require.NoError(t, reg.LoadSkill("demo-skill", tools, true))
	_, ok := tools.Get("echo_hi")
	assert.True(t, ok)
}
