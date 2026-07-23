package api

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const claimsKey contextKey = "claims"

// jwtWrap validates JWT token from Authorization header and adds claims to context.
func (h *Handler) jwtWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.jwtManager == nil {
			writeError(w, 500, "jwt not configured")
			return
		}

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

// authorizeWrap accepts JWT (web UI login) or the configured static API token.
func (h *Handler) authorizeWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r.Header.Get("Authorization"))

		if h.jwtManager != nil && token != "" {
			if claims, err := h.jwtManager.ValidateToken(token); err == nil {
				ctx := context.WithValue(r.Context(), claimsKey, claims)
				next(w, r.WithContext(ctx))
				return
			}
		}

		if !h.auth.TokenConfigured() {
			next(w, r)
			return
		}

		if token == "" {
			writeError(w, 401, "missing authorization header")
			return
		}

		if !h.auth.ValidAPIToken(token) {
			writeError(w, 401, "invalid token")
			return
		}

		next(w, r)
	}
}

func extractBearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return header[len(prefix):]
	}
	return ""
}
