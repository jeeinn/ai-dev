package config

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Store is the interface for system_config CRUD (avoids circular dependency with store package).
type Store interface {
	GetConfig(key string) (string, error)
	SetConfig(key, value string) error
	DeleteConfig(key string) error
	ListConfigs() (map[string]string, error)
}

// ConfigManager manages configuration with DB overrides on top of file config.
type ConfigManager struct {
	mu         sync.RWMutex
	base       *Config // file-based config (immutable baseline)
	active     *Config // merged config (base + DB overrides)
	store      Store   // DB store for persistence
	modelCache map[string]modelCacheEntry
}

type modelCacheEntry struct {
	models    []ModelDefinition
	source    string // api | builtin | custom
	expiresAt time.Time
}

// ModelDiscoveryFunc is a function that can discover models for a provider.
// Returns model IDs and an error.
type ModelDiscoveryFunc func(providerName string, baseURL, apiKey string, providerType string) ([]string, error)

var modelDiscoveryFn ModelDiscoveryFunc

// SetModelDiscoveryFunc registers a function for dynamic model discovery.
// This is called at startup to avoid circular imports.
func SetModelDiscoveryFunc(fn ModelDiscoveryFunc) {
	modelDiscoveryFn = fn
}

// NewConfigManager creates a ConfigManager from a file-loaded config.
func NewConfigManager(fileCfg *Config) *ConfigManager {
	active := copyConfig(fileCfg)
	return &ConfigManager{
		base:   fileCfg,
		active: active,
	}
}

// SetStore sets the DB store for persistence. Must be called before ApplyDBOverrides.
func (m *ConfigManager) SetStore(s Store) {
	m.store = s
}

// ApplyDBOverrides loads DB config entries and merges them into the active config.
func (m *ConfigManager) ApplyDBOverrides() error {
	if m.store == nil {
		return nil
	}

	entries, err := m.store.ListConfigs()
	if err != nil {
		return fmt.Errorf("load db configs: %w", err)
	}

	if len(entries) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, value := range entries {
		if !IsConfigKey(key) {
			log.Printf("[WARN] Ignore non-config DB key %q (value: %s). If this is no longer needed, delete it via: DELETE /api/config/%s", key, value, key)
			continue
		}
		if err := applyConfigEntry(m.active, key, value); err != nil {
			log.Printf("[WARN] Skip invalid DB config %s=%s: %v. Fix via: PUT /api/config with correct value, or DELETE /api/config/%s", key, value, err, key)
		}
	}

	log.Printf("[INFO] Applied %d DB config overrides", len(entries))
	return nil
}

// MigrateLegacyConfigKeys remaps deprecated system_config keys to the new schema once.
func (m *ConfigManager) MigrateLegacyConfigKeys() error {
	if m.store == nil {
		return nil
	}
	entries, err := m.store.ListConfigs()
	if err != nil {
		return fmt.Errorf("list configs for migration: %w", err)
	}

	// agents.defaults.max_tokens / agents.loop.max_tokens → max_output_tokens (take max)
	out := 0
	if v, ok := entries["agents.defaults.max_output_tokens"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			out = n
		}
	}
	for _, key := range []string{"agents.defaults.max_tokens", "agents.loop.max_tokens", "llm.defaults.max_tokens"} {
		if v, ok := entries[key]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > out {
				out = n
			}
		}
	}
	if out > 0 {
		if err := m.store.SetConfig("agents.defaults.max_output_tokens", strconv.Itoa(out)); err != nil {
			return err
		}
	}

	// temperature: llm.defaults → agents.defaults if agents missing
	if _, ok := entries["agents.defaults.temperature"]; !ok {
		if v, ok := entries["llm.defaults.temperature"]; ok && strings.TrimSpace(v) != "" {
			if err := m.store.SetConfig("agents.defaults.temperature", v); err != nil {
				return err
			}
		}
	}

	// dispatcher.retry_count → task_retry_count; also seed llm.rate_limit_retries once
	if _, ok := entries["dispatcher.task_retry_count"]; !ok {
		if v, ok := entries["dispatcher.retry_count"]; ok && strings.TrimSpace(v) != "" {
			if err := m.store.SetConfig("dispatcher.task_retry_count", v); err != nil {
				return err
			}
		}
	}
	if _, ok := entries["llm.rate_limit_retries"]; !ok {
		if v, ok := entries["dispatcher.retry_count"]; ok && strings.TrimSpace(v) != "" {
			if err := m.store.SetConfig("llm.rate_limit_retries", v); err != nil {
				return err
			}
		}
	}

	legacyKeys := []string{
		"llm.defaults.max_tokens",
		"llm.defaults.temperature",
		"agents.defaults.max_tokens",
		"agents.loop.max_tokens",
		"agents.loop.timeout",
		"dispatcher.timeout",
		"dispatcher.retry_count",
	}
	for _, key := range legacyKeys {
		if _, ok := entries[key]; ok {
			if err := m.store.DeleteConfig(key); err != nil {
				log.Printf("[WARN] Failed to delete legacy config key %s: %v", key, err)
			} else {
				log.Printf("[INFO] Removed legacy config key: %s", key)
			}
		}
	}
	return nil
}

