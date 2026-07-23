package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeeinn/matea/internal/sandbox"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestSandbox(t *testing.T) *sandbox.Sandbox {
	t.Helper()
	sb := sandbox.New(sandbox.Config{Mode: sandbox.ModeTemp, BaseDir: t.TempDir()}, 0)
	t.Cleanup(func() { sb.Cleanup() })
	return sb
}

// writeSkillFile creates a skill file at the given path, ensuring parent dirs.
func writeSkillFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// ---------------------------------------------------------------------------
// Frontmatter parsing
// ---------------------------------------------------------------------------

func TestParseFrontmatterSkill(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skills", "deploy.md")
	content := `---
name: deploy
description: Deploy the application
tools:
  - name: deploy_app
    description: Deploy to target
    script: "deploy.sh {{env}}"
    parameters:
      - name: env
        type: string
        description: Target environment
        required: true
---

# Deploy Skill

When deploying, always run tests first.
Check the env parameter matches an allowed value.
`
	writeSkillFile(t, path, content)

	skill, err := parseSkillFile(path)
	require.NoError(t, err)

	assert.Equal(t, "deploy", skill.Name)
	assert.Equal(t, "Deploy the application", skill.Description)
	assert.Equal(t, "# Deploy Skill\n\nWhen deploying, always run tests first.\nCheck the env parameter matches an allowed value.", skill.Body)

	require.Len(t, skill.Tools, 1)
	tool := skill.Tools[0]
	assert.Equal(t, "deploy_app", tool.Name)
	assert.Equal(t, "Deploy to target", tool.Description)
	assert.Equal(t, "deploy.sh {{env}}", tool.Script)
	require.Len(t, tool.Parameters, 1)
	assert.Equal(t, "env", tool.Parameters[0].Name)
	assert.Equal(t, "string", tool.Parameters[0].Type)
	assert.Equal(t, []string{"env"}, tool.Required)
}

func TestParseFrontmatterSkillNameFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skills", "my-tool.md")
	content := `---
description: A skill without name
---
Body here.
`
	writeSkillFile(t, path, content)

	skill, err := parseSkillFile(path)
	require.NoError(t, err)
	// Name falls back to filename without extension
	assert.Equal(t, "my-tool", skill.Name)
	assert.Equal(t, "A skill without name", skill.Description)
}

func TestParseLegacySkill(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `# legacy-skill
A legacy skill for backward compat.

## Tools
- ` + "`echo_hi`" + `
  - description: echo hello
  - script: echo hello
`
	writeSkillFile(t, path, content)

	skill, err := parseSkillFile(path)
	require.NoError(t, err)

	assert.Equal(t, "legacy-skill", skill.Name)
	assert.Equal(t, "A legacy skill for backward compat.", skill.Description)
	require.Len(t, skill.Tools, 1)
	assert.Equal(t, "echo_hi", skill.Tools[0].Name)
}

// ---------------------------------------------------------------------------
// Scan roots: skills/ / .agents/skills/ / legacy SKILL.md
// ---------------------------------------------------------------------------

func TestScanSkillsDirectory(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "skills", "foo.md"), `---
name: foo
description: Foo skill
---
Foo body.
`)
	writeSkillFile(t, filepath.Join(dir, "skills", "bar.md"), `---
name: bar
description: Bar skill
---
Bar body.
`)

	sb := newTestSandbox(t)
	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	skills := reg.ListSkills()
	assert.Len(t, skills, 2)

	foo, ok := reg.GetSkill("foo")
	require.True(t, ok)
	assert.Equal(t, "Foo skill", foo.Description)
	assert.Equal(t, "Foo body.", foo.Body)

	bar, ok := reg.GetSkill("bar")
	require.True(t, ok)
	assert.Equal(t, "Bar skill", bar.Description)
}

func TestScanAgentsSkillsDirectory(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, ".agents", "skills", "review.md"), `---
name: code-review
description: Code review skill
---
Review body.
`)

	sb := newTestSandbox(t)
	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	skill, ok := reg.GetSkill("code-review")
	require.True(t, ok)
	assert.Equal(t, "Code review skill", skill.Description)
	assert.Equal(t, "Review body.", skill.Body)
}

func TestScanSubdirectoryPattern(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "skills", "my-skill", "SKILL.md"), `---
name: nested-skill
description: Nested in subdirectory
---
Nested body.
`)

	sb := newTestSandbox(t)
	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	skill, ok := reg.GetSkill("nested-skill")
	require.True(t, ok)
	assert.Equal(t, "Nested in subdirectory", skill.Description)
}

