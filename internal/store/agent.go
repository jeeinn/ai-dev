package store

import (
	"encoding/json"
	"fmt"
	"time"
)

// AgentLoopConfig contains agent-specific loop configuration.
type AgentLoopConfig struct {
	MaxIterations int    `json:"max_iterations,omitempty"`
	MaxTokens     int    `json:"max_tokens,omitempty"`
	Timeout       string `json:"timeout,omitempty"`
	TotalTimeout  string `json:"total_timeout,omitempty"`
}

// Agent represents an AI agent registered in the system.
type Agent struct {
	ID            int64            `json:"id"`
	Name          string           `json:"name"`
	GiteaUsername string           `json:"gitea_username"`
	GiteaToken    string           `json:"gitea_token"`
	AvatarURL     string           `json:"avatar_url"`
	Provider      string           `json:"provider"`
	Model         string           `json:"model"`
	MaxTokens     int              `json:"max_tokens"`
	Temperature   float64          `json:"temperature"`
	SystemPrompt  string           `json:"system_prompt"`
	UserTemplate  string           `json:"user_template"`
	LoopConfig    *AgentLoopConfig `json:"loop_config,omitempty"`
	Repos         []string         `json:"repos,omitempty"`
	Role          string           `json:"role"` // analyze | coder | review
	Status        string           `json:"status"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// CreateAgent inserts a new agent into the database.
func (db *DB) CreateAgent(a *Agent) error {
	loopConfigJSON := "{}"
	if a.LoopConfig != nil {
		data, _ := json.Marshal(a.LoopConfig)
		loopConfigJSON = string(data)
	}
	reposJSON := "[]"
	if a.Repos != nil {
		data, _ := json.Marshal(a.Repos)
		reposJSON = string(data)
	}

	result, err := db.Exec(`INSERT INTO agents
		(name, gitea_username, gitea_token, avatar_url, provider, model, max_tokens, temperature, system_prompt, user_template, loop_config, repos, role, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Name, a.GiteaUsername, a.GiteaToken, a.AvatarURL, a.Provider, a.Model, a.MaxTokens, a.Temperature, a.SystemPrompt, a.UserTemplate, loopConfigJSON, reposJSON, a.Role, a.Status)
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
	var loopConfigJSON, reposJSON string
	err := db.QueryRow(`SELECT id, name, gitea_username, gitea_token, avatar_url, provider, model, max_tokens, temperature, system_prompt, user_template, loop_config, repos, role, status, created_at, updated_at
		FROM agents WHERE id = ?`, id).Scan(
		&a.ID, &a.Name, &a.GiteaUsername, &a.GiteaToken, &a.AvatarURL, &a.Provider, &a.Model, &a.MaxTokens, &a.Temperature, &a.SystemPrompt, &a.UserTemplate, &loopConfigJSON, &reposJSON, &a.Role, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if loopConfigJSON != "" {
		json.Unmarshal([]byte(loopConfigJSON), &a.LoopConfig)
	}
	if reposJSON != "" && reposJSON != "[]" {
		json.Unmarshal([]byte(reposJSON), &a.Repos)
	}
	return &a, nil
}

// GetAgentByGiteaUsername returns an agent by their Gitea username.
func (db *DB) GetAgentByGiteaUsername(username string) (*Agent, error) {
	var a Agent
	var loopConfigJSON, reposJSON string
	err := db.QueryRow(`SELECT id, name, gitea_username, gitea_token, avatar_url, provider, model, max_tokens, temperature, system_prompt, user_template, loop_config, repos, role, status, created_at, updated_at
		FROM agents WHERE gitea_username = ?`, username).Scan(
		&a.ID, &a.Name, &a.GiteaUsername, &a.GiteaToken, &a.AvatarURL, &a.Provider, &a.Model, &a.MaxTokens, &a.Temperature, &a.SystemPrompt, &a.UserTemplate, &loopConfigJSON, &reposJSON, &a.Role, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get agent by username: %w", err)
	}
	if loopConfigJSON != "" {
		json.Unmarshal([]byte(loopConfigJSON), &a.LoopConfig)
	}
	if reposJSON != "" && reposJSON != "[]" {
		json.Unmarshal([]byte(reposJSON), &a.Repos)
	}
	return &a, nil
}

// ListAgents returns all agents.
func (db *DB) ListAgents() ([]*Agent, error) {
	rows, err := db.Query(`SELECT id, name, gitea_username, gitea_token, avatar_url, provider, model, max_tokens, temperature, system_prompt, user_template, loop_config, repos, role, status, created_at, updated_at
		FROM agents ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		var a Agent
		var loopConfigJSON, reposJSON string
		if err := rows.Scan(&a.ID, &a.Name, &a.GiteaUsername, &a.GiteaToken, &a.AvatarURL, &a.Provider, &a.Model, &a.MaxTokens, &a.Temperature, &a.SystemPrompt, &a.UserTemplate, &loopConfigJSON, &reposJSON, &a.Role, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		if loopConfigJSON != "" {
			json.Unmarshal([]byte(loopConfigJSON), &a.LoopConfig)
		}
		if reposJSON != "" && reposJSON != "[]" {
			json.Unmarshal([]byte(reposJSON), &a.Repos)
		}
		agents = append(agents, &a)
	}
	return agents, nil
}

// UpdateAgent updates an existing agent.
func (db *DB) UpdateAgent(a *Agent) error {
	loopConfigJSON := "{}"
	if a.LoopConfig != nil {
		data, _ := json.Marshal(a.LoopConfig)
		loopConfigJSON = string(data)
	}
	reposJSON := "[]"
	if a.Repos != nil {
		data, _ := json.Marshal(a.Repos)
		reposJSON = string(data)
	}

	_, err := db.Exec(`UPDATE agents SET name=?, provider=?, model=?, max_tokens=?, temperature=?, system_prompt=?, user_template=?, loop_config=?, repos=?, role=?, status=?, avatar_url=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
		a.Name, a.Provider, a.Model, a.MaxTokens, a.Temperature, a.SystemPrompt, a.UserTemplate, loopConfigJSON, reposJSON, a.Role, a.Status, a.AvatarURL, a.ID)
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
