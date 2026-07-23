package api

import (
	"encoding/json"
	"net/http"
)

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