func TestScanLegacyRootSKILLMd(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "SKILL.md"), `# root-skill
Root description.
`)

	sb := newTestSandbox(t)
	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	skill, ok := reg.GetSkill("root-skill")
	require.True(t, ok)
	assert.Equal(t, "Root description.", skill.Description)
}

func TestScanPriorityDedup(t *testing.T) {
	// skills/ takes priority over .agents/skills/ for same name
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "skills", "dup.md"), `---
name: dup
description: From skills/
---
Skills body.
`)
	writeSkillFile(t, filepath.Join(dir, ".agents", "skills", "dup.md"), `---
name: dup
description: From .agents/skills/
---
Agents body.
`)

	sb := newTestSandbox(t)
	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	skill, ok := reg.GetSkill("dup")
	require.True(t, ok)
	assert.Equal(t, "From skills/", skill.Description)
}

// ---------------------------------------------------------------------------
// Load + allowScripts
// ---------------------------------------------------------------------------

func TestLoadSkillBlocksScriptsWhenDisallowed(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "skills", "demo.md"), `---
name: demo-skill
description: A demo skill for tests
tools:
  - name: echo_hi
    description: echo hello
    script: echo hello
---
Demo body.
`)

	sb := newTestSandbox(t)
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
	writeSkillFile(t, filepath.Join(dir, "skills", "demo.md"), `---
name: demo-skill
description: A demo skill for tests
tools:
  - name: echo_hi
    description: echo hello
    script: echo hello
---
Demo body.
`)

	sb := newTestSandbox(t)
	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	tools := NewToolRegistry()
	require.NoError(t, reg.LoadSkill("demo-skill", tools, true))
	_, ok := tools.Get("echo_hi")
	assert.True(t, ok)
}

// ---------------------------------------------------------------------------
// Progressive disclosure: list_skills vs load_skill
// ---------------------------------------------------------------------------

func TestListSkillsOnlyReturnsNameAndDescription(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "skills", "deploy.md"), `---
name: deploy
description: Deploy the app
tools:
  - name: deploy_app
    description: Deploy to target
    script: deploy.sh
---
Detailed deployment instructions.
`)

	sb := newTestSandbox(t)
	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	tool := NewListSkillsTool(reg)
	result, err := tool.Fn(map[string]interface{}{})
	require.NoError(t, err)

	// list_skills should include name and description
	assert.Contains(t, result, "deploy")
	assert.Contains(t, result, "Deploy the app")
	// But NOT tool details or body
	assert.NotContains(t, result, "deploy_app")
	assert.NotContains(t, result, "Detailed deployment instructions")
}

func TestLoadSkillInjectsBody(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "skills", "deploy.md"), `---
name: deploy
description: Deploy the app
tools:
  - name: deploy_app
    description: Deploy to target
    script: deploy.sh
---
Detailed deployment instructions here.
`)

	sb := newTestSandbox(t)
	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	tools := NewToolRegistry()
	tool := NewLoadSkillTool(reg, tools, true)
	result, err := tool.Fn(map[string]interface{}{"name": "deploy"})
	require.NoError(t, err)

	// load_skill should include body instructions
	assert.Contains(t, result, "Detailed deployment instructions here.")
	assert.Contains(t, result, "Skill Instructions")
	// And tool list
	assert.Contains(t, result, "deploy_app")
}

func TestLoadSkillAnalyzeModeInjectsBodyWithoutTools(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, filepath.Join(dir, "skills", "deploy.md"), `---
name: deploy
description: Deploy the app
tools:
  - name: deploy_app
    description: Deploy to target
    script: deploy.sh
---
Instructions for analyze mode.
`)

	sb := newTestSandbox(t)
	reg := NewSkillRegistry(sb, dir)
	require.NoError(t, reg.ScanSkills())

	tools := NewToolRegistry()
	tool := NewLoadSkillTool(reg, tools, false)
	result, err := tool.Fn(map[string]interface{}{"name": "deploy"})
	require.NoError(t, err)

	// Analyze mode: body is injected, but scripts blocked
	assert.Contains(t, result, "Instructions for analyze mode.")
	assert.Contains(t, result, "scripts blocked")
	assert.Contains(t, result, "Skill Instructions")
}