// Get returns a deep copy of the active (merged) config.
// Callers may retain the result without racing config hot-updates.
func (m *ConfigManager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return copyConfig(m.active)
}

// Update sets a config entry: writes to DB and applies to active config in memory.
func (m *ConfigManager) Update(key, value string) error {
	m.mu.RLock()
	testCfg := copyConfig(m.active)
	m.mu.RUnlock()
	if err := applyConfigEntry(testCfg, key, value); err != nil {
		return fmt.Errorf("invalid value for %s: %w", key, err)
	}

	// Persist to DB
	if m.store != nil {
		if err := m.store.SetConfig(key, value); err != nil {
			return fmt.Errorf("persist config: %w", err)
		}
	}

	// Apply to a new snapshot so concurrent Get() holders keep a stable copy.
	m.mu.Lock()
	next := copyConfig(m.active)
	if err := applyConfigEntry(next, key, value); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("apply config: %w", err)
	}
	m.active = next
	m.mu.Unlock()

	// Invalidate model cache when providers change
	if key == "llm.providers" {
		m.InvalidateAllModelCache()
	}

	log.Printf("[INFO] Config updated: %s = %s", key, value)
	return nil
}

// Delete removes a DB override, reverting to file config default.
func (m *ConfigManager) Delete(key string) error {
	if m.store != nil {
		if err := m.store.DeleteConfig(key); err != nil {
			return fmt.Errorf("delete config: %w", err)
		}
	}

	// Reset to file config value on a new snapshot.
	m.mu.Lock()
	fileVal := getConfigEntry(m.base, key)
	next := copyConfig(m.active)
	if err := applyConfigEntry(next, key, fileVal); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("revert config: %w", err)
	}
	m.active = next
	m.mu.Unlock()

	// Invalidate model cache when providers change
	if key == "llm.providers" {
		m.InvalidateAllModelCache()
	}

	log.Printf("[INFO] Config reverted to default: %s", key)
	return nil
}

// GetMap returns the current config as a flat map for API response.
// Display values prefer DB entries; empty/missing DB values fall back to file config.
func (m *ConfigManager) GetMap() map[string]interface{} {
	display, err := m.GetDisplayMap()
	if err != nil {
		log.Printf("[WARN] GetDisplayMap failed, falling back to active config: %v", err)
		return m.getActiveMap()
	}
	return display
}

