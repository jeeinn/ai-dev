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
	db         *store.DB
	jwtManager *auth.JWTManager
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(db *store.DB, jwtManager *auth.JWTManager) *AuthHandler {
	return &AuthHandler{
		db:         db,
		jwtManager: jwtManager,
	}
}

// RegisterAuthRoutes registers authentication routes.
func (h *AuthHandler) RegisterAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("GET /api/auth/me", h.me)
	mux.HandleFunc("PUT /api/auth/password", h.changePassword)
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

	// Find user
	user, err := h.db.GetUserByUsername(req.Username)
	if err != nil {
		writeError(w, 401, "invalid credentials")
		return
	}

	// Check password
	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		writeError(w, 401, "invalid credentials")
		return
	}

	// Check if user is active
	if !user.IsActive {
		writeError(w, 403, "account disabled")
		return
	}

	// Generate JWT
	token, err := h.jwtManager.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		log.Printf("[ERROR] Failed to generate token: %v", err)
		writeError(w, 500, "failed to generate token")
		return
	}

	// Update last login
	h.db.UpdateLastLogin(user.ID)

	log.Printf("[INFO] User %s logged in", user.Username)

	writeJSON(w, 200, map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":           user.ID,
			"username":     user.Username,
			"role":         user.Role,
			"display_name": user.DisplayName,
			"email":        user.Email,
		},
	})
}

// logout handles user logout (client-side token removal).
func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	// JWT is stateless, so logout is handled client-side
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// me returns the current user info.
func (h *AuthHandler) me(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value("claims").(*auth.Claims)
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
		"id":           user.ID,
		"username":     user.Username,
		"role":         user.Role,
		"display_name": user.DisplayName,
		"email":        user.Email,
		"last_login":   user.LastLogin,
		"created_at":   user.CreatedAt,
	})
}

// changePassword changes the current user's password.
func (h *AuthHandler) changePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value("claims").(*auth.Claims)
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

	// Get user
	user, err := h.db.GetUser(claims.UserID)
	if err != nil {
		writeError(w, 404, "user not found")
		return
	}

	// Verify old password
	if !auth.CheckPassword(req.OldPassword, user.PasswordHash) {
		writeError(w, 400, "invalid old password")
		return
	}

	// Hash new password
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, 500, "failed to hash password")
		return
	}

	// Update password
	if err := h.db.UpdatePassword(claims.UserID, hash); err != nil {
		writeError(w, 500, "failed to update password")
		return
	}

	log.Printf("[INFO] User %s changed password", claims.Username)

	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// EnsureDefaultAdmin creates a default admin user if none exists.
func EnsureDefaultAdmin(db *store.DB, defaultPassword string) error {
	// Check if any admin exists
	users, err := db.ListUsers()
	if err != nil {
		return err
	}

	for _, u := range users {
		if u.Role == "admin" {
			return nil // Admin exists
		}
	}

	// Create default admin
	hash, err := auth.HashPassword(defaultPassword)
	if err != nil {
		return err
	}

	admin := &store.User{
		Username:     "admin",
		PasswordHash: hash,
		Role:         "admin",
		DisplayName:  "Administrator",
		IsActive:     true,
	}

	if err := db.CreateUser(admin); err != nil {
		return err
	}

	log.Printf("[INFO] Default admin user created (username: admin, password: %s)", defaultPassword)
	return nil
}

// JWTAuthMiddleware creates a middleware that validates JWT tokens.
func JWTAuthMiddleware(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get token from header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, 401, "missing authorization header")
				return
			}

			// Parse Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeError(w, 401, "invalid authorization format")
				return
			}

			// Validate token
			claims, err := jwtManager.ValidateToken(parts[1])
			if err != nil {
				writeError(w, 401, "invalid token")
				return
			}

			// Add claims to context
			ctx := context.WithValue(r.Context(), "claims", claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
