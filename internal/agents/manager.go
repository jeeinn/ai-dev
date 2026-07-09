package agents

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/store"
)

const agentTokenName = "gateway-agent"

// Manager handles agent lifecycle (create, update, delete) and Gitea account registration.
type Manager struct {
	db    *store.DB
	gitea *gitea.Client
	cfg   *config.GiteaConfig
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
	Name          string                 `json:"name"`
	GiteaUsername string                 `json:"gitea_username"`
	Provider      string                 `json:"provider"`
	Model         string                 `json:"model"`
	MaxTokens     int                    `json:"max_tokens"`
	Temperature   float64                `json:"temperature"`
	SystemPrompt  string                 `json:"system_prompt"`
	UserTemplate  string                 `json:"user_template"`
	LoopConfig    *store.AgentLoopConfig `json:"loop_config,omitempty"`
	Repos         []string               `json:"repos,omitempty"` // Repos to add as collaborator (e.g. ["owner/repo"])
	Role          string                 `json:"role"`            // analyze | coder | review
}

// ReloadGitea updates the Gitea client after config changes.
func (m *Manager) ReloadGitea(cfg *config.GiteaConfig) {
	m.gitea = gitea.NewClient(cfg.URL, cfg.AdminToken)
	m.cfg = cfg
}

// ListRepos returns all repositories from Gitea.
func (m *Manager) ListRepos() ([]gitea.RepoItem, error) {
	return m.gitea.ListRepos()
}

// AddCollaboratorToRepos adds the agent user as a collaborator to the specified repos.
func (m *Manager) AddCollaboratorToRepos(username string, repos []string) []string {
	var errors []string
	for _, repo := range repos {
		parts := splitRepo(repo)
		if len(parts) != 2 {
			errors = append(errors, fmt.Sprintf("invalid repo format: %s", repo))
			continue
		}
		if err := m.gitea.AdminAddCollaborator(parts[0], parts[1], username); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", repo, err))
		} else {
			log.Printf("[INFO] Added %s as collaborator to %s", username, repo)
		}
	}
	return errors
}

func splitRepo(fullName string) []string {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	return parts
}

// EnsureGiteaAccount ensures the agent user exists on the current Gitea instance
// and returns a valid API token. It creates the user when missing, or refreshes
// the token when the stored token is invalid (e.g. after switching Gitea URL).
func (m *Manager) EnsureGiteaAccount(username, currentToken string) (token string, userCreated bool, err error) {
	if strings.TrimSpace(username) == "" {
		return "", false, fmt.Errorf("gitea username is empty")
	}

	user, err := m.gitea.GetUser(username)
	if err != nil {
		return "", false, fmt.Errorf("lookup gitea user: %w", err)
	}

	if user != nil && m.gitea.ValidateUserToken(username, currentToken) {
		return currentToken, false, nil
	}

	password := generatePassword()
	if user == nil {
		if _, err := m.gitea.AdminCreateUser(gitea.CreateUserRequest{
			LoginName:          username,
			Username:           username,
			Email:              username + "@gateway.local",
			Password:           password,
			SendNotify:         false,
			MustChangePassword: false,
		}); err != nil {
			return "", false, fmt.Errorf("create gitea user: %w", err)
		}
		userCreated = true
		log.Printf("[INFO] Created Gitea user: %s", username)
	} else {
		if err := m.gitea.AdminUpdateUserPassword(username, password); err != nil {
			return "", false, fmt.Errorf("reset gitea user password: %w", err)
		}
		log.Printf("[INFO] Refreshed Gitea credentials for existing user: %s", username)
	}

	tokenResp, err := m.gitea.CreateTokenWithCredentials(username, password, agentTokenName)
	if err != nil {
		return "", userCreated, fmt.Errorf("create gitea token: %w", err)
	}
	log.Printf("[INFO] Created Gitea token for: %s", username)
	return tokenResp.SHA1, userCreated, nil
}

// CreateAgent registers a new agent with Gitea account and stores it in DB.
func (m *Manager) CreateAgent(req CreateAgentRequest) (*store.Agent, error) {
	token, _, err := m.EnsureGiteaAccount(req.GiteaUsername, "")
	if err != nil {
		return nil, err
	}

	role := req.Role
	if role == "" {
		role = store.RoleAnalyze
	}
	agent := &store.Agent{
		Name:          req.Name,
		GiteaUsername: req.GiteaUsername,
		GiteaToken:    token,
		Provider:      req.Provider,
		Model:         req.Model,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		SystemPrompt:  req.SystemPrompt,
		UserTemplate:  req.UserTemplate,
		LoopConfig:    req.LoopConfig,
		Repos:         req.Repos,
		Role:          role,
		Status:        "active",
	}
	if err := m.db.CreateAgent(agent); err != nil {
		return nil, fmt.Errorf("store agent: %w", err)
	}

	log.Printf("[INFO] Agent created: id=%d name=%s gitea=%s", agent.ID, agent.Name, agent.GiteaUsername)
	return agent, nil
}

// UpdateAgent updates an agent's configuration and ensures its Gitea account exists.
func (m *Manager) UpdateAgent(agent *store.Agent) error {
	token, userCreated, err := m.EnsureGiteaAccount(agent.GiteaUsername, agent.GiteaToken)
	if err != nil {
		return err
	}
	if token != agent.GiteaToken {
		agent.GiteaToken = token
	}
	if userCreated {
		log.Printf("[INFO] Provisioned Gitea user for agent id=%d username=%s", agent.ID, agent.GiteaUsername)
	}
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
