package api

import (
	"net/http"

	"gitea-agent-gateway/internal/config"
)

func (h *Handler) getSetupStatus(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg
	if h.cfgManager != nil {
		cfg = h.cfgManager.Get()
	}
	if cfg == nil {
		writeError(w, 500, "config not available")
		return
	}
	writeJSON(w, 200, config.CheckSetup(cfg))
}
