package sandbox

import (
	"fmt"
	"strings"
	"time"

	"github.com/jeeinn/matea/internal/store"
)

// AuditLogger logs command executions to the database.
type AuditLogger struct {
	db      *store.DB
	taskID  int64
	agentID int64
}

// NewAuditLogger creates a new audit logger for a task.
func NewAuditLogger(db *store.DB, taskID, agentID int64) *AuditLogger {
	return &AuditLogger{
		db:      db,
		taskID:  taskID,
		agentID: agentID,
	}
}

// LogCommand logs a command execution to the database.
// Safe to call when db is nil (no-op).
func (l *AuditLogger) LogCommand(command string, args []string, result *Result) {
	if l.db == nil {
		return
	}
	// Truncate output for logging
	stdout := truncateString(result.Stdout, 1000)
	stderr := truncateString(result.Stderr, 1000)

	detail := formatLogDetail(command, args, result.ExitCode, stdout, stderr, result.Duration)
	l.db.LogOperation(l.agentID, l.taskID, "command", detail)
}

// LogAction logs a high-level action to the database.
// Safe to call when db is nil (no-op).
func (l *AuditLogger) LogAction(action, detail string) {
	if l.db == nil {
		return
	}
	l.db.LogOperation(l.agentID, l.taskID, action, detail)
}

// formatLogDetail formats command execution details for logging.
func formatLogDetail(command string, args []string, exitCode int, stdout, stderr string, duration time.Duration) string {
	var sb strings.Builder

	sb.WriteString("command: ")
	sb.WriteString(command)
	if len(args) > 0 {
		sb.WriteString(" ")
		sb.WriteString(strings.Join(args, " "))
	}
	sb.WriteString("\n")

	sb.WriteString("exit_code: ")
	sb.WriteString(fmt.Sprintf("%d", exitCode))
	sb.WriteString("\n")

	sb.WriteString("duration: ")
	sb.WriteString(duration.String())
	sb.WriteString("\n")

	if stdout != "" {
		sb.WriteString("stdout:\n")
		sb.WriteString(stdout)
		sb.WriteString("\n")
	}

	if stderr != "" {
		sb.WriteString("stderr:\n")
		sb.WriteString(stderr)
		sb.WriteString("\n")
	}

	return sb.String()
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}
