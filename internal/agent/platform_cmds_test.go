package agent

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/sandbox"
)

func TestPlatformCmdsSelectByOS(t *testing.T) {
	cmd, args := listFilesCmd(".")
	require.NotEmpty(t, cmd)
	require.NotEmpty(t, args)

	if runtime.GOOS == "windows" {
		assert.Equal(t, "powershell", cmd)
		assert.Contains(t, args, "-Command")
	} else {
		assert.Equal(t, "find", cmd)
		assert.Contains(t, args, "-maxdepth")
	}

	scmd, sargs := searchCodeCmd("package", ".")
	if runtime.GOOS == "windows" {
		assert.Equal(t, "powershell", scmd)
	} else {
		assert.Equal(t, "grep", scmd)
		assert.Contains(t, sargs, "package")
	}

	tcmd, _ := treeCmd(".", 2)
	if runtime.GOOS == "windows" {
		assert.Equal(t, "powershell", tcmd)
	} else {
		assert.Equal(t, "find", tcmd)
	}
}

func TestListFilesAndSearchCrossPlatform(t *testing.T) {
	cfg := sandbox.SandboxConfig{
		Mode:           sandbox.ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: sandbox.DefaultSandboxConfig().CommandTimeout,
		MaxOutput:      1024 * 1024,
		MaxFileSize:    1024 * 1024,
	}
	sb := sandbox.New(cfg, 9001)
	require.NoError(t, sb.Setup())
	t.Cleanup(func() { _ = sb.Cleanup() })

	require.NoError(t, sb.WriteFile("hello.go", []byte("package hello\n")))
	require.NoError(t, sb.WriteFile("subdir/note.md", []byte("note\n")))

	reg := DefaultTools(sb)

	listOut, err := reg.ExecuteTool(llm.ToolCall{
		Function: llm.FuncCall{Name: "list_files", Arguments: `{"path":"."}`},
	})
	require.NoError(t, err)
	assert.Contains(t, listOut, "hello.go")

	searchOut, err := reg.ExecuteTool(llm.ToolCall{
		Function: llm.FuncCall{Name: "search_code", Arguments: `{"pattern":"package","path":"."}`},
	})
	require.NoError(t, err)
	assert.Contains(t, searchOut, "hello.go")

	treeOut, err := reg.ExecuteTool(llm.ToolCall{
		Function: llm.FuncCall{Name: "tree", Arguments: `{"path":".","depth":2}`},
	})
	require.NoError(t, err)
	assert.NotContains(t, treeOut, "Error:")
	assert.True(t, len(treeOut) > 0)

	echoCmd := "echo hello"
	if runtime.GOOS == "windows" {
		echoCmd = "echo hello"
	}
	runOut, err := reg.ExecuteTool(llm.ToolCall{
		Function: llm.FuncCall{Name: "run_command", Arguments: `{"command":"` + echoCmd + `"}`},
	})
	require.NoError(t, err)
	assert.Contains(t, runOut, "hello")
	assert.Contains(t, runOut, "Exit code: 0")
}

func TestEscapePSSingle(t *testing.T) {
	assert.Equal(t, "a''b", escapePSSingle("a'b"))
}
