package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

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

// --- Workflow policy endpoints ---

func (h *Handler) listWorkflowPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := h.db.ListWorkflowPolicies()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, policies)
}

func (h *Handler) getWorkflowPolicy(w http.ResponseWriter, r *http.Request) {
	repo := strings.Trim(r.PathValue("repo"), "/")
	if repo == "" {
		writeError(w, 400, "repo is required")
		return
	}

	wp, err := h.db.GetWorkflowPolicy(repo)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if wp == nil {
		writeError(w, 404, "workflow policy not found")
		return
	}
	writeJSON(w, 200, wp)
}

func (h *Handler) upsertWorkflowPolicy(w http.ResponseWriter, r *http.Request) {
	repo := strings.Trim(r.PathValue("repo"), "/")
	if repo == "" {
		writeError(w, 400, "repo is required")
		return
	}

	var req struct {
		Preset string            `json:"preset"`
		Gates  map[string]string `json:"gates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}

	preset := req.Preset
	if preset == "" {
		preset = "standard"
	}

	gatesJSON := "{}"
	if len(req.Gates) > 0 {
		b, _ := json.Marshal(req.Gates)
		gatesJSON = string(b)
	}

	if err := h.db.UpsertWorkflowPolicy(repo, preset, gatesJSON); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"repo":   repo,
		"preset": preset,
		"gates":  req.Gates,
	})
}

func (h *Handler) deleteWorkflowPolicy(w http.ResponseWriter, r *http.Request) {
	repo := strings.Trim(r.PathValue("repo"), "/")
	if repo == "" {
		writeError(w, 400, "repo is required")
		return
	}

	if err := h.db.DeleteWorkflowPolicy(repo); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"status": "deleted", "repo": repo})
}
