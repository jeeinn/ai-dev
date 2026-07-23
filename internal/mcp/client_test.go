package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jeeinn/matea/internal/config"
)

func newTestMCPServer(handler http.Handler) *httptest.Server {
	return httptest.NewServer(handler)
}

func TestClient_ListTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Method != "tools/list" {
			t.Errorf("expected tools/list, got %s", req.Method)
		}

		result := ToolsListResult{
			Tools: []Tool{
				{
					Name:        "echo",
					Description: "Echo back the message",
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"message": map[string]interface{}{
								"type":        "string",
								"description": "The message to echo",
							},
						},
						"required": []interface{}{"message"},
					},
				},
			},
		}
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}
		resultJSON, _ := json.Marshal(result)
		resp.Result = resultJSON
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.MCPServerConfig{BaseURL: server.URL}
	client := NewClient("test", cfg)

	result, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "echo" {
		t.Errorf("expected tool name 'echo', got '%s'", result.Tools[0].Name)
	}
}

func TestClient_CallTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Method != "tools/call" {
			t.Errorf("expected tools/call, got %s", req.Method)
		}

		result := CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: "hello world"},
			},
		}
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}
		resultJSON, _ := json.Marshal(result)
		resp.Result = resultJSON
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.MCPServerConfig{BaseURL: server.URL}
	client := NewClient("test", cfg)

	result, err := client.CallTool(context.Background(), "echo", map[string]interface{}{
		"message": "hello",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Text != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result.Content[0].Text)
	}
}

func TestClient_APIKeyAuth(t *testing.T) {
	gotAPIKey := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("Authorization")
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: 1, Result: json.RawMessage(`{"tools":[]}`)}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.MCPServerConfig{BaseURL: server.URL, APIKey: "test-key-123"}
	client := NewClient("test", cfg)

	client.ListTools(context.Background())

	if gotAPIKey != "Bearer test-key-123" {
		t.Errorf("expected 'Bearer test-key-123', got '%s'", gotAPIKey)
	}
}

func TestRegistry_LoadTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := ToolsListResult{
			Tools: []Tool{
				{Name: "tool1", Description: "First tool"},
				{Name: "tool2", Description: "Second tool"},
			},
		}
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: 1}
		resultJSON, _ := json.Marshal(result)
		resp.Result = resultJSON
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServerConfig{
			"test-server": {BaseURL: server.URL},
		},
	}
	reg := NewRegistry(cfg)

	if !reg.HasServer("test-server") {
		t.Error("expected test-server not found")
	}
	if reg.HasServer("nonexistent") {
		t.Error("unexpected server 'nonexistent'")
	}

	tools, err := reg.LoadTools(context.Background(), "test-server")
	if err != nil {
		t.Fatalf("LoadTools failed: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	tools2, err := reg.LoadTools(context.Background(), "test-server")
	if err != nil {
		t.Fatalf("LoadTools (cached) failed: %v", err)
	}
	if len(tools2) != 2 {
		t.Fatalf("expected 2 cached tools, got %d", len(tools2))
	}
}

func TestRegistry_CallTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Method == "tools/list" {
			result := ToolsListResult{
				Tools: []Tool{
					{Name: "greet", Description: "Greet someone"},
				},
			}
			resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}
			resultJSON, _ := json.Marshal(result)
			resp.Result = resultJSON
			json.NewEncoder(w).Encode(resp)
			return
		}

		result := CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: "Hello, Alice!"},
			},
		}
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}
		resultJSON, _ := json.Marshal(result)
		resp.Result = resultJSON
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServerConfig{
			"my-server": {BaseURL: server.URL},
		},
	}
	reg := NewRegistry(cfg)

	result, err := reg.CallTool(context.Background(), "my-server", "greet", map[string]interface{}{
		"name": "Alice",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.Content[0].Text != "Hello, Alice!" {
		t.Errorf("expected 'Hello, Alice!', got '%s'", result.Content[0].Text)
	}
}
