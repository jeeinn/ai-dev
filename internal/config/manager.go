package config

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
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
	mu     sync.RWMutex
	base   *Config // file-based config (immutable baseline)
	active *Config // merged config (base + DB overrides)
	store  Store   // DB store for persistence
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

// Get returns the active (merged) config. Safe for concurrent read.
func (m *ConfigManager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// Update sets a config entry: writes to DB and applies to active config in memory.
func (m *ConfigManager) Update(key, value string) error {
	// Validate: try applying to a temp copy first
	testCfg := copyConfig(m.active)
	if err := applyConfigEntry(testCfg, key, value); err != nil {
		return fmt.Errorf("invalid value for %s: %w", key, err)
	}

	// Persist to DB
	if m.store != nil {
		if err := m.store.SetConfig(key, value); err != nil {
			return fmt.Errorf("persist config: %w", err)
		}
	}

	// Apply to active config
	m.mu.Lock()
	if err := applyConfigEntry(m.active, key, value); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("apply config: %w", err)
	}
	m.mu.Unlock()

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

	// Reset to file config value
	m.mu.Lock()
	fileVal := getConfigEntry(m.base, key)
	if err := applyConfigEntry(m.active, key, fileVal); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("revert config: %w", err)
	}
	m.mu.Unlock()

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
		if dbVal, ok := dbEntries[key]; ok && strings.TrimSpace(dbVal) != "" {
			val, err := parseConfigValue(key, dbVal)
			if err != nil {
				return nil, fmt.Errorf("invalid db config %s: %w", key, err)
			}
			result[key] = val
			sources[key] = "db"
			continue
		}
		result[key] = getConfigValueTyped(base, key)
		sources[key] = "file"
	}

	result["_meta"] = map[string]interface{}{
		"sources": sources,
	}
	return result, nil
}

func (m *ConfigManager) getActiveMap() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.active
	return map[string]interface{}{
		"gitea.url":                     cfg.Gitea.URL,
		"gitea.admin_token":             cfg.Gitea.AdminToken,
		"gitea.webhook_secret":          cfg.Gitea.WebhookSecret,
		"llm.defaults.provider":         cfg.LLM.Defaults.Provider,
		"llm.defaults.model":            cfg.LLM.Defaults.Model,
		"llm.defaults.max_tokens":       cfg.LLM.Defaults.MaxTokens,
		"llm.defaults.temperature":      cfg.LLM.Defaults.Temperature,
		"llm.providers":                 cfg.LLM.Providers,
		"dispatcher.max_concurrent":     cfg.Dispatcher.MaxConcurrent,
		"dispatcher.retry_count":        cfg.Dispatcher.RetryCount,
		"dispatcher.timeout":            cfg.Dispatcher.Timeout,
		"agents.defaults.provider":      cfg.Agents.Defaults.Provider,
		"agents.defaults.model":         cfg.Agents.Defaults.Model,
		"agents.defaults.max_tokens":    cfg.Agents.Defaults.MaxTokens,
		"agents.defaults.temperature": cfg.Agents.Defaults.Temperature,
	}
}

var configKeys = []string{
	"gitea.url",
	"gitea.admin_token",
	"gitea.webhook_secret",
	"llm.defaults.provider",
	"llm.defaults.model",
	"llm.defaults.max_tokens",
	"llm.defaults.temperature",
	"llm.providers",
	"dispatcher.max_concurrent",
	"dispatcher.retry_count",
	"dispatcher.timeout",
	"agents.defaults.provider",
	"agents.defaults.model",
	"agents.defaults.max_tokens",
	"agents.defaults.temperature",
}

