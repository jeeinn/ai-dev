package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareEmptyToken(t *testing.T) {
	// When token is empty, auth should be disabled
	auth := NewAuthMiddleware("")

	called := false
	handler := auth.Wrap(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if !called {
		t.Error("Handler should be called when token is empty")
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	auth := NewAuthMiddleware("test-secret-token")

	called := false
	handler := auth.Wrap(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()

	handler(w, req)

	if !called {
		t.Error("Handler should be called with valid token")
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddlewareMissingHeader(t *testing.T) {
	auth := NewAuthMiddleware("test-secret-token")

	called := false
	handler := auth.Wrap(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if called {
		t.Error("Handler should not be called without auth header")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	auth := NewAuthMiddleware("test-secret-token")

	called := false
	handler := auth.Wrap(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()

	handler(w, req)

	if called {
		t.Error("Handler should not be called with invalid token")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareInvalidFormat(t *testing.T) {
	auth := NewAuthMiddleware("test-secret-token")

	tests := []struct {
		name   string
		header string
	}{
		{"no space", "Bearertest"},
		{"wrong scheme", "Basic dXNlcjpwYXNz"},
		{"empty value", "Bearer "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			handler := auth.Wrap(func(w http.ResponseWriter, r *http.Request) {
				called = true
			})

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()

			handler(w, req)

			if called {
				t.Error("Handler should not be called with invalid format")
			}
			if w.Code != http.StatusUnauthorized {
				t.Errorf("Expected status 401, got %d", w.Code)
			}
		})
	}
}
