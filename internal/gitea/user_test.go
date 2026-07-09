package gitea

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetUserNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/users/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		t.Fatalf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()

	client := NewClient(server.URL, "admin")
	user, err := client.GetUser("missing")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if user != nil {
		t.Fatalf("expected nil user, got %+v", user)
	}
}

func TestGetUserFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/users/agent-bot" {
			json.NewEncoder(w).Encode(UserResponse{ID: 9, Login: "agent-bot", Email: "agent-bot@gateway.local"})
			return
		}
		t.Fatalf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()

	client := NewClient(server.URL, "admin")
	user, err := client.GetUser("agent-bot")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if user == nil || user.Login != "agent-bot" {
		t.Fatalf("unexpected user: %+v", user)
	}
}

func TestValidateUserToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" && r.Header.Get("Authorization") == "token good-token" {
			json.NewEncoder(w).Encode(CurrentUser{Login: "agent-bot"})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL, "admin")
	if !client.ValidateUserToken("agent-bot", "good-token") {
		t.Fatal("expected valid token")
	}
	if client.ValidateUserToken("agent-bot", "bad-token") {
		t.Fatal("expected invalid token")
	}
}