func parseConfigValue(key, value string) (interface{}, error) {
	switch key {
	case "llm.defaults.max_tokens", "dispatcher.max_concurrent", "dispatcher.retry_count", "dispatcher.timeout", "agents.defaults.max_tokens":
		n, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("not a number: %s", value)
		}
		return n, nil
	case "llm.defaults.temperature", "agents.defaults.temperature":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("not a float: %s", value)
		}
		return f, nil
	case "llm.providers":
		var providers map[string]ProviderConfig
		if err := json.Unmarshal([]byte(value), &providers); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return providers, nil
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
	case "llm.defaults.max_tokens":
		return cfg.LLM.Defaults.MaxTokens
	case "llm.defaults.temperature":
		return cfg.LLM.Defaults.Temperature
	case "llm.providers":
		return cfg.LLM.Providers
	case "dispatcher.max_concurrent":
		return cfg.Dispatcher.MaxConcurrent
	case "dispatcher.retry_count":
		return cfg.Dispatcher.RetryCount
	case "dispatcher.timeout":
		return cfg.Dispatcher.Timeout
	case "agents.defaults.provider":
		return cfg.Agents.Defaults.Provider
	case "agents.defaults.model":
		return cfg.Agents.Defaults.Model
	case "agents.defaults.max_tokens":
		return cfg.Agents.Defaults.MaxTokens
	case "agents.defaults.temperature":
		return cfg.Agents.Defaults.Temperature
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
	case "llm.defaults.max_tokens":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.LLM.Defaults.MaxTokens = n
	case "llm.defaults.temperature":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("not a float: %s", value)
		}
		cfg.LLM.Defaults.Temperature = f
	case "llm.providers":
		var providers map[string]ProviderConfig
		if err := json.Unmarshal([]byte(value), &providers); err != nil {
			return fmt.Errorf("invalid JSON: %s", err)
		}
		cfg.LLM.Providers = providers
	case "dispatcher.max_concurrent":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Dispatcher.MaxConcurrent = n
	case "dispatcher.retry_count":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Dispatcher.RetryCount = n
	case "dispatcher.timeout":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Dispatcher.Timeout = n
	case "agents.defaults.provider":
		cfg.Agents.Defaults.Provider = value
	case "agents.defaults.model":
		cfg.Agents.Defaults.Model = value
	case "agents.defaults.max_tokens":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a number: %s", value)
		}
		cfg.Agents.Defaults.MaxTokens = n
	case "agents.defaults.temperature":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("not a float: %s", value)
		}
		cfg.Agents.Defaults.Temperature = f
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

// IsConfigKey returns true if the key is a recognized config field (not a data key like prompt.templates).
func IsConfigKey(key string) bool {
	switch key {
	case "gitea.url", "gitea.admin_token", "gitea.webhook_secret",
		"llm.defaults.provider", "llm.defaults.model", "llm.defaults.max_tokens", "llm.defaults.temperature",
		"llm.providers",
		"dispatcher.max_concurrent", "dispatcher.retry_count", "dispatcher.timeout",
		"agents.defaults.provider", "agents.defaults.model", "agents.defaults.max_tokens", "agents.defaults.temperature":
		return true
	default:
		return false
	}
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
	case "llm.defaults.max_tokens":
		return strconv.Itoa(cfg.LLM.Defaults.MaxTokens)
	case "llm.defaults.temperature":
		return fmt.Sprintf("%g", cfg.LLM.Defaults.Temperature)
	case "llm.providers":
		data, _ := json.Marshal(cfg.LLM.Providers)
		return string(data)
	case "dispatcher.max_concurrent":
		return strconv.Itoa(cfg.Dispatcher.MaxConcurrent)
	case "dispatcher.retry_count":
		return strconv.Itoa(cfg.Dispatcher.RetryCount)
	case "dispatcher.timeout":
		return strconv.Itoa(cfg.Dispatcher.Timeout)
	case "agents.defaults.provider":
		return cfg.Agents.Defaults.Provider
	case "agents.defaults.model":
		return cfg.Agents.Defaults.Model
	case "agents.defaults.max_tokens":
		return strconv.Itoa(cfg.Agents.Defaults.MaxTokens)
	case "agents.defaults.temperature":
		return fmt.Sprintf("%g", cfg.Agents.Defaults.Temperature)
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
			dst.LLM.Providers[k] = v
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
