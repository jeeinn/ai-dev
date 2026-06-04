package store

import (
	"database/sql"
	"fmt"
	"time"
)

// SystemConfig represents a system configuration entry.
type SystemConfig struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetConfig returns a config value by key.
func (db *DB) GetConfig(key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM system_config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get config: %w", err)
	}
	return value, nil
}

// SetConfig inserts or updates a config value.
func (db *DB) SetConfig(key, value string) error {
	_, err := db.Exec(`INSERT INTO system_config (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value)
	if err != nil {
		return fmt.Errorf("set config: %w", err)
	}
	return nil
}

// DeleteConfig deletes a config entry (revert to file default).
func (db *DB) DeleteConfig(key string) error {
	_, err := db.Exec(`DELETE FROM system_config WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("delete config: %w", err)
	}
	return nil
}

// ListConfigs returns all config entries as a map.
func (db *DB) ListConfigs() (map[string]string, error) {
	rows, err := db.Query(`SELECT key, value FROM system_config`)
	if err != nil {
		return nil, fmt.Errorf("list configs: %w", err)
	}
	defer rows.Close()

	configs := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan config: %w", err)
		}
		configs[key] = value
	}
	return configs, nil
}
