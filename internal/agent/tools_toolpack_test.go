package agent

import (
	"testing"

	"github.com/jeeinn/matea/internal/llm"
	"github.com/jeeinn/matea/internal/sandbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssembleToolRegistryCoderDefault(t *testing.T) {
	sb := sandbox.New(sandbox.Config{Mode: sandbox.ModeTemp, BaseDir: t.TempDir()}, 0)
	defer sb.Cleanup()

	registry, err := AssembleToolRegistry([]string{
		"read_file", "write_file", "list_files", "search_code", "rg",
		"run_command", "apply_diff", "tree", "git_log", "git_blame",
	}, sb)
	require.NoError(t, err)

	for _, name := range []string{
		"read_file", "write_file", "list_files", "search_code", "rg",
		"run_command", "apply_diff", "tree", "git_log", "git_blame",
	} {
		_, ok := registry.Get(name)
		assert.True(t, ok, "tool %s should be registered", name)
	}
}

func TestAssembleToolRegistryAnalyzeReadonly(t *testing.T) {
	sb := sandbox.New(sandbox.Config{Mode: sandbox.ModeTemp, BaseDir: t.TempDir()}, 0)
	defer sb.Cleanup()

	registry, err := AssembleToolRegistry([]string{
		"list_files", "rg", "search_code", "read_file", "tree", "git_log",
	}, sb)
	require.NoError(t, err)

	for _, name := range []string{"list_files", "rg", "search_code", "read_file", "tree", "git_log"} {
		_, ok := registry.Get(name)
		assert.True(t, ok, "tool %s should be registered", name)
	}

	// write tools must NOT be present
	for _, name := range []string{"write_file", "run_command", "apply_diff", "git_blame"} {
		_, ok := registry.Get(name)
		assert.False(t, ok, "tool %s should NOT be registered", name)
	}
}

func TestAssembleToolRegistryUnknownTool(t *testing.T) {
	sb := sandbox.New(sandbox.Config{Mode: sandbox.ModeTemp, BaseDir: t.TempDir()}, 0)
	defer sb.Cleanup()

	_, err := AssembleToolRegistry([]string{"read_file", "nonexistent_tool"}, sb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
	assert.Contains(t, err.Error(), "nonexistent_tool")
}

func TestAssembleToolRegistryEmpty(t *testing.T) {
	sb := sandbox.New(sandbox.Config{Mode: sandbox.ModeTemp, BaseDir: t.TempDir()}, 0)
	defer sb.Cleanup()

	registry, err := AssembleToolRegistry([]string{}, sb)
	require.NoError(t, err)
	assert.Len(t, registry.ToLLMTools(), 0)
}

func TestDefaultToolsRegression(t *testing.T) {
	sb := sandbox.New(sandbox.Config{Mode: sandbox.ModeTemp, BaseDir: t.TempDir()}, 0)
	defer sb.Cleanup()

	// DefaultTools must register the coder-default pack (incl. rg)
	registry := DefaultTools(sb)
	for _, name := range []string{
		"read_file", "write_file", "list_files", "search_code", "rg",
		"run_command", "apply_diff", "tree", "git_log", "git_blame",
	} {
		_, ok := registry.Get(name)
		assert.True(t, ok, "DefaultTools must include %s", name)
	}
}

func TestKnownToolNames(t *testing.T) {
	names := KnownToolNames()
	require.Len(t, names, 10)
	assert.Equal(t, []string{
		"apply_diff", "git_blame", "git_log", "list_files",
		"read_file", "rg", "run_command", "search_code", "tree", "write_file",
	}, names)
}

func TestToolDefToLLMTool(t *testing.T) {
	tool := &ToolDef{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"arg": {Type: "string", Description: "An argument"},
			},
			Required: []string{"arg"},
		},
	}
	llmTool := tool.ToLLMTool()
	assert.Equal(t, "function", llmTool.Type)
	assert.Equal(t, "test_tool", llmTool.Function.Name)
	assert.Equal(t, "A test tool", llmTool.Function.Description)
}
