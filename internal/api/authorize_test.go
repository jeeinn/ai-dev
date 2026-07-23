package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitea-agent-gateway/internal/auth"
)

func TestAuthorizeWrapAcceptsJWT(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	h := &Handler{
		auth:       NewAuthMiddleware("api-token"),
		jwtManager: jwtMgr,
	}

	token, err := jwtMgr.GenerateToken(1, "admin", "admin", false)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	called := false
	handler := h.authorizeWrap(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler(w, req)

	if !called {
		t.Error("handler should be called with valid JWT")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestAuthorizeWrapAcceptsAPIToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	h := &Handler{
		auth:       NewAuthMiddleware("api-token"),
		jwtManager: jwtMgr,
	}

	called := false
	handler := h.authorizeWrap(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/stats", nil)
	req.Header.Set("Authorization", "Bearer api-token")
	w := httptest.NewRecorder()

	handler(w, req)

	if !called {
		t.Error("handler should be called with valid API token")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestAuthorizeWrapRejectsInvalidToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	h := &Handler{
		auth:       NewAuthMiddleware("api-token"),
		jwtManager: jwtMgr,
	}

	called := false
	handler := h.authorizeWrap(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/api/stats", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()

	handler(w, req)

	if called {
		t.Error("handler should not be called with invalid token")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}
