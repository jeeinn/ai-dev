package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/gitea"
	"gitea-agent-gateway/internal/llm"
)

// --- System Config endpoints ---

func (h *Handler) getConfig(w http.ResponseWriter, r *http.Request) {
	if h.cfgManager == nil {
		writeError(w, 500, "config manager not initialized")
		return
	}
	display, err := h.cfgManager.GetDisplayMap()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, display)
}

func (h *Handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	if h.cfgManager == nil {
		writeError(w, 500, "config manager not initialized")
		return
	}

	var entries map[string]string
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		writeError(w, 400, "invalid request body: expected {\"key\": \"value\", ...}")
		return
	}

	if len(entries) == 0 {
		writeError(w, 400, "no config entries provided")
		return
	}

	// Validate all keys first
	for key := range entries {
		if !config.IsConfigKey(key) {
			writeError(w, 400, fmt.Sprintf("invalid config key: %s", key))
			return
		}
	}

	// Apply all entries
	for key, value := range entries {
		if err := h.cfgManager.Update(key, value); err != nil {
			writeError(w, 400, err.Error())
			return
		}
	}

	h.notifyConfigChange()

	display, err := h.cfgManager.GetDisplayMap()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, display)
}

func (h *Handler) deleteConfigEntry(w http.ResponseWriter, r *http.Request) {
	if h.cfgManager == nil {
		writeError(w, 500, "config manager not initialized")
		return
	}

	key := r.PathValue("key")
	if key == "" {
		writeError(w, 400, "missing config key")
		return
	}

	if err := h.cfgManager.Delete(key); err != nil {
		writeError(w, 500, err.Error())
		return
	}

	h.notifyConfigChange()

	display, err := h.cfgManager.GetDisplayMap()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, display)
}

func (h *Handler) getProviderModels(w http.ResponseWriter, r *http.Request) {
	if h.cfgManager == nil {
		writeError(w, 500, "config manager not initialized")
		return
	}

	providerName := r.PathValue("name")
	if providerName == "" {
		writeError(w, 400, "missing provider name")
		return
	}

	models, source, err := h.cfgManager.GetProviderModels(providerName)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{
			"success":  false,
			"error":    err.Error(),
			"fallback_source": source,
			"source":   source,
			"models":   models,
		})
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"success": true,
		"source":  source,
		"models":  models,
	})
}

func (h *Handler) notifyConfigChange() {
	if h.onConfigChange != nil {
		h.onConfigChange(h.cfgManager.Get())
	}
}

func (h *Handler) testGiteaConfig(w http.ResponseWriter, r *http.Request) {
	var payload map[string]string
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}

	url := strings.TrimSpace(firstNonEmpty(payload["gitea.url"], h.stringConfigValue("gitea.url")))
	token := strings.TrimSpace(firstNonEmpty(payload["gitea.admin_token"], h.stringConfigValue("gitea.admin_token")))

	client := gitea.NewClient(url, token)
	result, err := client.TestConnection()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	status := http.StatusOK
	if !result.OK {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}

func (h *Handler) testLLMConfig(w http.ResponseWriter, r *http.Request) {
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}

	providerName := strings.TrimSpace(firstNonEmpty(
		asString(payload["llm.defaults.provider"]),
		h.stringConfigValue("llm.defaults.provider"),
	))
	if providerName == "" {
		writeError(w, 400, "默认 Provider 不能为空")
		return
	}

	providers := h.resolveProvidersForTest(payload)
	pcfg, ok := providers[providerName]
	if !ok {
		writeError(w, 400, fmt.Sprintf("Provider %q 未配置 API Key / Base URL", providerName))
		return
	}
	if strings.TrimSpace(pcfg.APIKey) == "" {
		writeError(w, 400, fmt.Sprintf("Provider %q 的 api_key 不能为空", providerName))
		return
	}

	var provider llm.Provider
	if strings.EqualFold(providerName, "claude") || strings.EqualFold(providerName, "anthropic") {
		provider = llm.NewAnthropicProvider(pcfg.APIKey)
	} else {
		baseURL := pcfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		provider = llm.NewOpenAICompatibleProvider(baseURL, pcfg.APIKey)
	}

	model := strings.TrimSpace(firstNonEmpty(
		asString(payload["llm.defaults.model"]),
		h.stringConfigValue("llm.defaults.model"),
		"deepseek-chat",
	))

		maxTokens := 8
		if v := firstNonEmpty(
			asString(payload["agents.defaults.max_output_tokens"]),
			h.stringConfigValue("agents.defaults.max_output_tokens"),
		); v != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				maxTokens = n
			}
		}
		if maxTokens > 16 {
			maxTokens = 16
		}
		if maxTokens <= 0 {
			maxTokens = 8
		}

		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		resp, err := provider.ChatCompletion(ctx, &llm.ChatRequest{
			Model: model,
			Messages: []llm.Message{
				{Role: "user", Content: "ping"},
			},
			MaxTokens:   maxTokens,
			Temperature: 0,
		})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"ok":      false,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"provider": providerName,
		"model":    model,
		"message":  fmt.Sprintf("连接成功，模型响应: %s", strings.TrimSpace(resp.Content)),
	})
}

func (h *Handler) stringConfigValue(key string) string {
	if h.cfgManager == nil {
		return ""
	}
	display, err := h.cfgManager.GetDisplayMap()
	if err != nil {
		return ""
	}
	val, ok := display[key]
	if !ok {
		return ""
	}
	return asString(val)
}

func (h *Handler) resolveProvidersForTest(payload map[string]interface{}) map[string]config.ProviderConfig {
	if raw, ok := payload["llm.providers"]; ok {
		providers, err := config.ParseProvidersFromInterface(raw)
		if err == nil && len(providers) > 0 {
			return providers
		}
	}

	display, err := h.cfgManager.GetDisplayMap()
	if err != nil {
		return nil
	}
	if raw, ok := display["llm.providers"]; ok {
		providers, err := config.ParseProvidersFromInterface(raw)
		if err == nil {
			return providers
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return ""
	}
}
