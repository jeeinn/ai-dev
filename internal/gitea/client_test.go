package gitea

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:3000", "test-token")

	if client.BaseURL != "http://localhost:3000" {
		t.Errorf("Expected baseURL=http://localhost:3000, got %s", client.BaseURL)
	}
	if client.Token != "test-token" {
		t.Errorf("Expected token=test-token, got %s", client.Token)
	}
}

func TestIssueComment(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/repos/owner/repo/issues/1/comments" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		// Verify auth header
		auth := r.Header.Get("Authorization")
		if auth != "token test-token" {
			t.Errorf("Expected auth='token test-token', got %s", auth)
		}

		// Verify body
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["body"] != "Test comment" {
			t.Errorf("Expected body='Test comment', got %s", body["body"])
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "1"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.IssueComment("owner", "repo", 1, "Test comment")
	if err != nil {
		t.Fatalf("IssueComment failed: %v", err)
	}
}

func TestPRComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/repos/owner/repo/issues/10/comments" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "1"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.PRComment("owner", "repo", 10, "PR comment")
	if err != nil {
		t.Fatalf("PRComment failed: %v", err)
	}
}

func TestIssueGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     1,
			"number": 1,
			"title":  "Test Issue",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	issue, err := client.IssueGet("owner", "repo", 1)
	if err != nil {
		t.Fatalf("IssueGet failed: %v", err)
	}

	if issue["number"].(float64) != 1 {
		t.Errorf("Expected issue number=1, got %v", issue["number"])
	}
}

func TestCreatePR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/repos/owner/repo/pulls" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(PRResponse{
			ID:      1,
			Number:  10,
			Title:   "New PR",
			HTMLURL: "http://localhost/owner/repo/pulls/10",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	pr, err := client.CreatePR("owner", "repo", CreatePRRequest{
		Title: "New PR",
		Body:  "PR description",
		Head:  "feature",
		Base:  "main",
	})
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if pr.Number != 10 {
		t.Errorf("Expected PR number=10, got %d", pr.Number)
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.IssueComment("owner", "repo", 999, "Test")
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}