// GetDisplayMap returns config for Web UI display.
// Per-field: non-empty DB value wins; otherwise file config (config.yaml) is used.
// JSON-type fields with semantically empty DB values (e.g. "{}") fall back to file config.
// Sensitive fields are returned as real values (for editing).
func (m *ConfigManager) GetDisplayMap() (map[string]interface{}, error) {
	dbEntries := map[string]string{}
	if m.store != nil {
		entries, err := m.store.ListConfigs()
		if err != nil {
			return nil, fmt.Errorf("load db configs: %w", err)
		}
		dbEntries = entries
	}

	m.mu.RLock()
	base := m.base
	m.mu.RUnlock()

	result := make(map[string]interface{}, len(configKeys)+1)
	sources := make(map[string]string, len(configKeys))

	for _, key := range configKeys {
		if dbVal, ok := dbEntries[key]; ok && strings.TrimSpace(dbVal) != "" && !isSemanticallyEmptyConfigValue(key, dbVal) {
			val, err := parseConfigValue(key, dbVal)
			if err != nil {
				return nil, fmt.Errorf("invalid db config %s: %w", key, err)
			}
			result[key] = val
			sources[key] = "db"
			continue
		}
		val := getConfigValueTyped(base, key)
		result[key] = val
		sources[key] = "file"
	}

	// Build models metadata from active (merged) providers so DB overlays are visible.
	modelsMeta := make(map[string][]ModelDefinition)
	m.mu.RLock()
	activeProviders := m.active.LLM.Providers
	activeBackends := m.active.Agents.Backends
	m.mu.RUnlock()
	for name, pc := range activeProviders {
		models, _ := m.resolveProviderModels(name, pc)
		modelsMeta[name] = models
	}

	backendsMeta := make([]map[string]interface{}, 0, len(activeBackends.Backends)+1)
	backendsMeta = append(backendsMeta, map[string]interface{}{
		"name": "internal",
		"type": BackendTypeBuiltin,
	})
	for name, bc := range activeBackends.Backends {
		if name == "internal" {
			continue
		}
		backendsMeta = append(backendsMeta, map[string]interface{}{
			"name": name,
			"type": bc.Type,
		})
	}

	result["_meta"] = map[string]interface{}{
		"sources":          sources,
		"models":           modelsMeta,
		"backends":         backendsMeta,
		"backends_default": activeBackends.Default,
		"workflow_presets": []map[string]interface{}{
			{"name": "free", "label": "自由模式", "description": "最小限制，允许跳过分析直接开发"},
			{"name": "standard", "label": "标准模式", "description": "平衡配置，开发中重新分析会警告"},
			{"name": "strict", "label": "严格模式", "description": "最大限制，强制分析后才能开发"},
		},
	}
	return result, nil
}

// resolveProviderModels determines the effective model list for a provider.
// Priority: user-defined (non-empty) > builtin catalog.
// Returns the models and their source ("custom" or "builtin").
func (m *ConfigManager) resolveProviderModels(name string, pc ProviderConfig) ([]ModelDefinition, string) {
	if len(pc.Models) > 0 {
		return pc.Models, "custom"
	}
	if builtin, ok := BuiltinModelCatalog[name]; ok {
		return builtin, "builtin"
	}
	return nil, ""
}

// modelDiscoveryCacheTTL is the cache duration for dynamically discovered models.
const modelDiscoveryCacheTTL = 1 * time.Hour

// GetProviderModels returns the effective model list for a provider.
// This is used by the Web UI for model selection dropdowns.
// getProviderConfig returns the ProviderConfig from the active (merged) config.
func (m *ConfigManager) getProviderConfig(providerName string) (ProviderConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pc, ok := m.active.LLM.Providers[providerName]
	return pc, ok
}

// Behavior depends on ProviderConfig.Models:
//   - nil / unset: returns BuiltinModelCatalog, source="builtin"
//   - empty slice []: attempts dynamic discovery via /models API; on failure falls back to builtin
//   - non-empty: returns user-defined models, source="custom"
func (m *ConfigManager) GetProviderModels(providerName string) ([]ModelDefinition, string, error) {
	pc, ok := m.getProviderConfig(providerName)
	if !ok {
		return nil, "", fmt.Errorf("provider not found: %s", providerName)
	}

	// Check in-memory cache
	m.mu.RLock()
	if cache, ok := m.modelCache[providerName]; ok && time.Now().Before(cache.expiresAt) {
		m.mu.RUnlock()
		return cache.models, cache.source, nil
	}
	m.mu.RUnlock()

	var models []ModelDefinition
	var source string
	var err error

	switch {
	case pc.Models == nil:
		// Unset → builtin
		models, source = m.resolveProviderModels(providerName, pc)

	case len(pc.Models) == 0:
		// Empty → dynamic discovery
		models, source, err = m.discoverModels(providerName, pc)
		if err != nil {
			// Fall back to builtin on error, but return error info
			models, source = m.resolveProviderModels(providerName, pc)
			if models == nil {
				models = []ModelDefinition{}
			}
			// Cache short-lived fallback result
			m.cacheModels(providerName, models, source, 5*time.Minute)
			return models, source, fmt.Errorf("dynamic discovery failed, using fallback: %w", err)
		}

	default:
		// Non-empty → custom
		models, source = m.resolveProviderModels(providerName, pc)
	}

	if models == nil {
		models = []ModelDefinition{}
	}

	m.cacheModels(providerName, models, source, modelDiscoveryCacheTTL)
	return models, source, nil
}

