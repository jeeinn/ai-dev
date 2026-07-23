package api

import (
	"encoding/json"
	"net/http"

	"github.com/jeeinn/matea/internal/auth"
	"github.com/jeeinn/matea/internal/store"
)

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
