package mcp

import (
	"context"
	"fmt"
	"sync"

	"gitea-agent-gateway/internal/config"
)

// Registry manages MCP server clients and their tools.
// It provides tool discovery and invocation for multiple MCP servers.
type Registry struct {
	mu       sync.RWMutex
	clients  map[string]*Client
	toolList map[string][]Tool // server name -> tools (cached)
}

// NewRegistry creates a new MCP registry from the given configuration.
func NewRegistry(cfg config.MCPConfig) *Registry {
	r := &Registry{
		clients:  make(map[string]*Client),
		toolList: make(map[string][]Tool),
	}
	for name, serverCfg := range cfg.Servers {
		r.clients[name] = NewClient(name, serverCfg)
	}
	return r
}

// GetClient returns the MCP client for the given server name.
func (r *Registry) GetClient(name string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[name]
	return c, ok
}

// ServerNames returns all configured MCP server names.
func (r *Registry) ServerNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	return names
}

// LoadTools fetches the tool list from the specified MCP server.
// Results are cached; call RefreshTools to force a refresh.
func (r *Registry) LoadTools(ctx context.Context, serverName string) ([]Tool, error) {
	r.mu.RLock()
	if tools, ok := r.toolList[serverName]; ok {
		r.mu.RUnlock()
		return tools, nil
	}
	r.mu.RUnlock()

	client, ok := r.GetClient(serverName)
	if !ok {
		return nil, fmt.Errorf("unknown mcp server: %s", serverName)
	}

	result, err := client.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tools from %s: %w", serverName, err)
	}

	r.mu.Lock()
	r.toolList[serverName] = result.Tools
	r.mu.Unlock()

	return result.Tools, nil
}

// RefreshTools forces a refresh of the tool list for the given server.
func (r *Registry) RefreshTools(ctx context.Context, serverName string) ([]Tool, error) {
	r.mu.Lock()
	delete(r.toolList, serverName)
	r.mu.Unlock()
	return r.LoadTools(ctx, serverName)
}

// CallTool invokes a tool on the specified MCP server.
func (r *Registry) CallTool(ctx context.Context, serverName, toolName string, args map[string]interface{}) (*CallToolResult, error) {
	client, ok := r.GetClient(serverName)
	if !ok {
		return nil, fmt.Errorf("unknown mcp server: %s", serverName)
	}
	return client.CallTool(ctx, toolName, args)
}

// HasServer checks if a server with the given name is configured.
func (r *Registry) HasServer(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.clients[name]
	return ok
}
