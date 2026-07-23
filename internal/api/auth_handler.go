package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"gitea-agent-gateway/internal/auth"
	"gitea-agent-gateway/internal/store"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	db                   *store.DB
	jwtManager           *auth.JWTManager
	defaultAdminPassword string
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(db *store.DB, jwtManager *auth.JWTManager, defaultAdminPassword string) *AuthHandler {
	return &AuthHandler{
		db:                   db,
		jwtManager:           jwtManager,
		defaultAdminPassword: defaultAdminPassword,
	}
}

// RegisterAuthRoutes registers authentication routes.
func (h *AuthHandler) RegisterAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("GET /api/auth/me", h.jwtAuth(h.me))
	mux.HandleFunc("PUT /api/auth/password", h.jwtAuth(h.changePassword))
}

func (h *AuthHandler) jwtAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

// login handles user login.
func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}

	user, err := h.db.GetUserByUsername(req.Username)
	if err != nil {
		writeError(w, 401, "invalid credentials")
		return
	}

	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		writeError(w, 401, "invalid credentials")
		return
	}

	if !user.IsActive {
		writeError(w, 403, "account disabled")
		return
	}

	mustChange := user.MustChangePassword
	// Seed admin still using the configured default password → force change.
	// Only apply to the bootstrap username "admin", not arbitrary users who
	// happen to choose the same password string.
	if !mustChange && user.Username == "admin" && h.defaultAdminPassword != "" && req.Password == h.defaultAdminPassword {
		mustChange = true
		if err := h.db.SetMustChangePassword(user.ID, true); err != nil {
			log.Printf("[WARN] Failed to set must_change_password for user %s: %v", user.Username, err)
		}
	}

	token, err := h.jwtManager.GenerateToken(user.ID, user.Username, user.Role, mustChange)
	if err != nil {
		log.Printf("[ERROR] Failed to generate token: %v", err)
		writeError(w, 500, "failed to generate token")
		return
	}

	h.db.UpdateLastLogin(user.ID)

	log.Printf("[INFO] User %s logged in", user.Username)

	writeJSON(w, 200, map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":                   user.ID,
			"username":             user.Username,
			"role":                 user.Role,
			"display_name":         user.DisplayName,
			"email":                user.Email,
			"must_change_password": mustChange,
		},
		"must_change_password": mustChange,
	})
}

// logout handles user logout (client-side token removal).
func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// me returns the current user info.
func (h *AuthHandler) me(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}

	user, err := h.db.GetUser(claims.UserID)
	if err != nil {
		writeError(w, 404, "user not found")
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"id":                   user.ID,
		"username":             user.Username,
		"role":                 user.Role,
		"display_name":         user.DisplayName,
		"email":                user.Email,
		"must_change_password": user.MustChangePassword,
		"last_login":           user.LastLogin,
		"created_at":           user.CreatedAt,
	})
}

// changePassword changes the current user's password.
func (h *AuthHandler) changePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}

	if strings.TrimSpace(req.NewPassword) == "" || len(req.NewPassword) < 8 {
		writeError(w, 400, "new password must be at least 8 characters")
		return
	}
	if req.NewPassword == req.OldPassword {
		writeError(w, 400, "new password must differ from old password")
		return
	}
	if h.defaultAdminPassword != "" && req.NewPassword == h.defaultAdminPassword {
		writeError(w, 400, "new password must not be the default password")
		return
	}

	user, err := h.db.GetUser(claims.UserID)
	if err != nil {
		writeError(w, 404, "user not found")
		return
	}

	if !auth.CheckPassword(req.OldPassword, user.PasswordHash) {
		writeError(w, 400, "invalid old password")
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, 500, "failed to hash password")
		return
	}

	if err := h.db.UpdatePassword(claims.UserID, hash); err != nil {
		writeError(w, 500, "failed to update password")
		return
	}

	// Issue a fresh token without must_change_password so subsequent API calls work.
	newToken, err := h.jwtManager.GenerateToken(user.ID, user.Username, user.Role, false)
	if err != nil {
		log.Printf("[ERROR] Failed to generate token after password change: %v", err)
		writeError(w, 500, "password updated but failed to issue new token")
		return
	}

	log.Printf("[INFO] User %s changed password", claims.Username)

	writeJSON(w, 200, map[string]interface{}{
		"status": "ok",
		"token":  newToken,
		"user": map[string]interface{}{
			"id":                   user.ID,
			"username":             user.Username,
			"role":                 user.Role,
			"display_name":         user.DisplayName,
			"email":                user.Email,
			"must_change_password": false,
		},
		"must_change_password": false,
	})
}

// EnsureDefaultAdmin creates a default admin user if none exists.
func EnsureDefaultAdmin(db *store.DB, defaultPassword string) error {
	users, err := db.ListUsers()
	if err != nil {
		return err
	}

	for _, u := range users {
		if u.Role == "admin" {
			return nil
		}
	}

	hash, err := auth.HashPassword(defaultPassword)
	if err != nil {
		return err
	}

	admin := &store.User{
		Username:           "admin",
		PasswordHash:       hash,
		Role:               "admin",
		DisplayName:        "Administrator",
		IsActive:           true,
		MustChangePassword: true,
	}

	if err := db.CreateUser(admin); err != nil {
		return err
	}

	log.Printf("[INFO] Default admin user created (username: admin, password: %s) — must change on first login", defaultPassword)
	return nil
}

// JWTAuthMiddleware creates a middleware that validates JWT tokens.
func JWTAuthMiddleware(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, 401, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeError(w, 401, "invalid authorization format")
				return
			}

			claims, err := jwtManager.ValidateToken(parts[1])
			if err != nil {
				writeError(w, 401, "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
