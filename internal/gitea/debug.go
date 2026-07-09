package gitea

import (
	"encoding/json"
	"net/http"
	"strings"

	"gitea-agent-gateway/internal/logging"
)

const debugBodyLimit = 512

var sensitiveKeys = []string{
	"password", "token", "admin_token", "api_key", "secret",
	"webhook_secret", "sha1", "authorization",
}

func logGiteaRequest(req *http.Request, reqBody []byte) {
	if !logging.LevelEnabled(logging.LevelDebug) {
		return
	}
	path := requestPath(req)
	bodyPreview := sanitizeBodyForLog(reqBody)
	if bodyPreview == "" {
		logging.Debugf("Gitea API → %s %s", req.Method, path)
		return
	}
	logging.Debugf("Gitea API → %s %s body=%s", req.Method, path, bodyPreview)
}

func logGiteaResponse(req *http.Request, status int, respBody []byte) {
	if !logging.LevelEnabled(logging.LevelDebug) {
		return
	}
	path := requestPath(req)
	preview := sanitizeBodyForLog(respBody)
	if preview == "" {
		logging.Debugf("Gitea API ← %d %s %s", status, req.Method, path)
		return
	}
	logging.Debugf("Gitea API ← %d %s %s body=%dB preview=%s", status, req.Method, path, len(respBody), preview)
}

func logGiteaTransportError(req *http.Request, err error) {
	if !logging.LevelEnabled(logging.LevelDebug) {
		return
	}
	logging.Debugf("Gitea API ✗ %s %s transport error: %v", req.Method, req.URL.Path, err)
}

func truncateForLog(s string, limit int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "...(truncated)"
}

func requestPath(req *http.Request) string {
	path := req.URL.Path
	if req.URL.RawQuery != "" {
		path += "?" + req.URL.RawQuery
	}
	return path
}

func sanitizeBodyForLog(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var v interface{}
	if err := json.Unmarshal(body, &v); err != nil {
		return truncateForLog(string(body), debugBodyLimit)
	}
	redactSensitive(v)
	data, err := json.Marshal(v)
	if err != nil {
		return truncateForLog(string(body), debugBodyLimit)
	}
	return truncateForLog(string(data), debugBodyLimit)
}

func redactSensitive(v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			if isSensitiveKey(k) {
				t[k] = "***"
				continue
			}
			redactSensitive(val)
		}
	case []interface{}:
		for _, item := range t {
			redactSensitive(item)
		}
	}
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if lower == s || strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
