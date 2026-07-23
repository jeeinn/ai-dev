package api

import (
	"net/http"
	"strconv"
)

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
