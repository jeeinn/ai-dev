package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/auth"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/store"
)

// Handler serves the management API.
type Handler struct {
	db             *store.DB
	manager        *agents.Manager
	prompt         *agents.PromptManager
	auth           *AuthMiddleware
	jwtManager     *auth.JWTManager
	cfg            *config.Config
	cfgManager     *config.ConfigManager
	onConfigChange func(cfg *config.Config)
}

// NewHandler creates a new API handler.
func NewHandler(db *store.DB, manager *agents.Manager, cfg *config.Config, jwtManager *auth.JWTManager, cfgManager *config.ConfigManager, onConfigChange func(cfg *config.Config)) *Handler {
	return &Handler{
		db:             db,
		manager:        manager,
		prompt:         agents.NewPromptManager(db, &cfg.Agents),
		auth:           NewAuthMiddleware(cfg.API.AuthToken),
		jwtManager:     jwtManager,
		cfg:            cfg,
		cfgManager:     cfgManager,
		onConfigChange: onConfigChange,
	}
}

// RegisterRoutes registers all API routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// User management endpoints
	mux.HandleFunc("GET /api/users", h.jwtWrap(h.listUsers))
	mux.HandleFunc("POST /api/users", h.jwtWrap(h.createUser))
	mux.HandleFunc("PUT /api/users/{id}", h.jwtWrap(h.updateUser))
	mux.HandleFunc("DELETE /api/users/{id}", h.jwtWrap(h.deleteUser))

	// Repo endpoints
	mux.HandleFunc("GET /api/repos", h.jwtWrap(h.listRepos))

	// System config endpoints
	mux.HandleFunc("GET /api/config", h.jwtWrap(h.getConfig))
	mux.HandleFunc("PUT /api/config", h.jwtWrap(h.updateConfig))
	mux.HandleFunc("DELETE /api/config/{key}", h.jwtWrap(h.deleteConfigEntry))
	mux.HandleFunc("GET /api/config/providers/{name}/models", h.jwtWrap(h.getProviderModels))
	mux.HandleFunc("POST /api/config/test/gitea", h.jwtWrap(h.testGiteaConfig))
	mux.HandleFunc("POST /api/config/test/llm", h.jwtWrap(h.testLLMConfig))

	// Prompt template endpoints
	mux.HandleFunc("GET /api/prompt-templates", h.authorizeWrap(h.listPromptTemplates))
	mux.HandleFunc("PUT /api/prompt-templates", h.jwtWrap(h.updatePromptTemplates))
	mux.HandleFunc("DELETE /api/prompt-templates/{name}", h.jwtWrap(h.deletePromptTemplate))

	// Agent endpoints
	mux.HandleFunc("GET /api/agents", h.authorizeWrap(h.listAgents))
	mux.HandleFunc("POST /api/agents", h.authorizeWrap(h.createAgent))
	mux.HandleFunc("GET /api/agents/{id}", h.authorizeWrap(h.getAgent))
	mux.HandleFunc("PUT /api/agents/{id}", h.authorizeWrap(h.updateAgent))
	mux.HandleFunc("DELETE /api/agents/{id}", h.authorizeWrap(h.deleteAgent))
	mux.HandleFunc("GET /api/agents/{id}/tasks", h.authorizeWrap(h.listAgentTasks))
	mux.HandleFunc("GET /api/tasks", h.authorizeWrap(h.listTasks))
	mux.HandleFunc("GET /api/tasks/{id}", h.authorizeWrap(h.getTask))
	mux.HandleFunc("POST /api/tasks/{id}/reset", h.authorizeWrap(h.resetTask))
	mux.HandleFunc("GET /api/logs", h.authorizeWrap(h.listLogs))
	mux.HandleFunc("GET /api/stats", h.authorizeWrap(h.getStats))
	mux.HandleFunc("GET /api/templates", h.authorizeWrap(h.listTemplates))

	// Prompt management endpoints
	mux.HandleFunc("GET /api/agents/{id}/prompts", h.authorizeWrap(h.listPrompts))
	mux.HandleFunc("POST /api/agents/{id}/prompts", h.authorizeWrap(h.createPrompt))
	mux.HandleFunc("GET /api/agents/{id}/prompts/active", h.authorizeWrap(h.getActivePrompt))
	mux.HandleFunc("POST /api/prompts/{id}/activate", h.authorizeWrap(h.activatePrompt))
	mux.HandleFunc("DELETE /api/prompts/{id}", h.authorizeWrap(h.deletePrompt))

	// Session reset endpoint
	mux.HandleFunc("POST /api/sessions/reset", h.authorizeWrap(h.resetSession))

	// Workflow context endpoints
	mux.HandleFunc("GET /api/workflow-context", h.authorizeWrap(h.listWorkflowContexts))
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseID(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}

