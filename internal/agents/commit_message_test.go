package agents

import (
	"context"
	"testing"
	"time"

	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/sandbox"
	"gitea-agent-gateway/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitScopeFromFiles(t *testing.T) {
	assert.Equal(t, "config", commitScope([]string{"internal/config/schema.go"}))
	assert.Equal(t, "config", commitScope([]string{
		"internal/config/schema.go",
		"internal/config/manager.go",
		"internal/sandbox/sandbox.go",
	}))
	assert.Equal(t, "sandbox", commitScope([]string{"internal/sandbox/sandbox.go"}))
}

func TestFormatConventionalCommit(t *testing.T) {
	msg := formatConventionalCommit("fix", "schema", "add json tags and restore trailing newline")
	assert.Equal(t, "fix(schema): add json tags and restore trailing newline", msg)

	long := formatConventionalCommit("feat", "config", string(make([]byte, 100)))
	assert.LessOrEqual(t, len([]rune(long)), commitSubjectMaxLen)
}

func TestFallbackSubject(t *testing.T) {
	subject := fallbackSubject([]string{"internal/config/schema.go"}, " 1 file changed", "")
	assert.Equal(t, "update schema", subject)

	subject = fallbackSubject([]string{
		"internal/config/schema.go",
		"internal/config/manager.go",
	}, "", "")
	assert.Contains(t, subject, "config")
	assert.Contains(t, subject, "other file")
}

func TestParseLLMCommitSubject(t *testing.T) {
	assert.Equal(t,
		"fix(schema): add json tags and restore trailing newline",
		parseLLMCommitSubject("fix(schema): add json tags and restore trailing newline", "feat", "config"),
	)
	assert.Equal(t,
		"fix(config): add json tags",
		parseLLMCommitSubject("add json tags", "fix", "config"),
	)
}

func TestGenerateCommitMessageFallbackWithoutLLM(t *testing.T) {
	s, git := setupCommitTestRepo(t)

	require.NoError(t, s.WriteFile("internal/config/schema.go", []byte("package config\n")))
	git.Add()

	msg := GenerateCommitMessage(context.Background(), CommitMessageInput{
		Git:         git,
		TaskSubType: "dev",
		Task:        &store.Task{Event: "Fix review feedback"},
	})
	assert.Contains(t, msg, "feat(config):")
	assert.Contains(t, msg, "schema")
}

type commitSummaryProvider struct {
	response string
}

func (p *commitSummaryProvider) ChatCompletion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: p.response}, nil
}

func TestGenerateCommitMessageWithLLM(t *testing.T) {
	s, git := setupCommitTestRepo(t)

	require.NoError(t, s.WriteFile("internal/config/schema.go", []byte("package config\n// tags\n")))
	git.Add()

	provider := &commitSummaryProvider{
		response: "fix(schema): add json tags and restore trailing newline",
	}
	msg := GenerateCommitMessage(context.Background(), CommitMessageInput{
		Git:          git,
		Provider:     provider,
		Model:        "mock",
		TaskSubType:  "bugfix",
		Task:         &store.Task{Event: "Review fixes"},
		AgentSummary: "Added json tags to ProviderConfig fields.",
	})
	assert.Equal(t, "fix(schema): add json tags and restore trailing newline", msg)
}

func setupCommitTestRepo(t *testing.T) (*sandbox.Sandbox, *sandbox.Git) {
	t.Helper()
	cfg := sandbox.SandboxConfig{
		Mode:           sandbox.ModeFixed,
		BaseDir:        t.TempDir(),
		CommandTimeout: 30 * time.Second,
		MaxOutput:      4096,
		MaxFileSize:    4096,
	}
	s := sandbox.New(cfg, 4001)
	require.NoError(t, s.Setup())
	t.Cleanup(func() { s.Cleanup() })

	git := sandbox.NewGit(s)
	s.Execute("git", "init")
	s.Execute("git", "config", "user.email", "test@test.com")
	s.Execute("git", "config", "user.name", "Test")
	require.NoError(t, s.WriteFile("README.md", []byte("hi")))
	git.Add()
	require.NoError(t, git.Commit("init").Error)
	return s, git
}
