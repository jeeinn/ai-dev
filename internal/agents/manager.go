package agents

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/store"
)

// Manager handles agent lifecycle (create, update, delete) and Gitea account registration.
type Manager struct {
	db     *store.DB
	gitea  *gitea.Client
	cfg    *config.GiteaConfig
}

// NewManager creates a new agent Manager.
func NewManager(db *store.DB, cfg *config.GiteaConfig) *Manager {
	return &Manager{
		db:    db,
		gitea: gitea.NewClient(cfg.URL, cfg.AdminToken),
		cfg:   cfg,
	}
}

// CreateAgentRequest is the payload for creating a new agent.
type CreateAgentRequest struct {
	Name          string  `json:"name"`
	GiteaUsername string  `json:"gitea_username"`
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	MaxTokens     int     `json:"max_tokens"`
	Temperature   float64 `json:"temperature"`
	SystemPrompt  string  `json:"system_prompt"`
	UserTemplate  string  `json:"user_template"`
}

// CreateAgent registers a new agent with Gitea account and stores it in DB.
func (m *Manager) CreateAgent(req CreateAgentRequest) (*store.Agent, error) {
	// 1. Create Gitea user
	password := generatePassword()
	_, err := m.gitea.AdminCreateUser(gitea.CreateUserRequest{
		LoginName:        req.GiteaUsername,
		Username:         req.GiteaUsername,
		Email:            req.GiteaUsername + "@gateway.local",
		Password:         password,
		SendNotify:       false,
		MustChangePassword: false,
	})
	if err != nil {
		return nil, fmt.Errorf("create gitea user: %w", err)
	}
	log.Printf("[INFO] Created Gitea user: %s", req.GiteaUsername)

	// 2. Generate API token using user's own credentials
	// Note: Gitea 1.26+ requires using user's own credentials to create tokens
	token, err := m.gitea.CreateTokenWithCredentials(req.GiteaUsername, password, "gateway-agent")
	if err != nil {
		return nil, fmt.Errorf("create gitea token: %w", err)
	}
	log.Printf("[INFO] Created Gitea token for: %s", req.GiteaUsername)

	// 3. Store in DB
	agent := &store.Agent{
		Name:          req.Name,
		GiteaUsername: req.GiteaUsername,
		GiteaToken:    token.SHA1,
		Provider:      req.Provider,
		Model:         req.Model,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		SystemPrompt:  req.SystemPrompt,
		UserTemplate:  req.UserTemplate,
		Status:        "active",
	}
	if err := m.db.CreateAgent(agent); err != nil {
		return nil, fmt.Errorf("store agent: %w", err)
	}

	log.Printf("[INFO] Agent created: id=%d name=%s gitea=%s", agent.ID, agent.Name, agent.GiteaUsername)
	return agent, nil
}

// UpdateAgent updates an agent's configuration (not Gitea account).
func (m *Manager) UpdateAgent(agent *store.Agent) error {
	return m.db.UpdateAgent(agent)
}

// DeleteAgent deletes an agent and optionally the Gitea user.
func (m *Manager) DeleteAgent(id int64, deleteGiteaUser bool) error {
	agent, err := m.db.GetAgent(id)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}

	// Delete Gitea user if requested
	if deleteGiteaUser && agent.GiteaUsername != "" {
		if err := m.gitea.AdminDeleteUser(agent.GiteaUsername); err != nil {
			log.Printf("[WARN] Failed to delete Gitea user %s: %v", agent.GiteaUsername, err)
			// Continue with agent deletion even if Gitea user deletion fails
		} else {
			log.Printf("[INFO] Deleted Gitea user: %s", agent.GiteaUsername)
		}
	}

	return m.db.DeleteAgent(id)
}

func generatePassword() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