type contextKey string

const claimsKey contextKey = "claims"

// jwtWrap validates JWT token from Authorization header and adds claims to context.
func (h *Handler) jwtWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.jwtManager == nil {
			writeError(w, 500, "jwt not configured")
			return
		}

		token := extractBearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, 401, "missing or invalid authorization header")
			return
		}

		claims, err := h.jwtManager.ValidateToken(token)
		if err != nil {
			writeError(w, 401, "invalid token")
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

// authorizeWrap accepts JWT (web UI login) or the configured static API token.
func (h *Handler) authorizeWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r.Header.Get("Authorization"))

		if h.jwtManager != nil && token != "" {
			if claims, err := h.jwtManager.ValidateToken(token); err == nil {
				ctx := context.WithValue(r.Context(), claimsKey, claims)
				next(w, r.WithContext(ctx))
				return
			}
		}

		if !h.auth.TokenConfigured() {
			next(w, r)
			return
		}

		if token == "" {
			writeError(w, 401, "missing authorization header")
			return
		}

		if !h.auth.ValidAPIToken(token) {
			writeError(w, 401, "invalid token")
			return
		}

		next(w, r)
	}
}

func extractBearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return header[len(prefix):]
	}
	return ""
}

// --- User endpoints ---

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.db.ListUsers()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, users)
}

func (h *Handler) listRepos(w http.ResponseWriter, r *http.Request) {
	// Use admin client to list all repos
	repos, err := h.manager.ListRepos()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, repos)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, 400, "username and password are required")
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, 500, "failed to hash password")
		return
	}

	user := &store.User{
		Username:     req.Username,
		PasswordHash: hash,
		DisplayName:  req.DisplayName,
		Email:        req.Email,
		Role:         req.Role,
		IsActive:     true,
	}
	if err := h.db.CreateUser(user); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, user)
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	user, err := h.db.GetUser(id)
	if err != nil {
		writeError(w, 404, "user not found")
		return
	}

	var req struct {
		DisplayName *string `json:"display_name"`
		Email       *string `json:"email"`
		Role        *string `json:"role"`
		IsActive    *bool   `json:"is_active"`
		Password    *string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Role != nil {
		user.Role = *req.Role
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	if req.Password != nil && *req.Password != "" {
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			writeError(w, 500, "failed to hash password")
			return
		}
		if err := h.db.UpdatePassword(id, hash); err != nil {
			writeError(w, 500, err.Error())
			return
		}
	}

	if err := h.db.UpdateUser(user); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, user)
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	// Prevent deleting self
	if claims, ok := r.Context().Value(claimsKey).(*auth.Claims); ok && claims.UserID == id {
		writeError(w, 400, "cannot delete yourself")
		return
	}
	if err := h.db.DeleteUser(id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// AgentDTO is the API response for agents, hiding sensitive fields.
type AgentDTO struct {
	ID              int64                  `json:"id"`
	Name            string                 `json:"name"`
	GiteaUsername   string                 `json:"gitea_username"`
	AvatarURL       string                 `json:"avatar_url"`
	Provider        string                 `json:"provider"`
	Model           string                 `json:"model"`
	MaxOutputTokens int                    `json:"max_output_tokens"`
	MaxInputTokens  int                    `json:"max_input_tokens"`
	Temperature     float64                `json:"temperature"`
	Timeout         string                 `json:"timeout"`
	SystemPrompt    string                 `json:"system_prompt"`
	UserTemplate    string                 `json:"user_template"`
	LoopConfig      *store.AgentLoopConfig `json:"loop_config,omitempty"`
	Repos           []string               `json:"repos,omitempty"`
	Role            string                 `json:"role"`
	Status          string                 `json:"status"`
}

func toAgentDTO(a *store.Agent) AgentDTO {
	return AgentDTO{
		ID:              a.ID,
		Name:            a.Name,
		GiteaUsername:   a.GiteaUsername,
		AvatarURL:       a.AvatarURL,
		Repos:           a.Repos,
		Provider:        a.Provider,
		Model:           a.Model,
		MaxOutputTokens: a.MaxOutputTokens,
		MaxInputTokens:  a.MaxInputTokens,
		Temperature:     a.Temperature,
		Timeout:         a.Timeout,
		SystemPrompt:    a.SystemPrompt,
		UserTemplate:    a.UserTemplate,
		LoopConfig:      a.LoopConfig,
		Role:            a.Role,
		Status:          a.Status,
	}
}

// --- Agent endpoints ---

func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.db.ListAgents()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	dtos := make([]AgentDTO, len(agents))
	for i, a := range agents {
		dtos[i] = toAgentDTO(a)
	}
	writeJSON(w, 200, dtos)
}

