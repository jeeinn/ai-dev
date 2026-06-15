package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection.
type DB struct {
	*sql.DB
}

// Open creates or opens the SQLite database at the given path.
// It also creates all required tables if they don't exist.
func Open(dbPath string) (*DB, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=10000&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Set connection pool settings for SQLite
	sqlDB.SetMaxOpenConns(1) // SQLite only supports one writer at a time
	sqlDB.SetMaxIdleConns(1)

	// Enable foreign keys
	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	db := &DB{sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// migrate creates all required tables.
func (db *DB) migrate() error {
	// Step 1: Create tables (IF NOT EXISTS is safe to repeat)
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			name            TEXT NOT NULL,
			gitea_username  TEXT NOT NULL UNIQUE,
			gitea_token     TEXT NOT NULL,
			avatar_url      TEXT DEFAULT '',
			provider        TEXT NOT NULL DEFAULT 'deepseek',
			model           TEXT NOT NULL DEFAULT 'deepseek-chat',
			max_tokens      INTEGER DEFAULT 4096,
			temperature     REAL DEFAULT 0.3,
			system_prompt   TEXT NOT NULL DEFAULT '',
			user_template   TEXT NOT NULL DEFAULT '',
			status          TEXT DEFAULT 'active',
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS routes (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			event       TEXT NOT NULL,
			action      TEXT DEFAULT '',
			label       TEXT DEFAULT '',
			assignee    TEXT DEFAULT '',
			mention     TEXT DEFAULT '',
			agent_id    INTEGER NOT NULL,
			priority    INTEGER DEFAULT 0,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			event       TEXT NOT NULL,
			repo        TEXT NOT NULL,
			issue_id    INTEGER NOT NULL,
			agent_id    INTEGER NOT NULL,
			task_type   TEXT NOT NULL DEFAULT 'trigger',
			context     TEXT DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'pending',
			priority    INTEGER DEFAULT 0,
			delivery_id TEXT DEFAULT '',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			started_at  DATETIME,
			finished_at DATETIME,
			result      TEXT DEFAULT '',
			error       TEXT DEFAULT '',
			FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS prompt_history (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id      INTEGER NOT NULL,
			system_prompt TEXT NOT NULL,
			user_template TEXT NOT NULL,
			version       INTEGER NOT NULL,
			is_active     INTEGER DEFAULT 1,
			note          TEXT DEFAULT '',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_by    TEXT DEFAULT 'admin',
			FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS processed_deliveries (
			delivery_id TEXT PRIMARY KEY,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS operation_logs (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id    INTEGER,
			task_id     INTEGER,
			action      TEXT NOT NULL,
			detail      TEXT DEFAULT '',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			username      TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL DEFAULT 'user',
			display_name  TEXT DEFAULT '',
			email         TEXT DEFAULT '',
			is_active     INTEGER DEFAULT 1,
			last_login    DATETIME,
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_agent_id ON tasks(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_delivery_id ON tasks(delivery_id)`,
		`CREATE INDEX IF NOT EXISTS idx_operation_logs_agent_id ON operation_logs(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)`,
		`CREATE TABLE IF NOT EXISTS system_config (
			key         TEXT PRIMARY KEY,
			value       TEXT NOT NULL,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_contexts (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			repo            TEXT NOT NULL,
			issue_id        INTEGER NOT NULL DEFAULT 0,
			pr_id           INTEGER NOT NULL DEFAULT 0,
			stage           TEXT NOT NULL DEFAULT 'idle',
			active_agent_id INTEGER NOT NULL DEFAULT 0,
			active_role     TEXT NOT NULL DEFAULT '',
			session_id      TEXT NOT NULL DEFAULT '',
			updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(repo, issue_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_repo_issue ON workflow_contexts(repo, issue_id)`,
		`CREATE TABLE IF NOT EXISTS agent_sessions (
			id              TEXT PRIMARY KEY,
			repo            TEXT NOT NULL,
			issue_id        INTEGER NOT NULL DEFAULT 0,
			pr_id           INTEGER NOT NULL DEFAULT 0,
			agent_id        INTEGER NOT NULL,
			role            TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT 'active',
			branch          TEXT NOT NULL DEFAULT '',
			workspace_path  TEXT NOT NULL DEFAULT '',
			last_task_id    INTEGER NOT NULL DEFAULT 0,
			message_count   INTEGER NOT NULL DEFAULT 0,
			last_active_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_repo_issue ON agent_sessions(repo, issue_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_status ON agent_sessions(status)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("execute migration: %w\nSQL: %s", err, m)
		}
	}

	// Step 2: Schema evolution (ALTER TABLE) — runs after tables exist
	additionalMigrations := []string{
		`ALTER TABLE prompt_history ADD COLUMN is_active INTEGER DEFAULT 1`,
		`ALTER TABLE prompt_history ADD COLUMN note TEXT DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_prompt_history_agent_id ON prompt_history(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_prompt_history_is_active ON prompt_history(is_active)`,
		`ALTER TABLE agents ADD COLUMN loop_config TEXT DEFAULT '{}'`,
		`ALTER TABLE agents ADD COLUMN repos TEXT DEFAULT '[]'`,
		`ALTER TABLE tasks ADD COLUMN base_branch TEXT DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN role TEXT NOT NULL DEFAULT 'analyze'`,
		`ALTER TABLE tasks ADD COLUMN session_id TEXT DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN role TEXT DEFAULT ''`,
	}

	for _, m := range additionalMigrations {
		if _, err := db.Exec(m); err != nil {
			// Ignore "duplicate column" and "no such table" errors
			if !isDuplicateColumnError(err) && !isNoSuchTableError(err) {
				return fmt.Errorf("execute additional migration: %w\nSQL: %s", err, m)
			}
		}
	}

	return nil
}

// isDuplicateColumnError checks if the error is a "duplicate column" error.
func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "duplicate column") || contains(err.Error(), "already exists")
}

// isNoSuchTableError checks if the error is a "no such table" error.
func isNoSuchTableError(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "no such table")
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
