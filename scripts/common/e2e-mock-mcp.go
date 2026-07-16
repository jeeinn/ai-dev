//go:build ignore

// Minimal MCP JSON-RPC over HTTP mock for local E2E (tools/list + tools/call).
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcErr     `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	addr := "127.0.0.1:18080"
	if v := os.Getenv("E2E_MCP_ADDR"); v != "" {
		addr = v
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRPC)
	mux.HandleFunc("/mcp", handleRPC)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	log.Printf("e2e mock MCP listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req rpcReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 0, -32700, "parse error")
		return
	}

	switch req.Method {
	case "tools/list":
		writeOK(w, req.ID, map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "e2e_echo",
					"description": "E2E mock: echo a message back",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"message": map[string]interface{}{
								"type":        "string",
								"description": "Message to echo",
							},
						},
						"required": []string{"message"},
					},
				},
			},
		})
	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &params)
		msg := ""
		if params.Arguments != nil {
			if v, ok := params.Arguments["message"].(string); ok {
				msg = v
			}
		}
		writeOK(w, req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("e2e_echo:%s", msg)},
			},
		})
	case "initialize", "notifications/initialized", "ping":
		writeOK(w, req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"serverInfo":      map[string]string{"name": "e2e-mock", "version": "0.1.0"},
		})
	default:
		writeErr(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

func writeOK(w http.ResponseWriter, id int, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpcResp{JSONRPC: "2.0", ID: id, Result: result})
}

func writeErr(w http.ResponseWriter, id, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpcResp{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcErr{Code: code, Message: msg},
	})
}
