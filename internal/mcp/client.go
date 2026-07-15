package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"gitea-agent-gateway/internal/config"
)

// Client is an MCP (Model Context Protocol) client that communicates
// with an MCP server over HTTP (Streamable HTTP / JSON-RPC 2.0).
type Client struct {
	name    string
	cfg     config.MCPServerConfig
	client  *http.Client
	baseURL string
}

// jsonRPCRequest is a JSON-RPC 2.0 request envelope.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response envelope.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// Tool represents a tool exposed by an MCP server.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolsListResult is the result of a tools/list call.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolResult is the result of a tools/call call.
type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem is a single content item in a tool call result.
type ContentItem struct {
	Type string `json:"type"` // "text", "image", etc.
	Text string `json:"text,omitempty"`
}

// NewClient creates a new MCP client for the given server config.
func NewClient(name string, cfg config.MCPServerConfig) *Client {
	timeout := 30 * time.Second
	if cfg.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil {
			timeout = d
		}
	}
	return &Client{
		name:    name,
		cfg:     cfg,
		client:  &http.Client{Timeout: timeout},
		baseURL: cfg.BaseURL,
	}
}

// ListTools calls tools/list on the MCP server and returns the available tools.
func (c *Client) ListTools(ctx context.Context) (*ToolsListResult, error) {
	var result ToolsListResult
	if err := c.call(ctx, "tools/list", nil, &result); err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	return &result, nil
}

// CallTool calls tools/call on the MCP server with the given tool name and arguments.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*CallToolResult, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": arguments,
	}
	var result CallToolResult
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return nil, fmt.Errorf("tools/call %s: %w", name, err)
	}
	return &result, nil
}

// call performs a JSON-RPC call to the MCP server.
func (c *Client) call(ctx context.Context, method string, params interface{}, result interface{}) error {
	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if rpcResp.Error != nil {
		return fmt.Errorf("mcp error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	if result != nil && rpcResp.Result != nil {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}

	return nil
}

// Name returns the server name.
func (c *Client) Name() string {
	return c.name
}
