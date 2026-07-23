package agents

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/jeeinn/matea/internal/llm"
	"github.com/jeeinn/matea/internal/sandbox"
	"github.com/jeeinn/matea/internal/store"
)

const commitSubjectMaxLen = 72

var conventionalCommitPattern = regexp.MustCompile(`^(feat|fix|docs|chore|refactor|test|style|perf|ci|build)(\([a-z0-9._-]+\))?: .+`)

// CommitMessageInput configures commit message generation after staging.
type CommitMessageInput struct {
	Git          *sandbox.Git
	Provider     llm.Provider
	Model        string
	Temperature  float64
	TaskSubType  string
	Task         *store.Task
	AgentSummary string
}

// GenerateCommitMessage builds a conventional commit subject from staged diff and optional LLM summary.
func GenerateCommitMessage(ctx context.Context, in CommitMessageInput) string {
	stat := stagedDiffStat(in.Git)
	files := stagedChangedFiles(in.Git)
	commitType := commitTypeFromSubType(in.TaskSubType)
	scope := commitScope(files)
	taskTitle := ""
	if in.Task != nil {
		taskTitle = in.Task.Event
	}
	fallback := formatConventionalCommit(commitType, scope, fallbackSubject(files, stat, taskTitle))

	if in.Provider == nil || strings.TrimSpace(in.Model) == "" {
		return fallback
	}

	summary, err := summarizeCommitWithLLM(ctx, in, stat, files, commitType, scope)
	if err != nil {
		log.Printf("[WARN] LLM commit summary failed, using diff fallback: %v", err)
		return fallback
	}
	if msg := parseLLMCommitSubject(summary, commitType, scope); msg != "" {
		return msg
	}
	return fallback
}

func stagedDiffStat(git *sandbox.Git) string {
	if git == nil {
		return ""
	}
	result := git.DiffCachedStat()
	if result.Error != nil {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func stagedChangedFiles(git *sandbox.Git) []string {
	if git == nil {
		return nil
	}
	result := git.DiffCachedNameStatus()
	if result.Error != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			files = append(files, filepath.ToSlash(parts[len(parts)-1]))
		}
	}
	return files
}

func commitTypeFromSubType(taskSubType string) string {
	if taskSubType == "bugfix" {
		return "fix"
	}
	return "feat"
}

func commitScope(files []string) string {
	if len(files) == 0 {
		return ""
	}
	counts := make(map[string]int)
	for _, f := range files {
		scope := fileCommitScope(f)
		if scope != "" {
			counts[scope]++
		}
	}
	if len(counts) == 0 {
		return ""
	}
	type scopeCount struct {
		scope string
		count int
	}
	ranked := make([]scopeCount, 0, len(counts))
	for s, c := range counts {
		ranked = append(ranked, scopeCount{s, c})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].scope < ranked[j].scope
	})
	return ranked[0].scope
}

func fileCommitScope(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[0] == "internal" {
		return parts[1]
	}
	if len(parts) >= 2 && parts[0] == "pkg" {
		return parts[1]
	}
	if len(parts) >= 2 {
		return parts[0]
	}
	base := filepath.Base(path)
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return strings.ToLower(base)
}

func fallbackSubject(files []string, stat, taskTitle string) string {
	if len(files) == 1 {
		base := filepath.Base(files[0])
		if ext := filepath.Ext(base); ext != "" {
			base = strings.TrimSuffix(base, ext)
		}
		return fmt.Sprintf("update %s", base)
	}
	if scope := commitScope(files); scope != "" {
		if len(files) > 1 {
			return fmt.Sprintf("update %s and %d other files", scope, len(files)-1)
		}
		return fmt.Sprintf("update %s", scope)
	}
	title := strings.TrimSpace(taskTitle)
	if title != "" {
		title = truncateRunes(title, 50)
		return fmt.Sprintf("apply changes for %s", title)
	}
	if stat != "" {
		lines := strings.Split(stat, "\n")
		if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
			return "apply staged changes"
		}
	}
	return "apply AI code changes"
}

func formatConventionalCommit(commitType, scope, subject string) string {
	subject = normalizeSubject(subject)
	if scope != "" {
		msg := fmt.Sprintf("%s(%s): %s", commitType, scope, subject)
		return truncateRunes(msg, commitSubjectMaxLen)
	}
	msg := fmt.Sprintf("%s: %s", commitType, subject)
	return truncateRunes(msg, commitSubjectMaxLen)
}

func normalizeSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	subject = strings.Trim(subject, `"'`)
	if subject == "" {
		return "apply changes"
	}
	runes := []rune(subject)
	if len(runes) > 0 {
		runes[0] = unicode.ToLower(runes[0])
	}
	return string(runes)
}

func summarizeCommitWithLLM(ctx context.Context, in CommitMessageInput, stat string, files []string, commitType, scope string) (string, error) {
	taskTitle := ""
	if in.Task != nil {
		taskTitle = in.Task.Event
	}
	agentSummary := truncateRunes(strings.TrimSpace(in.AgentSummary), 500)

	var sb strings.Builder
	sb.WriteString("Write exactly ONE conventional commit subject line (max 72 characters).\n")
	sb.WriteString("Format: type(scope): imperative description\n")
	sb.WriteString(fmt.Sprintf("Prefer type %q", commitType))
	if scope != "" {
		sb.WriteString(fmt.Sprintf(" and scope %q", scope))
	}
	sb.WriteString(".\n")
	sb.WriteString("Use lowercase after the colon. No body, no quotes, no markdown.\n\n")
	if taskTitle != "" {
		sb.WriteString("Task: ")
		sb.WriteString(taskTitle)
		sb.WriteString("\n")
	}
	if agentSummary != "" {
		sb.WriteString("Agent summary: ")
		sb.WriteString(agentSummary)
		sb.WriteString("\n")
	}
	if len(files) > 0 {
		sb.WriteString("Changed files: ")
		sb.WriteString(strings.Join(files, ", "))
		sb.WriteString("\n")
	}
	if stat != "" {
		sb.WriteString("\nStaged diff stat:\n")
		sb.WriteString(stat)
	}

	temp := in.Temperature
	if temp <= 0 {
		temp = 0.2
	}

	resp, err := in.Provider.ChatCompletion(ctx, &llm.ChatRequest{
		Model: in.Model,
		Messages: []llm.Message{
			{Role: "system", Content: "You write concise git commit subject lines only."},
			{Role: "user", Content: sb.String()},
		},
		MaxTokens:   80,
		Temperature: temp,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func parseLLMCommitSubject(raw, commitType, scope string) string {
	line := firstNonEmptyLine(raw)
	if line == "" {
		return ""
	}
	line = truncateRunes(line, commitSubjectMaxLen)
	if conventionalCommitPattern.MatchString(line) {
		return line
	}
	if idx := strings.Index(line, ": "); idx > 0 {
		prefix := strings.TrimSpace(line[:idx])
		switch prefix {
		case "feat", "fix", "docs", "chore", "refactor", "test", "style", "perf", "ci", "build":
			return line
		}
	}
	return formatConventionalCommit(commitType, scope, line)
}

func firstNonEmptyLine(raw string) string {
	raw = strings.TrimSpace(strings.Trim(raw, "`"))
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		if line != "" {
			return line
		}
	}
	return ""
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}