func (h *Handler) createAgent(w http.ResponseWriter, r *http.Request) {
	var req agents.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	agent, err := h.manager.CreateAgent(req)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	// Add agent as collaborator to selected repos
	var repoWarnings []string
	if len(req.Repos) > 0 {
		repoWarnings = h.manager.AddCollaboratorToRepos(req.GiteaUsername, req.Repos)
	}

	resp := map[string]interface{}{
		"agent": toAgentDTO(agent),
	}
	if len(repoWarnings) > 0 {
		resp["repo_warnings"] = repoWarnings
	}
	writeJSON(w, 201, resp)
}

func (h *Handler) getAgent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	agent, err := h.db.GetAgent(id)
	if err != nil {
		writeError(w, 404, "agent not found")
		return
	}
	writeJSON(w, 200, toAgentDTO(agent))
}

func (h *Handler) updateAgent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	agent, err := h.db.GetAgent(id)
	if err != nil {
		writeError(w, 404, "agent not found")
		return
	}

	// Save old prompt for comparison
	oldSysPrompt := agent.SystemPrompt
	oldUsrTemplate := agent.UserTemplate

	// Decode into a temp struct to capture extra fields (repos)
	var req struct {
		store.Agent
		Repos []string `json:"repos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	// Copy decoded fields to agent
	agent.Name = req.Name
	agent.Provider = req.Provider
	agent.Model = req.Model
	agent.MaxOutputTokens = req.MaxOutputTokens
	agent.MaxInputTokens = req.MaxInputTokens
	agent.Temperature = req.Temperature
	agent.Timeout = req.Timeout
	agent.SystemPrompt = req.SystemPrompt
	agent.UserTemplate = req.UserTemplate
	agent.Status = req.Status
	agent.Repos = req.Repos
	if req.Role != "" {
		agent.Role = req.Role
	}
	if req.LoopConfig != nil {
		agent.LoopConfig = req.LoopConfig
	}
	agent.ID = id
	if err := h.manager.UpdateAgent(agent); err != nil {
		writeError(w, 500, err.Error())
		return
	}

	// Add agent as collaborator to newly selected repos
	var repoWarnings []string
	if len(req.Repos) > 0 {
		repoWarnings = h.manager.AddCollaboratorToRepos(agent.GiteaUsername, req.Repos)
	}

	// Auto-create prompt history if prompt changed
	if agent.SystemPrompt != oldSysPrompt || agent.UserTemplate != oldUsrTemplate {
		if h.prompt != nil && agent.SystemPrompt != "" {
			if _, err := h.prompt.SavePrompt(id, agent.SystemPrompt, agent.UserTemplate, "Agent 编辑更新", "ui"); err != nil {
				// Log but don't fail the request
				_ = err
			}
		}
	}

	resp := map[string]interface{}{
		"agent": agent,
	}
	if len(repoWarnings) > 0 {
		resp["repo_warnings"] = repoWarnings
	}
	writeJSON(w, 200, resp)
}

func (h *Handler) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	// Delete agent and Gitea user
	if err := h.manager.DeleteAgent(id, true); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// --- Task endpoints ---

func (h *Handler) listAgentTasks(w http.ResponseWriter, r *http.Request) {
	agentID, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid agent id")
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	tasks, err := h.db.ListTasksByAgentID(agentID, limit)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, tasks)
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	limit := 20
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	// Filter params
	status := r.URL.Query().Get("status")
	taskType := r.URL.Query().Get("type")
	var agentID int64
	if v := r.URL.Query().Get("agent_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			agentID = n
		}
	}

	tasks, total, err := h.db.ListTasksFiltered(limit, offset, status, taskType, agentID)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"data":  tasks,
		"total": total,
	})
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	task, err := h.db.GetTask(id)
	if err != nil {
		writeError(w, 404, "task not found")
		return
	}

	// Get usage summary
	usageSummary, _ := h.db.GetTaskUsageSummary(id)

	resp := map[string]interface{}{
		"task": task,
	}
	if usageSummary != nil {
		resp["usage"] = usageSummary
	}

	writeJSON(w, 200, resp)
}

func (h *Handler) resetTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	task, err := h.db.ResetTask(id, "manually reset from task list")
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "only pending/running") {
			writeError(w, 400, msg)
			return
		}
		if strings.Contains(msg, "get task") || strings.Contains(msg, "not found") {
			writeError(w, 404, "task not found")
			return
		}
		writeError(w, 500, msg)
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"ok":      true,
		"message": "任务已重置为失败，可重新触发",
		"task":    task,
	})
}

// --- Log endpoints ---

func (h *Handler) listLogs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	logs, err := h.db.ListOperationLogs(limit, offset)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, logs)
}

// --- Stats endpoint ---

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	agents, _ := h.db.ListAgents()
	tasks, _ := h.db.ListTasks(1000, 0)

	stats := map[string]interface{}{
		"total_agents": len(agents),
		"total_tasks":  len(tasks),
	}
	writeJSON(w, 200, stats)
}

// --- Templates endpoint ---

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	// Start with built-in templates from PromptManager
	result := make(map[string]interface{})
	if h.prompt != nil {
		for _, name := range h.prompt.GetAvailableTemplates() {
			sys, usr := h.prompt.GetBuiltinTemplate(name)
			result[name] = map[string]string{
				"name":          name,
				"system_prompt": sys,
				"user_template": usr,
			}
		}
	}
	// Overlay config templates
	for name, tmpl := range h.cfg.Agents.Templates {
		result[name] = map[string]string{
			"name":          tmpl.Name,
			"system_prompt": tmpl.SystemPrompt,
			"user_template": tmpl.UserTemplate,
		}
	}
	writeJSON(w, 200, result)
}

// --- Prompt endpoints ---

func (h *Handler) listPrompts(w http.ResponseWriter, r *http.Request) {
	agentID, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid agent id")
		return
	}

	versions, err := h.prompt.ListVersions(agentID)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, versions)
}

func (h *Handler) createPrompt(w http.ResponseWriter, r *http.Request) {
	agentID, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid agent id")
		return
	}

	var req struct {
		SystemPrompt string `json:"system_prompt"`
		UserTemplate string `json:"user_template"`
		Note         string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}

	if req.SystemPrompt == "" {
		writeError(w, 400, "system_prompt is required")
		return
	}

	pv, err := h.prompt.SavePrompt(agentID, req.SystemPrompt, req.UserTemplate, req.Note, "api")
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, pv)
}

func (h *Handler) getActivePrompt(w http.ResponseWriter, r *http.Request) {
	agentID, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid agent id")
		return
	}

	agent, err := h.db.GetAgent(agentID)
	if err != nil {
		writeError(w, 404, "agent not found")
		return
	}

	systemPrompt, userTemplate := h.prompt.GetPrompt(agent, "")
	writeJSON(w, 200, map[string]string{
		"system_prompt": systemPrompt,
		"user_template": userTemplate,
	})
}

func (h *Handler) activatePrompt(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid prompt id")
		return
	}

	if err := h.prompt.Rollback(id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "activated"})
}

func (h *Handler) deletePrompt(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid prompt id")
		return
	}

	if err := h.prompt.DeleteVersion(id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// --- Workflow context endpoint ---

func (h *Handler) listWorkflowContexts(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	if repo == "" {
		writeError(w, 400, "repo query parameter is required")
		return
	}

	issueStr := r.URL.Query().Get("issue")
	if issueStr != "" {
		// Single context lookup
		issueID, err := strconv.Atoi(issueStr)
		if err != nil {
			writeError(w, 400, "invalid issue number")
			return
		}
		ctx, err := h.db.GetWorkflowContext(repo, issueID)
		if err != nil {
			writeError(w, 404, "workflow context not found")
			return
		}
		writeJSON(w, 200, ctx)
		return
	}

	// List all contexts for the repo
	contexts, err := h.db.ListWorkflowContextsByRepo(repo)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, contexts)
}

// --- Session reset endpoint ---

func (h *Handler) resetSession(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	issueStr := r.URL.Query().Get("issue")

	if repo == "" || issueStr == "" {
		writeError(w, 400, "repo and issue query parameters are required")
		return
	}

	issueID, err := strconv.Atoi(issueStr)
	if err != nil {
		writeError(w, 400, "invalid issue number")
		return
	}

	// 1. Archive all sessions for this issue
	sessions, err := h.db.ListSessionsByIssue(repo, issueID)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	archived := 0
	for _, sess := range sessions {
		if err := h.db.ArchiveSession(sess.ID); err == nil {
			archived++
		}
	}

	// 2. Reset workflow context to idle
	ctx, err := h.db.GetWorkflowContext(repo, issueID)
	if err == nil {
		ctx.Stage = "idle"
		ctx.PreviousStage = ""
		ctx.ActiveAgentID = 0
		ctx.ActiveRole = ""
		ctx.SessionID = ""
		h.db.UpdateWorkflowContext(ctx)
	}

	writeJSON(w, 200, map[string]interface{}{
		"status":            "reset",
		"repo":              repo,
		"issue_id":          issueID,
		"sessions_archived": archived,
	})
}
