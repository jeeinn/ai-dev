package store

import (
	"fmt"
	"time"
)

// Agent represents an AI agent registered in the system.
type Agent struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	GiteaUsername string    `json:"gitea_username"`
	GiteaToken    string    `json:"gitea_token"`
	AvatarURL     string    `json:"avatar_url"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	MaxTokens     int       `json:"max_tokens"`
	Temperature   float64   `json:"temperature"`
	SystemPrompt  string    `json:"system_prompt"`
	UserTemplate  string    `json:"user_template"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CreateAgent inserts a new agent into the database.
func (db *DB) CreateAgent(a *Agent) error {
	result, err := db.Exec(`INSERT INTO agents 
		(name, gitea_username, gitea_token, avatar_url, provider, model, max_tokens, temperature, system_prompt, user_template, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Name, a.GiteaUsername, a.GiteaToken, a.AvatarURL, a.Provider, a.Model, a.MaxTokens, a.Temperature, a.SystemPrompt, a.UserTemplate, a.Status)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	id, _ := result.LastInsertId()
	a.ID = id
	return nil
}

// GetAgent returns an agent by ID.
func (db *DB) GetAgent(id int64) (*Agent, error) {
	var a Agent
	err := db.QueryRow(`SELECT id, name, gitea_username, gitea_token, avatar_url, provider, model, max_tokens, temperature, system_prompt, user_template, status, created_at, updated_at
		FROM agents WHERE id = ?`, id).Scan(
		&a.ID, &a.Name, &a.GiteaUsername, &a.GiteaToken, &a.AvatarURL, &a.Provider, &a.Model, &a.MaxTokens, &a.Temperature, &a.SystemPrompt, &a.UserTemplate, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return &a, nil
}

// GetAgentByGiteaUsername returns an agent by their Gitea username.
func (db *DB) GetAgentByGiteaUsername(username string) (*Agent, error) {
	var a Agent
	err := db.QueryRow(`SELECT id, name, gitea_username, gitea_token, avatar_url, provider, model, max_tokens, temperature, system_prompt, user_template, status, created_at, updated_at
		FROM agents WHERE gitea_username = ?`, username).Scan(
		&a.ID, &a.Name, &a.GiteaUsername, &a.GiteaToken, &a.AvatarURL, &a.Provider, &a.Model, &a.MaxTokens, &a.Temperature, &a.SystemPrompt, &a.UserTemplate, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get agent by username: %w", err)
	}
	return &a, nil
}

// ListAgents returns all agents.
func (db *DB) ListAgents() ([]*Agent, error) {
	rows, err := db.Query(`SELECT id, name, gitea_username, gitea_token, avatar_url, provider, model, max_tokens, temperature, system_prompt, user_template, status, created_at, updated_at
		FROM agents ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.GiteaUsername, &a.GiteaToken, &a.AvatarURL, &a.Provider, &a.Model, &a.MaxTokens, &a.Temperature, &a.SystemPrompt, &a.UserTemplate, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, &a)
	}
	return agents, nil
}

// UpdateAgent updates an existing agent.
func (db *DB) UpdateAgent(a *Agent) error {
	_, err := db.Exec(`UPDATE agents SET name=?, provider=?, model=?, max_tokens=?, temperature=?, system_prompt=?, user_template=?, status=?, avatar_url=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
		a.Name, a.Provider, a.Model, a.MaxTokens, a.Temperature, a.SystemPrompt, a.UserTemplate, a.Status, a.AvatarURL, a.ID)
	if err != nil {
		return fmt.Errorf("update agent: %w", err)
	}
	return nil
}

// DeleteAgent deletes an agent by ID.
func (db *DB) DeleteAgent(id int64) error {
	_, err := db.Exec("DELETE FROM agents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
}
