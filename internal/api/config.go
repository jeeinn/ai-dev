package api

import (
	"encoding/json"
	"net/http"
)

// --- System Config endpoints ---

func (h *Handler) getConfig(w http.ResponseWriter, r *http.Request) {
	if h.cfgManager == nil {
		writeError(w, 500, "config manager not initialized")
		return
	}
	writeJSON(w, 200, h.cfgManager.GetMap())
}

func (h *Handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	if h.cfgManager == nil {
		writeError(w, 500, "config manager not initialized")
		return
	}

	var entries map[string]string
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		writeError(w, 400, "invalid request body: expected {\"key\": \"value\", ...}")
		return
	}

	if len(entries) == 0 {
		writeError(w, 400, "no config entries provided")
		return
	}

	// Apply all entries
	for key, value := range entries {
		if err := h.cfgManager.Update(key, value); err != nil {
			writeError(w, 400, err.Error())
			return
		}
	}

	// Notify LLM registry to reload if LLM config changed
	if h.onConfigChange != nil {
		h.onConfigChange(h.cfgManager.Get())
	}

	writeJSON(w, 200, h.cfgManager.GetMap())
}

func (h *Handler) deleteConfigEntry(w http.ResponseWriter, r *http.Request) {
	if h.cfgManager == nil {
		writeError(w, 500, "config manager not initialized")
		return
	}

	key := r.PathValue("key")
	if key == "" {
		writeError(w, 400, "missing config key")
		return
	}

	if err := h.cfgManager.Delete(key); err != nil {
		writeError(w, 500, err.Error())
		return
	}

	// Notify on change
	if h.onConfigChange != nil {
		h.onConfigChange(h.cfgManager.Get())
	}

	writeJSON(w, 200, map[string]string{"status": "deleted", "key": key})
}
