package store

import (
	"fmt"
	"time"
)

// PromptVersion represents a versioned prompt configuration.
type PromptVersion struct {
	ID           int64     `json:"id"`
	AgentID      int64     `json:"agent_id"`
	SystemPrompt string    `json:"system_prompt"`
	UserTemplate string    `json:"user_template"`
	Version      int       `json:"version"`
	IsActive     bool      `json:"is_active"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by"`
}

// CreatePromptVersion inserts a new prompt version.
func (db *DB) CreatePromptVersion(p *PromptVersion) error {
	// Get next version number for this agent
	version, err := db.getNextVersion(p.AgentID)
	if err != nil {
		return fmt.Errorf("get next version: %w", err)
	}
	p.Version = version

	// Deactivate previous active version
	if err := db.DeactivatePromptVersions(p.AgentID); err != nil {
		return fmt.Errorf("deactivate previous versions: %w", err)
	}

	result, err := db.Exec(`INSERT INTO prompt_history (agent_id, system_prompt, user_template, version, is_active, note, created_by)
		VALUES (?, ?, ?, ?, 1, ?, ?)`,
		p.AgentID, p.SystemPrompt, p.UserTemplate, p.Version, p.Note, p.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert prompt version: %w", err)
	}
	id, _ := result.LastInsertId()
	p.ID = id
	p.IsActive = true
	return nil
}

// GetPromptVersion returns a specific prompt version.
func (db *DB) GetPromptVersion(id int64) (*PromptVersion, error) {
	var p PromptVersion
	err := db.QueryRow(`SELECT id, agent_id, system_prompt, user_template, version, is_active, note, created_at, created_by
		FROM prompt_history WHERE id=?`, id).Scan(
		&p.ID, &p.AgentID, &p.SystemPrompt, &p.UserTemplate, &p.Version, &p.IsActive, &p.Note, &p.CreatedAt, &p.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("get prompt version: %w", err)
	}
	return &p, nil
}

// GetActivePrompt returns the active prompt for an agent.
func (db *DB) GetActivePrompt(agentID int64) (*PromptVersion, error) {
	var p PromptVersion
	err := db.QueryRow(`SELECT id, agent_id, system_prompt, user_template, version, is_active, note, created_at, created_by
		FROM prompt_history WHERE agent_id=? AND is_active=1 ORDER BY version DESC LIMIT 1`, agentID).Scan(
		&p.ID, &p.AgentID, &p.SystemPrompt, &p.UserTemplate, &p.Version, &p.IsActive, &p.Note, &p.CreatedAt, &p.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("get active prompt: %w", err)
	}
	return &p, nil
}

// ListPromptVersions returns all prompt versions for an agent.
func (db *DB) ListPromptVersions(agentID int64) ([]*PromptVersion, error) {
	rows, err := db.Query(`SELECT id, agent_id, system_prompt, user_template, version, is_active, note, created_at, created_by
		FROM prompt_history WHERE agent_id=? ORDER BY version DESC`, agentID)
	if err != nil {
		return nil, fmt.Errorf("list prompt versions: %w", err)
	}
	defer rows.Close()

	var versions []*PromptVersion
	for rows.Next() {
		var p PromptVersion
		if err := rows.Scan(&p.ID, &p.AgentID, &p.SystemPrompt, &p.UserTemplate, &p.Version, &p.IsActive, &p.Note, &p.CreatedAt, &p.CreatedBy); err != nil {
			return nil, fmt.Errorf("scan prompt version: %w", err)
		}
		versions = append(versions, &p)
	}
	return versions, nil
}

// DeactivatePromptVersions deactivates all prompt versions for an agent.
func (db *DB) DeactivatePromptVersions(agentID int64) error {
	_, err := db.Exec(`UPDATE prompt_history SET is_active=0 WHERE agent_id=? AND is_active=1`, agentID)
	if err != nil {
		return fmt.Errorf("deactivate prompt versions: %w", err)
	}
	return nil
}

// ActivatePromptVersion activates a specific prompt version.
func (db *DB) ActivatePromptVersion(id int64) error {
	// Get the prompt to find agent_id
	p, err := db.GetPromptVersion(id)
	if err != nil {
		return fmt.Errorf("get prompt version: %w", err)
	}

	// Deactivate all versions for this agent
	if err := db.DeactivatePromptVersions(p.AgentID); err != nil {
		return fmt.Errorf("deactivate versions: %w", err)
	}

	// Activate the specified version
	_, err = db.Exec(`UPDATE prompt_history SET is_active=1 WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("activate prompt version: %w", err)
	}
	return nil
}

// DeletePromptVersion deletes a prompt version.
func (db *DB) DeletePromptVersion(id int64) error {
	_, err := db.Exec(`DELETE FROM prompt_history WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete prompt version: %w", err)
	}
	return nil
}

// getNextVersion returns the next version number for an agent.
func (db *DB) getNextVersion(agentID int64) (int, error) {
	var maxVersion int
	err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM prompt_history WHERE agent_id=?`, agentID).Scan(&maxVersion)
	if err != nil {
		return 0, err
	}
	return maxVersion + 1, nil
}
