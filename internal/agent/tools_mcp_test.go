package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/mcp"
)

func TestRegisterMCPTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)

		method := req["method"].(string)
		id := int(req["id"].(float64))

		if method == "tools/list" {
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"tools": []map[string]interface{}{
						{
							"name":        "my_mcp_tool",
							"description": "A test MCP tool",
							"inputSchema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"query": map[string]interface{}{
										"type":        "string",
										"description": "Search query",
									},
								},
								"required": []interface{}{"query"},
							},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if method == "tools/call" {
			params := req["params"].(map[string]interface{})
			args := params["arguments"].(map[string]interface{})
			query := args["query"].(string)

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"content": []map[string]interface{}{
						{"type": "text", "text": "Result for: " + query},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
	}))
	defer server.Close()

	mcpCfg := config.MCPConfig{
		Servers: map[string]config.MCPServerConfig{
			"test-server": {BaseURL: server.URL},
		},
	}
	mcpReg := mcp.NewRegistry(mcpCfg)

	registry := NewToolRegistry()

	err := registry.RegisterMCPTools(context.Background(), mcpReg, []string{"test-server"})
	if err != nil {
		t.Fatalf("RegisterMCPTools failed: %v", err)
	}

	tool, ok := registry.Get("my_mcp_tool")
	if !ok {
		t.Fatal("expected my_mcp_tool in registry")
	}
	if tool.Description != "A test MCP tool" {
		t.Errorf("unexpected description: %s", tool.Description)
	}
	if tool.Parameters.Type != "object" {
		t.Errorf("expected type object, got %s", tool.Parameters.Type)
	}
	if _, ok := tool.Parameters.Properties["query"]; !ok {
		t.Error("expected 'query' property")
	}
	if len(tool.Parameters.Required) != 1 || tool.Parameters.Required[0] != "query" {
		t.Errorf("expected required ['query'], got %v", tool.Parameters.Required)
	}

	result, err := registry.ExecuteTool(llm.ToolCall{
		Function: llm.FuncCall{
			Name:      "my_mcp_tool",
			Arguments: `{"query":"hello"}`,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}
	if result != "Result for: hello" {
		t.Errorf("expected 'Result for: hello', got '%s'", result)
	}
}

func TestRegisterMCPTools_NilRegistry(t *testing.T) {
	registry := NewToolRegistry()
	err := registry.RegisterMCPTools(context.Background(), nil, []string{"test"})
	if err != nil {
		t.Errorf("expected nil error for nil mcp registry, got %v", err)
	}
}

func TestRegisterMCPTools_EmptyServers(t *testing.T) {
	mcpCfg := config.MCPConfig{}
	mcpReg := mcp.NewRegistry(mcpCfg)

	registry := NewToolRegistry()
	err := registry.RegisterMCPTools(context.Background(), mcpReg, nil)
	if err != nil {
		t.Errorf("expected nil error for empty server list, got %v", err)
	}
}