// cacheModels stores the model list in the in-memory cache.
func (m *ConfigManager) cacheModels(providerName string, models []ModelDefinition, source string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.modelCache == nil {
		m.modelCache = make(map[string]modelCacheEntry)
	}
	m.modelCache[providerName] = modelCacheEntry{
		models:    models,
		source:    source,
		expiresAt: time.Now().Add(ttl),
	}
}

// InvalidateModelCache clears the cached model list for a provider.
func (m *ConfigManager) InvalidateModelCache(providerName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.modelCache, providerName)
}

// InvalidateAllModelCache clears all cached model lists.
func (m *ConfigManager) InvalidateAllModelCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modelCache = nil
}

// GetModelMeta returns the metadata for a specific model in a provider.
// Returns nil if the model is not found.
// Discovery errors are ignored here: GetProviderModels already returns fallback
// models (builtin/custom) alongside the error, and callers still need metadata.
func (m *ConfigManager) GetModelMeta(providerName, modelID string) *ModelDefinition {
	models, _, _ := m.GetProviderModels(providerName)
	for i := range models {
		if models[i].ID == modelID {
			return &models[i]
		}
	}
	return nil
}

// discoverModels attempts to fetch the model list from the provider's API.
// Uses the injected ModelDiscoveryFunc (if set) to avoid circular imports.
func (m *ConfigManager) discoverModels(providerName string, pc ProviderConfig) ([]ModelDefinition, string, error) {
	if modelDiscoveryFn == nil {
		// No discovery function configured → fall back to builtin
		builtin, source := m.resolveProviderModels(providerName, pc)
		if builtin == nil {
			builtin = []ModelDefinition{}
		}
		return builtin, source, nil
	}

	providerType := pc.Type
	if providerType == "" {
		providerType = "openai_compatible"
	}

	ids, err := modelDiscoveryFn(providerName, pc.BaseURL, pc.APIKey, providerType)
	if err != nil {
		return nil, "", fmt.Errorf("list models: %w", err)
	}

	// Merge with builtin catalog to enrich metadata
	builtin := BuiltinModelCatalog[providerName]
	builtinMap := make(map[string]ModelDefinition, len(builtin))
	for _, bm := range builtin {
		builtinMap[bm.ID] = bm
	}

	result := make([]ModelDefinition, 0, len(ids))
	for _, id := range ids {
		if bm, ok := builtinMap[id]; ok {
			result = append(result, bm)
		} else {
			result = append(result, ModelDefinition{
				ID:   id,
				Name: id,
			})
		}
	}
	return result, "api", nil
}

func (m *ConfigManager) getActiveMap() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.active
	return map[string]interface{}{
		"gitea.url":                                cfg.Gitea.URL,
		"gitea.admin_token":                        cfg.Gitea.AdminToken,
		"gitea.webhook_secret":                     cfg.Gitea.WebhookSecret,
		"llm.defaults.provider":                    cfg.LLM.Defaults.Provider,
		"llm.defaults.model":                       cfg.LLM.Defaults.Model,
		"llm.providers":                            cfg.LLM.Providers,
		"dispatcher.max_concurrent":                cfg.Dispatcher.MaxConcurrent,
		"dispatcher.task_retry_count":              cfg.Dispatcher.TaskRetryCount,
		"dispatcher.rate_limit_backoff":            cfg.Dispatcher.RateLimitBackoff,
		"llm.rate_limit_retries":                   cfg.LLM.RateLimitRetries,
		"agents.defaults.provider":                 cfg.Agents.Defaults.Provider,
		"agents.defaults.model":                    cfg.Agents.Defaults.Model,
		"agents.defaults.max_output_tokens":        cfg.Agents.Defaults.MaxOutputTokens,
		"agents.defaults.max_input_tokens":         cfg.Agents.Defaults.MaxInputTokens,
		"agents.defaults.temperature":              cfg.Agents.Defaults.Temperature,
		"agents.defaults.timeout":                  cfg.Agents.Defaults.Timeout,
		"agents.loop.max_iterations":               cfg.Agents.Loop.MaxIterations,
		"agents.loop.total_timeout":                cfg.Agents.Loop.TotalTimeout,
		"agents.loop.iteration_interval":           cfg.Agents.Loop.IterationInterval,
		"agents.loop.no_progress_limit":            cfg.Agents.Loop.NoProgressLimit,
		"agents.loop.verify_commands":              cfg.Agents.Loop.VerifyCommands,
		"debug.conversation_log.enabled":           cfg.Debug.ConversationLog.Enabled,
		"debug.conversation_log.max_content_chars": cfg.Debug.ConversationLog.MaxContentChars,
	}
}

