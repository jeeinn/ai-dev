package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/sandbox"
)

func TestToolRegistry(t *testing.T) {
	registry := NewToolRegistry()

	// Register a tool
	registry.Register(&ToolDef{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"input": {Type: "string", Description: "Input value"},
			},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			return "result", nil
		},
	})

	// Get tool
	tool, ok := registry.Get("test_tool")
	assert.True(t, ok)
	assert.Equal(t, "test_tool", tool.Name)

	// Get non-existent tool
	_, ok = registry.Get("nonexistent")
	assert.False(t, ok)
}

func TestToolRegistryToLLMTools(t *testing.T) {
	registry := NewToolRegistry()

	registry.Register(&ToolDef{
		Name:        "tool1",
		Description: "Tool 1",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"p1": {Type: "string"},
			},
		},
		Fn: func(params map[string]interface{}) (string, error) { return "", nil },
	})

	registry.Register(&ToolDef{
		Name:        "tool2",
		Description: "Tool 2",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"p2": {Type: "string"},
			},
		},
		Fn: func(params map[string]interface{}) (string, error) { return "", nil },
	})

	tools := registry.ToLLMTools()
	assert.Len(t, tools, 2)
	assert.Equal(t, "function", tools[0].Type)
}

func TestExecuteTool(t *testing.T) {
	registry := NewToolRegistry()

	registry.Register(&ToolDef{
		Name:        "echo",
		Description: "Echo input",
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"message": {Type: "string", Description: "Message to echo"},
			},
			Required: []string{"message"},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			msg, _ := params["message"].(string)
			return msg, nil
		},
	})

	// Execute tool
	call := llm.ToolCall{
		ID:   "call-1",
		Type: "function",
		Function: llm.FuncCall{
			Name:      "echo",
			Arguments: `{"message": "hello"}`,
		},
	}

	result, err := registry.ExecuteTool(call)
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestExecuteToolUnknown(t *testing.T) {
	registry := NewToolRegistry()

	call := llm.ToolCall{
		ID:   "call-1",
		Type: "function",
		Function: llm.FuncCall{
			Name:      "unknown",
			Arguments: "{}",
		},
	}

	_, err := registry.ExecuteTool(call)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}

func TestDefaultTools(t *testing.T) {
	cfg := sandbox.SandboxConfig{
		Mode:           sandbox.ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 10 * time.Second,
		MaxOutput:      1024,
		MaxFileSize:    1024,
	}

	sb := sandbox.New(cfg, 1001)
	sb.Setup()
	defer sb.Cleanup()

	registry := DefaultTools(sb)

	// Verify all tools are registered
	expectedTools := []string{"read_file", "write_file", "list_files", "search_code", "run_command", "apply_diff", "tree", "git_log", "git_blame"}
	for _, name := range expectedTools {
		_, ok := registry.Get(name)
		assert.True(t, ok, "Tool %s should be registered", name)
	}
}

func TestWriteReadFile(t *testing.T) {
	cfg := sandbox.SandboxConfig{
		Mode:           sandbox.ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 10 * time.Second,
		MaxOutput:      1024,
		MaxFileSize:    1024,
	}

	sb := sandbox.New(cfg, 1002)
	sb.Setup()
	defer sb.Cleanup()

	registry := DefaultTools(sb)

	// Write file
	writeCall := llm.ToolCall{
		ID:   "call-1",
		Type: "function",
		Function: llm.FuncCall{
			Name:      "write_file",
			Arguments: `{"path": "test.txt", "content": "hello world"}`,
		},
	}

	result, err := registry.ExecuteTool(writeCall)
	require.NoError(t, err)
	assert.Contains(t, result, "successfully")

	// Read file
	readCall := llm.ToolCall{
		ID:   "call-2",
		Type: "function",
		Function: llm.FuncCall{
			Name:      "read_file",
			Arguments: `{"path": "test.txt"}`,
		},
	}

	result, err = registry.ExecuteTool(readCall)
	require.NoError(t, err)
	assert.Equal(t, "hello world", result)
}

func TestRunCommand(t *testing.T) {
	cfg := sandbox.SandboxConfig{
		Mode:           sandbox.ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 10 * time.Second,
		MaxOutput:      1024,
	}

	sb := sandbox.New(cfg, 1003)
	sb.Setup()
	defer sb.Cleanup()

	registry := DefaultTools(sb)

	// Run echo command
	call := llm.ToolCall{
		ID:   "call-1",
		Type: "function",
		Function: llm.FuncCall{
			Name:      "run_command",
			Arguments: `{"command": "echo hello"}`,
		},
	}

	result, err := registry.ExecuteTool(call)
	require.NoError(t, err)
	assert.Contains(t, result, "hello")
	assert.Contains(t, result, "Exit code: 0")
}
