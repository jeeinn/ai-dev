package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"gitea-agent-gateway/internal/config"
)

const (
	opencodeWorkspaceModeGatewayPath = "gateway_path"
	opencodeDefaultTimeout          = 45 * time.Minute
)

// OpenCodeHTTPBackend implements CodingBackend by calling a local
// `opencode serve` sidecar over HTTP (Path A).
//
// API reference (from opencode-sdk-js):
//
//	POST /session                           → create session, returns {id,...}
//	POST /session/{id}/message              → send a message (modelID, providerID, parts, system, tools)
//	POST /session/{id}/abort                → abort an in-progress message
//	GET  /session/{id}/message              → list messages (info + parts[])
//
// The workspace directory is the same one prepared by prepareWriteWorkspace;
// the OpenCode sidecar must have access to the same filesystem (same machine).
// This matches the first-release constraint: only local sidecar, no remote.
type OpenCodeHTTPBackend struct {
	cfg    config.BackendConfig
	client *http.Client
	name   string // backend name from config, e.g. "opencode-local"
}

// NewOpenCodeHTTPBackend builds an OpenCode HTTP backend from a named config entry.
// It validates required fields and sets a default timeout.
func NewOpenCodeHTTPBackend(name string, cfg config.BackendConfig) (*OpenCodeHTTPBackend, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("opencode backend %q: base_url is required", name)
	}
	if cfg.WorkspaceMode != "" && cfg.WorkspaceMode != opencodeWorkspaceModeGatewayPath {
		return nil, fmt.Errorf("opencode backend %q: unsupported workspace_mode %q (first release only supports %q)",
			name, cfg.WorkspaceMode, opencodeWorkspaceModeGatewayPath)
	}

	timeout := opencodeDefaultTimeout
	if cfg.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}

	client := &http.Client{
		Timeout: timeout,
	}

	return &OpenCodeHTTPBackend{
		cfg:    cfg,
		client: client,
		name:   name,
	}, nil
}

// Name returns the backend name (e.g. "opencode-local").
func (b *OpenCodeHTTPBackend) Name() string { return b.name }

// Run creates a session, sends the user prompt as a text part, and waits for
// the response. The returned CodingResult carries the assistant's text summary
// and the remote session ID (for future continue support).
//
// Provider mapping:
//   - Agent.Provider is passed through as providerID; Agent.Model as modelID.
//   - If the provider/model combination is unknown to the OpenCode sidecar, it
//     will fall back to its own default (matching v4 §4.3).
//
// The actual file modifications happen on disk via the sidecar; Run does not
// touch the workspace. finalizeWriteChanges uses git.HasChanges() to decide
// whether to commit, regardless of what the summary says.
func (b *OpenCodeHTTPBackend) Run(ctx context.Context, req CodingRequest) (*CodingResult, error) {
	task := req.Task

	// 1. Create session
	sessionID, err := b.createSession(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create opencode session: %w", err)
	}
	log.Printf("[INFO] Task %d opencode session created: %s", task.ID, sessionID)

	// 2. Send message
	summary, err := b.sendMessage(ctx, sessionID, req)
	if err != nil {
		// Try to abort on failure so the sidecar doesn't keep working
		if abortErr := b.Abort(ctx, sessionID); abortErr != nil {
			log.Printf("[WARN] Failed to abort opencode session %s after message error: %v", sessionID, abortErr)
		}
		return nil, fmt.Errorf("opencode message: %w", err)
	}

	log.Printf("[INFO] Task %d opencode coding completed, summary len=%d", task.ID, len(summary))

	return &CodingResult{
		Summary:         summary,
		Success:         true,
		RemoteSessionID: sessionID,
		// Provider is nil for opencode backend — the LLM call is handled
		// server-side. finalizeWriteChanges will still generate a commit
		// message using the gateway's own provider (see note in finalize).
		Provider: nil,
	}, nil
}

// Abort issues POST /session/{id}/abort to the sidecar. It is best-effort —
// a network error does not fail the caller, only logs.
func (b *OpenCodeHTTPBackend) Abort(ctx context.Context, handle string) error {
	if handle == "" {
		return nil
	}
	url := fmt.Sprintf("%s/session/%s/abort", b.cfg.BaseURL, handle)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	b.setAuth(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("abort request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("abort returned status %d", resp.StatusCode)
	}
	return nil
}

// --- API helpers -----------------------------------------------------------

// opencodeCreateSessionResponse is the minimal response shape we need from
// POST /session. Full Session object has many more fields; we only need id.
type opencodeCreateSessionResponse struct {
	ID string `json:"id"`
}

// createSession calls POST /session and returns the new session id.
// Currently we do not pass title/cwd at create time — the message endpoint
// handles the working directory via the sidecar's own workspace binding.
func (b *OpenCodeHTTPBackend) createSession(ctx context.Context, req CodingRequest) (string, error) {
	url := b.cfg.BaseURL + "/session"

	body := map[string]interface{}{
		"title": fmt.Sprintf("gateway-task-%d", req.Task.ID),
	}

	httpReq, err := b.newJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return "", err
	}

	var resp opencodeCreateSessionResponse
	if err := b.doJSON(httpReq, &resp); err != nil {
		return "", err
	}
	if resp.ID == "" {
		return "", fmt.Errorf("session create returned empty id")
	}
	return resp.ID, nil
}

// opencodeMessageRequest maps to the /session/{id}/message request body.
type opencodeMessageRequest struct {
	ModelID    string                   `json:"modelID"`
	ProviderID string                   `json:"providerID"`
	Parts      []opencodeMessagePart    `json:"parts"`
	System     string                   `json:"system,omitempty"`
	Tools      *opencodeToolsConfig     `json:"tools,omitempty"`
	Directory  string                   `json:"directory,omitempty"`
}

