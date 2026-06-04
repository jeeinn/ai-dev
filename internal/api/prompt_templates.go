package api

import (
	"encoding/json"
	"net/http"
)

const promptTemplatesKey = "prompt.templates"

// listPromptTemplates returns all prompt templates (built-in + custom from DB).
func (h *Handler) listPromptTemplates(w http.ResponseWriter, r *http.Request) {
	result := make(map[string]interface{})

	// 1. Built-in templates (base layer)
	if h.prompt != nil {
		for _, name := range h.prompt.GetAvailableTemplates() {
			sys, usr := h.prompt.GetBuiltinTemplate(name)
			result[name] = map[string]string{
				"name":          name,
				"system_prompt": sys,
				"user_template": usr,
				"source":        "builtin",
			}
		}
	}

	// 2. Config templates (override built-in)
	for name, tmpl := range h.cfg.Agents.Templates {
		result[name] = map[string]string{
			"name":          tmpl.Name,
			"system_prompt": tmpl.SystemPrompt,
			"user_template": tmpl.UserTemplate,
			"source":        "config",
		}
	}

	// 3. DB custom templates (highest priority)
	dbTemplates := h.loadDBTemplates()
	for name, tmpl := range dbTemplates {
		tmpl["source"] = "custom"
		result[name] = tmpl
	}

	writeJSON(w, 200, result)
}

// updatePromptTemplates saves custom templates to DB.
func (h *Handler) updatePromptTemplates(w http.ResponseWriter, r *http.Request) {
	var templates map[string]map[string]string
	if err := json.NewDecoder(r.Body).Decode(&templates); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}

	// Merge with existing DB templates
	existing := h.loadDBTemplates()
	for name, tmpl := range templates {
		existing[name] = tmpl
	}

	data, _ := json.Marshal(existing)
	if err := h.db.SetConfig(promptTemplatesKey, string(data)); err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, 200, existing)
}

// deletePromptTemplate deletes a custom template from DB.
func (h *Handler) deletePromptTemplate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, 400, "missing template name")
		return
	}

	existing := h.loadDBTemplates()
	if _, ok := existing[name]; !ok {
		writeError(w, 404, "template not found in custom templates")
		return
	}

	delete(existing, name)

	data, _ := json.Marshal(existing)
	if err := h.db.SetConfig(promptTemplatesKey, string(data)); err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "deleted", "name": name})
}

// loadDBTemplates loads custom templates from DB.
func (h *Handler) loadDBTemplates() map[string]map[string]string {
	raw, err := h.db.GetConfig(promptTemplatesKey)
	if err != nil || raw == "" {
		return make(map[string]map[string]string)
	}
	var templates map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &templates); err != nil {
		return make(map[string]map[string]string)
	}
	return templates
}