var configKeys = []string{
	"gitea.url",
	"gitea.admin_token",
	"gitea.webhook_secret",
	"llm.defaults.provider",
	"llm.defaults.model",
	"llm.providers",
	"dispatcher.max_concurrent",
	"dispatcher.task_retry_count",
	"dispatcher.rate_limit_backoff",
	"llm.rate_limit_retries",
	"agents.defaults.provider",
	"agents.defaults.model",
	"agents.defaults.max_output_tokens",
	"agents.defaults.max_input_tokens",
	"agents.defaults.temperature",
	"agents.defaults.timeout",
	"agents.loop.max_iterations",
	"agents.loop.total_timeout",
	"agents.loop.iteration_interval",
	"agents.loop.no_progress_limit",
	"agents.loop.verify_commands",
	"debug.conversation_log.enabled",
	"debug.conversation_log.max_content_chars",
	"workflow.preset",
	"workflow.gates",
}

func parseConfigValue(key, value string) (interface{}, error) {
	switch key {
	case "dispatcher.max_concurrent", "dispatcher.task_retry_count", "dispatcher.rate_limit_backoff",
		"llm.rate_limit_retries",
		"agents.defaults.max_output_tokens", "agents.defaults.max_input_tokens",
		"agents.loop.max_iterations", "agents.loop.iteration_interval",
		"agents.loop.no_progress_limit",
		"debug.conversation_log.max_content_chars":
		n, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("not a number: %s", value)
		}
		return n, nil
	case "agents.defaults.temperature":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("not a float: %s", value)
		}
		return f, nil
	case "debug.conversation_log.enabled":
		b, err := parseBoolValue(value)
		if err != nil {
			return nil, err
		}
		return b, nil
	case "llm.providers":
		var providers map[string]ProviderConfig
		if err := json.Unmarshal([]byte(value), &providers); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return providers, nil
	case "agents.loop.verify_commands":
		var cmds []string
		if err := json.Unmarshal([]byte(value), &cmds); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return cmds, nil
	default:
		return value, nil
	}
}

func getConfigValueTyped(cfg *Config, key string) interface{} {
	switch key {
	case "gitea.url":
		return cfg.Gitea.URL
	case "gitea.admin_token":
		return cfg.Gitea.AdminToken
	case "gitea.webhook_secret":
		return cfg.Gitea.WebhookSecret
	case "llm.defaults.provider":
		return cfg.LLM.Defaults.Provider
	case "llm.defaults.model":
		return cfg.LLM.Defaults.Model
	case "llm.providers":
		return cfg.LLM.Providers
	case "dispatcher.max_concurrent":
		return cfg.Dispatcher.MaxConcurrent
	case "dispatcher.task_retry_count":
		return cfg.Dispatcher.TaskRetryCount
	case "dispatcher.rate_limit_backoff":
		return cfg.Dispatcher.RateLimitBackoff
	case "llm.rate_limit_retries":
		return cfg.LLM.RateLimitRetries
	case "agents.defaults.provider":
		return cfg.Agents.Defaults.Provider
	case "agents.defaults.model":
		return cfg.Agents.Defaults.Model
	case "agents.defaults.max_output_tokens":
		return cfg.Agents.Defaults.MaxOutputTokens
	case "agents.defaults.max_input_tokens":
		return cfg.Agents.Defaults.MaxInputTokens
	case "agents.defaults.temperature":
		return cfg.Agents.Defaults.Temperature
	case "agents.defaults.timeout":
		return cfg.Agents.Defaults.Timeout
	case "agents.loop.max_iterations":
		return cfg.Agents.Loop.MaxIterations
	case "agents.loop.total_timeout":
		return cfg.Agents.Loop.TotalTimeout
	case "agents.loop.iteration_interval":
		return cfg.Agents.Loop.IterationInterval
	case "agents.loop.no_progress_limit":
		return cfg.Agents.Loop.NoProgressLimit
	case "agents.loop.verify_commands":
		return cfg.Agents.Loop.VerifyCommands
	case "debug.conversation_log.enabled":
		return cfg.Debug.ConversationLog.Enabled
	case "debug.conversation_log.max_content_chars":
		return cfg.Debug.ConversationLog.MaxContentChars
	case "workflow.preset":
		return cfg.Workflow.Preset
	case "workflow.gates":
		return cfg.Workflow.Gates
	default:
		return ""
	}
}

