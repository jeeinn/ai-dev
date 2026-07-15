package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/mcp"
)

// RegisterMCPTools discovers tools from the specified MCP servers and registers
// them into the ToolRegistry. Tools from multiple servers are merged; in case of
// name collisions, later servers in the list overwrite earlier ones.
//
// The tool function delegates to mcp.Registry.CallTool at invocation time.
func (r *ToolRegistry) RegisterMCPTools(ctx context.Context, mcpReg *mcp.Registry, serverNames []string) error {
	if mcpReg == nil || len(serverNames) == 0 {
		return nil
	}

	for _, serverName := range serverNames {
		tools, err := mcpReg.LoadTools(ctx, serverName)
		if err != nil {
			return fmt.Errorf("load tools from mcp server %q: %w", serverName, err)
		}

		for _, t := range tools {
			toolDef := mcpToolToToolDef(serverName, t, mcpReg)
			r.Register(toolDef)
		}
	}

	return nil
}

// mcpToolToToolDef converts an MCP Tool to a ToolDef that can be registered
// in the ToolRegistry. The tool name is prefixed with the server name if
// there is a collision risk — but for a clean UX we keep the original name
// and let the caller decide about prefixing.
func mcpToolToToolDef(serverName string, t mcp.Tool, mcpReg *mcp.Registry) *ToolDef {
	params := mcpSchemaToLLMParams(t.InputSchema)

	return &ToolDef{
		Name:        t.Name,
		Description: t.Description,
		Parameters:  params,
		Fn: func(params map[string]interface{}) (string, error) {
			ctx := context.Background()
			result, err := mcpReg.CallTool(ctx, serverName, t.Name, params)
			if err != nil {
				return "", err
			}
			if result.IsError {
				return "", fmt.Errorf("mcp tool %s returned error", t.Name)
			}
			return mcpContentToText(result.Content), nil
		},
	}
}

// mcpSchemaToLLMParams converts an MCP JSON Schema inputSchema to the
// llm.Parameters format used by ToolDef.
func mcpSchemaToLLMParams(schema map[string]interface{}) llm.Parameters {
	params := llm.Parameters{
		Type:       "object",
		Properties: make(map[string]llm.Property),
	}

	if schema == nil {
		return params
	}

	if t, ok := schema["type"].(string); ok && t != "" {
		params.Type = t
	}

	if props, ok := schema["properties"].(map[string]interface{}); ok {
		for name, prop := range props {
			if propMap, ok := prop.(map[string]interface{}); ok {
				p := llm.Property{}
				if pt, ok := propMap["type"].(string); ok {
					p.Type = pt
				}
				if desc, ok := propMap["description"].(string); ok {
					p.Description = desc
				}
				params.Properties[name] = p
			}
		}
	}

	if required, ok := schema["required"].([]interface{}); ok {
		reqStrs := make([]string, 0, len(required))
		for _, r := range required {
			if s, ok := r.(string); ok {
				reqStrs = append(reqStrs, s)
			}
		}
		params.Required = reqStrs
	}

	return params
}

// mcpContentToText flattens MCP content items into a single text string.
// Text items are joined with newlines; non-text items are JSON-serialized
// so the LLM still gets structured data.
func mcpContentToText(content []mcp.ContentItem) string {
	if len(content) == 0 {
		return ""
	}

	var parts []string
	for _, item := range content {
		switch item.Type {
		case "text":
			parts = append(parts, item.Text)
		default:
			data, _ := json.Marshal(item)
			parts = append(parts, string(data))
		}
	}
	return strings.Join(parts, "\n")
}
