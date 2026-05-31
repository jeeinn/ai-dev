package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
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

	sqlDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

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
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_agent_id ON tasks(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_delivery_id ON tasks(delivery_id)`,
		`CREATE INDEX IF NOT EXISTS idx_operation_logs_agent_id ON operation_logs(agent_id)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("execute migration: %w\nSQL: %s", err, m)
		}
	}

	return nil
}
