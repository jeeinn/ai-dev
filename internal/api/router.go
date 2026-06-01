package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/store"
)

// Handler serves the management API.
type Handler struct {
	db      *store.DB
	manager *agents.Manager
	auth    *AuthMiddleware
	cfg     *config.Config
}

// NewHandler creates a new API handler.
func NewHandler(db *store.DB, manager *agents.Manager, cfg *config.Config) *Handler {
	return &Handler{
		db:      db,
		manager: manager,
		auth:    NewAuthMiddleware(cfg.API.AuthToken),
		cfg:     cfg,
	}
}

// RegisterRoutes registers all API routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agents", h.auth.Wrap(h.listAgents))
	mux.HandleFunc("POST /api/agents", h.auth.Wrap(h.createAgent))
	mux.HandleFunc("GET /api/agents/{id}", h.auth.Wrap(h.getAgent))
	mux.HandleFunc("PUT /api/agents/{id}", h.auth.Wrap(h.updateAgent))
	mux.HandleFunc("DELETE /api/agents/{id}", h.auth.Wrap(h.deleteAgent))
	mux.HandleFunc("GET /api/tasks", h.auth.Wrap(h.listTasks))
	mux.HandleFunc("GET /api/tasks/{id}", h.auth.Wrap(h.getTask))
	mux.HandleFunc("GET /api/routes", h.auth.Wrap(h.listRoutes))
	mux.HandleFunc("POST /api/routes", h.auth.Wrap(h.createRoute))
	mux.HandleFunc("DELETE /api/routes/{id}", h.auth.Wrap(h.deleteRoute))
	mux.HandleFunc("GET /api/logs", h.auth.Wrap(h.listLogs))
	mux.HandleFunc("GET /api/stats", h.auth.Wrap(h.getStats))
	mux.HandleFunc("GET /api/templates", h.auth.Wrap(h.listTemplates))
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

// AgentDTO is the API response for agents, hiding sensitive fields.
type AgentDTO struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	GiteaUsername string  `json:"gitea_username"`
	AvatarURL    string  `json:"avatar_url"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	MaxTokens    int     `json:"max_tokens"`
	Temperature  float64 `json:"temperature"`
	SystemPrompt string  `json:"system_prompt"`
	UserTemplate string  `json:"user_template"`
	Status       string  `json:"status"`
}

func toAgentDTO(a *store.Agent) AgentDTO {
	return AgentDTO{
		ID:           a.ID,
		Name:         a.Name,
		GiteaUsername: a.GiteaUsername,
		AvatarURL:    a.AvatarURL,
		Provider:     a.Provider,
		Model:        a.Model,
		MaxTokens:    a.MaxTokens,
		Temperature:  a.Temperature,
		SystemPrompt: a.SystemPrompt,
		UserTemplate: a.UserTemplate,
		Status:       a.Status,
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
	writeJSON(w, 201, toAgentDTO(agent))
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
	if err := json.NewDecoder(r.Body).Decode(agent); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	agent.ID = id
	if err := h.db.UpdateAgent(agent); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, agent)
}

func (h *Handler) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	if err := h.db.DeleteAgent(id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// --- Task endpoints ---

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
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
	tasks, err := h.db.ListTasks(limit, offset)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, tasks)
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
	writeJSON(w, 200, task)
}

// --- Route endpoints ---

func (h *Handler) listRoutes(w http.ResponseWriter, r *http.Request) {
	routes, err := h.db.ListRoutes()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, routes)
}

func (h *Handler) createRoute(w http.ResponseWriter, r *http.Request) {
	var route store.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	if err := h.db.CreateRoute(&route); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, route)
}

func (h *Handler) deleteRoute(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	if err := h.db.DeleteRoute(id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
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
	writeJSON(w, 200, h.cfg.Agents.Templates)
}
