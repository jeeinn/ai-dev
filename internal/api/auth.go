package api

import (
	"net/http"
	"strings"
)

// AuthMiddleware provides Bearer token authentication for API endpoints.
type AuthMiddleware struct {
	token string
}

// NewAuthMiddleware creates a new auth middleware.
// If token is empty, authentication is disabled.
func NewAuthMiddleware(token string) *AuthMiddleware {
	return &AuthMiddleware{token: token}
}

// ValidAPIToken returns true when API token auth is disabled or the token matches.
func (a *AuthMiddleware) ValidAPIToken(token string) bool {
	if a.token == "" {
		return true
	}
	return token == a.token
}

// TokenConfigured returns true when a static API token is configured.
func (a *AuthMiddleware) TokenConfigured() bool {
	return a.token != ""
}

// Wrap wraps an http.HandlerFunc with authentication.
func (a *AuthMiddleware) Wrap(next http.HandlerFunc) http.HandlerFunc {
	// If no token configured, skip auth
	if a.token == "" {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Check Authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, 401, "missing authorization header")
			return
		}

		// Parse Bearer token
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(w, 401, "invalid authorization format, expected: Bearer <token>")
			return
		}

		// Validate token
		if parts[1] != a.token {
			writeError(w, 401, "invalid token")
			return
		}

		// Token valid, call next handler
		next(w, r)
	}
}