type opencodeMessagePart struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

type opencodeToolsConfig struct {
	Search bool `json:"search"`
	Read   bool `json:"read"`
	Write  bool `json:"write"`
	Edit   bool `json:"edit"`
	// Command disabled by default for safety; sidecar manages its own perms.
}

// opencodeMessagesListItem is one entry from GET /session/{id}/message.
// Each item has info (role, id, etc.) and parts (text / tool / etc.).
type opencodeMessagesListItem struct {
	Info  opencodeMessageInfo   `json:"info"`
	Parts []opencodeMessagePart `json:"parts"`
}

type opencodeMessageInfo struct {
	ID   string `json:"id"`
	Role string `json:"role"` // "user" | "assistant"
}

// sendMessage calls POST /session/{id}/message and then polls
// GET /session/{id}/message to extract the assistant's text response.
//
// Note: the SDK uses streaming tokens (SSE) for real-time UI. For the gateway's
// headless use case, a synchronous POST + subsequent list-messages lookup is
// sufficient and simpler. The HTTP client timeout guards against hangs.
func (b *OpenCodeHTTPBackend) sendMessage(ctx context.Context, sessionID string, req CodingRequest) (string, error) {
	msgReq := opencodeMessageRequest{
		ModelID:    req.Agent.Model,
		ProviderID: req.Agent.Provider,
		Parts: []opencodeMessagePart{
			{Type: "text", Text: req.Prompt},
		},
		System:    req.SystemPrompt,
		Directory: req.WorkDir,
		Tools: &opencodeToolsConfig{
			Search: true,
			Read:   true,
			Write:  true,
			Edit:   true,
		},
	}

	// Inject override from backend_options if present
	if bo := req.BackendOptions; bo != nil {
		if v, ok := bo["opencode_model"].(string); ok && v != "" {
			msgReq.ModelID = v
		}
		if v, ok := bo["opencode_provider"].(string); ok && v != "" {
			msgReq.ProviderID = v
		}
		if v, ok := bo["opencode_agent"].(string); ok && v != "" {
			// opencode_agent is noted in v4 §4.3 but the server-side agent
			// selection mechanism differs between releases. Pass through as
			// a non-fatal hint; ignore if the endpoint doesn't consume it.
			log.Printf("[INFO] Task %d: opencode_agent=%q (informational)", req.Task.ID, v)
		}
	}

	url := fmt.Sprintf("%s/session/%s/message", b.cfg.BaseURL, sessionID)
	httpReq, err := b.newJSONRequest(ctx, http.MethodPost, url, msgReq)
	if err != nil {
		return "", err
	}

	// The POST /message endpoint is synchronous and returns when the run completes.
	// We ignore the response body (token stream shape varies) and instead fetch
	// the final assistant message from the messages list, which is more stable.
	if err := b.doJSON(httpReq, nil); err != nil {
		return "", err
	}

	// Fetch message list and extract the last assistant text part
	return b.getLastAssistantMessage(ctx, sessionID)
}

// getLastAssistantMessage fetches the message list and concatenates all
// text parts from the most recent assistant message.
func (b *OpenCodeHTTPBackend) getLastAssistantMessage(ctx context.Context, sessionID string) (string, error) {
	url := fmt.Sprintf("%s/session/%s/message", b.cfg.BaseURL, sessionID)
	httpReq, err := b.newJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	var messages []opencodeMessagesListItem
	if err := b.doJSON(httpReq, &messages); err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}

	// Walk backwards to find the last assistant message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Info.Role != "assistant" {
			continue
		}
		// Concatenate all text parts
		var texts []string
		for _, p := range messages[i].Parts {
			if p.Type == "text" && p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
		if len(texts) > 0 {
			// Join with double-newline to match multi-part natural flow
			return joinParts(texts), nil
		}
	}

	return "", fmt.Errorf("no assistant text message found in session %s", sessionID)
}

func joinParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += "\n\n" + p
	}
	return result
}

// --- HTTP plumbing ---------------------------------------------------------

func (b *OpenCodeHTTPBackend) newJSONRequest(ctx context.Context, method, url string, body interface{}) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	b.setAuth(req)
	return req, nil
}

func (b *OpenCodeHTTPBackend) setAuth(req *http.Request) {
	if b.cfg.Auth.Username != "" || b.cfg.Auth.Password != "" {
		req.SetBasicAuth(b.cfg.Auth.Username, b.cfg.Auth.Password)
	}
}

// doJSON executes httpReq and decodes the JSON response into out (if out != nil).
// Non-2xx responses return an error with the status and a truncated body.
func (b *OpenCodeHTTPBackend) doJSON(req *http.Request, out interface{}) error {
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Truncate long error bodies for readability
		snippet := string(body)
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		return fmt.Errorf("opencode API %s %s returned %d: %s",
			req.Method, req.URL.Path, resp.StatusCode, snippet)
	}

	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// --- Health check ----------------------------------------------------------

// HealthCheck calls the configured health endpoint and reports whether the
// sidecar is reachable and healthy. Returns nil on success, error otherwise.
//
// The health endpoint path is configurable (default "/health" if HealthCheck.Path
// is empty). The check is a simple GET; any 2xx status counts as healthy.
func (b *OpenCodeHTTPBackend) HealthCheck(ctx context.Context) error {
	path := b.cfg.HealthCheck.Path
	if path == "" {
		path = "/health"
	}
	url := b.cfg.BaseURL + path

	// Short timeout for health checks — we don't want to block the task queue
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	b.setAuth(req)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}
