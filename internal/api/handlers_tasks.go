package api

import (
	"net/http"
	"strconv"
	"strings"
)

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