// GetMapLegacy returns the active merged config without display metadata.
func (m *ConfigManager) GetMapLegacy() map[string]interface{} {
	return m.getActiveMap()
}

// applyConfigEntry sets a single config key on the Config struct.
func applyConfigEntry(cfg *Config, key, value string) error {
	switch key {
	case "gitea.url":
		cfg.Gitea.URL = value
	case "gitea.admin_token":
		cfg.Gitea.AdminToken = value
	case "gitea.webhook_secret":
		cfg.Gitea.WebhookSecret = value
	case "llm.defaults.provider":
		cfg.LLM.Defaults.Provider = value
	case "llm.defaults.model":
		cfg.LLM.Defaults.Model = value
	case "llm.providers":
		providers, err := ParseProvidersJSON(value)
		if err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		cfg.LLM.Providers = providers
	case "dispatcher.max_concurrent":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Dispatcher.MaxConcurrent = n
	case "dispatcher.task_retry_count":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Dispatcher.TaskRetryCount = n
	case "dispatcher.rate_limit_backoff":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Dispatcher.RateLimitBackoff = n
	case "llm.rate_limit_retries":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.LLM.RateLimitRetries = n
	case "agents.defaults.provider":
		cfg.Agents.Defaults.Provider = value
	case "agents.defaults.model":
		cfg.Agents.Defaults.Model = value
	case "agents.defaults.max_output_tokens":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Agents.Defaults.MaxOutputTokens = n
	case "agents.defaults.max_input_tokens":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Agents.Defaults.MaxInputTokens = n
	case "agents.defaults.temperature":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("not a float: %s", value)
		}
		cfg.Agents.Defaults.Temperature = f
	case "agents.defaults.timeout":
		cfg.Agents.Defaults.Timeout = value
	case "agents.loop.max_iterations":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Agents.Loop.MaxIterations = n
		if err := ValidateAgentLoopConfig(cfg.Agents.Loop); err != nil {
			return err
		}
	case "agents.loop.total_timeout":
		cfg.Agents.Loop.TotalTimeout = value
		if err := ValidateAgentLoopConfig(cfg.Agents.Loop); err != nil {
			return err
		}
	case "agents.loop.iteration_interval":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Agents.Loop.IterationInterval = n
	case "agents.loop.no_progress_limit":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Agents.Loop.NoProgressLimit = n
	case "agents.loop.verify_commands":
		var cmds []string
		if err := json.Unmarshal([]byte(value), &cmds); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		cfg.Agents.Loop.VerifyCommands = cmds
	case "debug.conversation_log.enabled":
		b, err := parseBoolValue(value)
		if err != nil {
			return fmt.Errorf("not a boolean: %s", value)
		}
		cfg.Debug.ConversationLog.Enabled = b
	case "debug.conversation_log.max_content_chars":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Debug.ConversationLog.MaxContentChars = n
	case "workflow.preset":
		cfg.Workflow.Preset = value
	case "workflow.gates":
		var gates map[string]string
		if err := json.Unmarshal([]byte(value), &gates); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		cfg.Workflow.Gates = gates
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

// IsConfigKey returns true if the key is a recognized config field (not a data key like prompt.templates).
func IsConfigKey(key string) bool {
	for _, k := range configKeys {
		if k == key {
			return true
		}
	}
	return false
}

// getConfigEntry reads a single config value from a Config struct.
func getConfigEntry(cfg *Config, key string) string {
	switch key {
	case "gitea.url":
		return cfg.Gitea.URL
	case "gitea.admin_token":
		return cfg.Gitea.AdminToken
	case "gitea.webhook_secret":
		return cfg.Gitea.WebhookSecret
	case "llm.defaults.provider":
		return cfg.LLM.Defaults.Provider
	case "llm.defaults.model":
		return cfg.LLM.Defaults.Model
	case "llm.providers":
		data, _ := json.Marshal(cfg.LLM.Providers)
		return string(data)
	case "dispatcher.max_concurrent":
		return strconv.Itoa(cfg.Dispatcher.MaxConcurrent)
	case "dispatcher.task_retry_count":
		return strconv.Itoa(cfg.Dispatcher.TaskRetryCount)
	case "dispatcher.rate_limit_backoff":
		return strconv.Itoa(cfg.Dispatcher.RateLimitBackoff)
	case "llm.rate_limit_retries":
		return strconv.Itoa(cfg.LLM.RateLimitRetries)
	case "agents.defaults.provider":
		return cfg.Agents.Defaults.Provider
	case "agents.defaults.model":
		return cfg.Agents.Defaults.Model
	case "agents.defaults.max_output_tokens":
		return strconv.Itoa(cfg.Agents.Defaults.MaxOutputTokens)
	case "agents.defaults.max_input_tokens":
		return strconv.Itoa(cfg.Agents.Defaults.MaxInputTokens)
	case "agents.defaults.temperature":
		return fmt.Sprintf("%g", cfg.Agents.Defaults.Temperature)
	case "agents.defaults.timeout":
		return cfg.Agents.Defaults.Timeout
	case "agents.loop.max_iterations":
		return strconv.Itoa(cfg.Agents.Loop.MaxIterations)
	case "agents.loop.total_timeout":
		return cfg.Agents.Loop.TotalTimeout
	case "agents.loop.iteration_interval":
		return strconv.Itoa(cfg.Agents.Loop.IterationInterval)
	case "agents.loop.no_progress_limit":
		return strconv.Itoa(cfg.Agents.Loop.NoProgressLimit)
	case "agents.loop.verify_commands":
		data, _ := json.Marshal(cfg.Agents.Loop.VerifyCommands)
		return string(data)
	case "debug.conversation_log.enabled":
		return strconv.FormatBool(cfg.Debug.ConversationLog.Enabled)
	case "debug.conversation_log.max_content_chars":
		return strconv.Itoa(cfg.Debug.ConversationLog.MaxContentChars)
	default:
		return ""
	}
}

// copyConfig deep-copies a Config struct.
func copyConfig(src *Config) *Config {
	dst := *src
	// Deep copy map fields
	if src.LLM.Providers != nil {
		dst.LLM.Providers = make(map[string]ProviderConfig, len(src.LLM.Providers))
		for k, v := range src.LLM.Providers {
			dst.LLM.Providers[k] = deepCopyProviderConfig(v)
		}
	}
	if src.Agents.Templates != nil {
		dst.Agents.Templates = make(map[string]AgentTemplateConfig, len(src.Agents.Templates))
		for k, v := range src.Agents.Templates {
			dst.Agents.Templates[k] = v
		}
	}
	return &dst
}

func deepCopyProviderConfig(src ProviderConfig) ProviderConfig {
	dst := src
	if len(src.Models) > 0 {
		dst.Models = make([]ModelDefinition, len(src.Models))
		copy(dst.Models, src.Models)
	}
	return dst
}

func parseBoolValue(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean: %s", value)
	}
}

// isSemanticallyEmptyConfigValue checks whether a DB config value is semantically empty
// even though it is non-empty as a raw string. This prevents values like "{}" or "null"
// from blocking fallback to file config for JSON-type fields.
func isSemanticallyEmptyConfigValue(key, dbVal string) bool {
	trimmed := strings.TrimSpace(dbVal)
	switch key {
	case "llm.providers":
		// "{}" or "null" represent an empty providers map — treat as unset
		if trimmed == "{}" || trimmed == "null" {
			return true
		}
		// Also check parsed JSON: a valid map with zero entries is semantically empty
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &m); err == nil && len(m) == 0 {
			return true
		}
	case "agents.loop.verify_commands":
		// "null" means unset → fall back to file config.
		// "[]" is intentional disable and must NOT fall back.
		if trimmed == "null" {
			return true
		}
	}
	return false
}
