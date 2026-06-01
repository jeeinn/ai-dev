package store

import "fmt"

// LogOperation records an operation in the audit log.
func (db *DB) LogOperation(agentID int64, taskID int64, action, detail string) {
	_, err := db.Exec(`INSERT INTO operation_logs (agent_id, task_id, action, detail) VALUES (?, ?, ?, ?)`,
		agentID, taskID, action, detail)
	if err != nil {
		// Log errors are non-fatal
		fmt.Printf("[WARN] Failed to log operation: %v\n", err)
	}
}

// OperationLog represents an audit log entry.
type OperationLog struct {
	ID        int64  `json:"id"`
	AgentID   int64  `json:"agent_id"`
	TaskID    int64  `json:"task_id"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

// ListOperationLogs returns operation logs with pagination.
func (db *DB) ListOperationLogs(limit, offset int) ([]*OperationLog, error) {
	rows, err := db.Query(`SELECT id, agent_id, task_id, action, detail, created_at
		FROM operation_logs ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list logs: %w", err)
	}
	defer rows.Close()

	var logs []*OperationLog
	for rows.Next() {
		var l OperationLog
		if err := rows.Scan(&l.ID, &l.AgentID, &l.TaskID, &l.Action, &l.Detail, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan log: %w", err)
		}
		logs = append(logs, &l)
	}
	return logs, nil
}
