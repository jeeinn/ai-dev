package store

import (
	"fmt"
	"time"
)

// Task represents an agent execution task.
type Task struct {
	ID         int64      `json:"id"`
	Event      string     `json:"event"`
	Repo       string     `json:"repo"`
	IssueID    int        `json:"issue_id"`
	AgentID    int64      `json:"agent_id"`
	TaskType   string     `json:"task_type"`
	Context    string     `json:"context"`
	Status     string     `json:"status"`
	Priority   int        `json:"priority"`
	DeliveryID string     `json:"delivery_id"`
	BaseBranch string     `json:"base_branch"` // PR head branch for solve_comment (empty = create new branch)
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Result     string     `json:"result"`
	Error      string     `json:"error"`
}

// CreateTask inserts a new task.
func (db *DB) CreateTask(t *Task) error {
	// Check for duplicate delivery_id to prevent duplicate tasks
	if t.DeliveryID != "" {
		exists, err := db.deliveryExists(t.DeliveryID)
		if err != nil {
			return fmt.Errorf("check delivery: %w", err)
		}
		if exists {
			return fmt.Errorf("task with delivery_id %s already exists", t.DeliveryID)
		}
	}

	result, err := db.Exec(`INSERT INTO tasks (event, repo, issue_id, agent_id, task_type, context, status, priority, delivery_id, base_branch)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Event, t.Repo, t.IssueID, t.AgentID, t.TaskType, t.Context, t.Status, t.Priority, t.DeliveryID, t.BaseBranch)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	id, _ := result.LastInsertId()
	t.ID = id
	return nil
}

// deliveryExists checks if a task with the given delivery_id already exists.
func (db *DB) deliveryExists(deliveryID string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE delivery_id = ?`, deliveryID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateTaskStatus updates a task's status, result, and error.
func (db *DB) UpdateTaskStatus(id int64, status, result, errMsg string) error {
	_, err := db.Exec(`UPDATE tasks SET status=?, result=?, error=?, 
		CASE WHEN status='running' AND started_at IS NULL THEN started_at=CURRENT_TIMESTAMP END,
		CASE WHEN status IN ('success','failed') THEN finished_at=CURRENT_TIMESTAMP END
		WHERE id=?`, status, result, errMsg, id)
	if err != nil {
		// Simplified update without CASE
		_, err2 := db.Exec(`UPDATE tasks SET status=?, result=?, error=? WHERE id=?`, status, result, errMsg, id)
		if err2 != nil {
			return fmt.Errorf("update task status: %w", err2)
		}
	}
	return nil
}

// ListPendingTasks returns all pending tasks ordered by priority and creation time.
func (db *DB) ListPendingTasks() ([]*Task, error) {
	rows, err := db.Query(`SELECT id, event, repo, issue_id, agent_id, task_type, context, status, priority, delivery_id, base_branch, created_at
		FROM tasks WHERE status='pending' ORDER BY priority DESC, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list pending tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Event, &t.Repo, &t.IssueID, &t.AgentID, &t.TaskType, &t.Context, &t.Status, &t.Priority, &t.DeliveryID, &t.BaseBranch, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, &t)
	}
	return tasks, nil
}

// GetTask returns a task by ID.
func (db *DB) GetTask(id int64) (*Task, error) {
	var t Task
	err := db.QueryRow(`SELECT id, event, repo, issue_id, agent_id, task_type, context, status, priority, delivery_id, base_branch, created_at, started_at, finished_at, result, error
		FROM tasks WHERE id=?`, id).Scan(
		&t.ID, &t.Event, &t.Repo, &t.IssueID, &t.AgentID, &t.TaskType, &t.Context, &t.Status, &t.Priority, &t.DeliveryID, &t.BaseBranch, &t.CreatedAt, &t.StartedAt, &t.FinishedAt, &t.Result, &t.Error)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	return &t, nil
}

// CountTasks returns the total number of tasks.
func (db *DB) CountTasks() (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count tasks: %w", err)
	}
	return count, nil
}

// ListTasksFiltered returns tasks with filtering and pagination, plus total count.
func (db *DB) ListTasksFiltered(limit, offset int, status, taskType string, agentID int64) ([]*Task, int, error) {
	// Build WHERE clause
	where := "1=1"
	args := []interface{}{}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if taskType != "" {
		where += " AND task_type = ?"
		args = append(args, taskType)
	}
	if agentID > 0 {
		where += " AND agent_id = ?"
		args = append(args, agentID)
	}

	// Count total
	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM tasks WHERE %s`, where), countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}

	// Fetch page
	args = append(args, limit, offset)
	rows, err := db.Query(fmt.Sprintf(`SELECT id, event, repo, issue_id, agent_id, task_type, context, status, priority, delivery_id, base_branch, created_at, started_at, finished_at, result, error
		FROM tasks WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`, where), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Event, &t.Repo, &t.IssueID, &t.AgentID, &t.TaskType, &t.Context, &t.Status, &t.Priority, &t.DeliveryID, &t.BaseBranch, &t.CreatedAt, &t.StartedAt, &t.FinishedAt, &t.Result, &t.Error); err != nil {
			return nil, 0, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, &t)
	}
	return tasks, total, nil
}

// ListTasks returns tasks with pagination.
func (db *DB) ListTasks(limit, offset int) ([]*Task, error) {
	rows, err := db.Query(`SELECT id, event, repo, issue_id, agent_id, task_type, context, status, priority, delivery_id, base_branch, created_at, started_at, finished_at, result, error
		FROM tasks ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Event, &t.Repo, &t.IssueID, &t.AgentID, &t.TaskType, &t.Context, &t.Status, &t.Priority, &t.DeliveryID, &t.BaseBranch, &t.CreatedAt, &t.StartedAt, &t.FinishedAt, &t.Result, &t.Error); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, &t)
	}
	return tasks, nil
}

// ListTasksByAgentID returns tasks for a specific agent.
func (db *DB) ListTasksByAgentID(agentID int64, limit int) ([]*Task, error) {
	rows, err := db.Query(`SELECT id, event, repo, issue_id, agent_id, task_type, context, status, priority, delivery_id, base_branch, created_at, started_at, finished_at, result, error
		FROM tasks WHERE agent_id = ? ORDER BY created_at DESC LIMIT ?`, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("list tasks by agent: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Event, &t.Repo, &t.IssueID, &t.AgentID, &t.TaskType, &t.Context, &t.Status, &t.Priority, &t.DeliveryID, &t.BaseBranch, &t.CreatedAt, &t.StartedAt, &t.FinishedAt, &t.Result, &t.Error); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, &t)
	}
	return tasks, nil
}

// ResetStaleRunningTasks resets tasks that have been in "running" state too long.
// Returns the number of tasks reset.
func (db *DB) ResetStaleRunningTasks(threshold time.Duration) (int, error) {
	cutoff := time.Now().Add(-threshold)
	result, err := db.Exec(`UPDATE tasks SET status='pending', started_at=NULL
		WHERE status='running' AND started_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("reset stale tasks: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return int(count), nil
}
