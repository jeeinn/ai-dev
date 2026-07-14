package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads the configuration from the given YAML file path.
// Environment variables in the form ${VAR_NAME} are expanded.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Expand environment variables
	expanded := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	applyDefaults(&cfg)
	return &cfg, nil
}

// expandEnvVars replaces ${VAR} or ${VAR:-default} patterns with env values.
func expandEnvVars(s string) string {
	return os.Expand(s, func(key string) string {
		if idx := strings.Index(key, ":-"); idx >= 0 {
			envKey := key[:idx]
			defaultVal := key[idx+2:]
			if val := os.Getenv(envKey); val != "" {
				return val
			}
			return defaultVal
		}
		return os.Getenv(key)
	})
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Workspace.BaseDir == "" {
		cfg.Workspace.BaseDir = "/tmp/gateway-work"
	}
	if cfg.Workspace.CleanupAfter == "" {
		cfg.Workspace.CleanupAfter = "24h"
	}
	if cfg.Dispatcher.MaxConcurrent == 0 {
		cfg.Dispatcher.MaxConcurrent = 3
	}
	// Migrate deprecated dispatcher.retry_count → task_retry_count
	if cfg.Dispatcher.TaskRetryCount == 0 && cfg.Dispatcher.RetryCount > 0 {
		cfg.Dispatcher.TaskRetryCount = cfg.Dispatcher.RetryCount
	}
	if cfg.Dispatcher.TaskRetryCount == 0 {
		cfg.Dispatcher.TaskRetryCount = 1
	}
	cfg.Dispatcher.RetryCount = 0 // clear deprecated field after migration
	if cfg.LLM.RateLimitRetries == 0 {
		cfg.LLM.RateLimitRetries = 1
	}
	if cfg.Dispatcher.QueueSize == 0 {
		cfg.Dispatcher.QueueSize = 100
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "./data/gateway.db"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.LLM.Defaults.Provider == "" {
		cfg.LLM.Defaults.Provider = "deepseek"
	}
	if cfg.LLM.Defaults.Model == "" {
		cfg.LLM.Defaults.Model = "deepseek-v4-flash"
	}
	defs := DefaultAgentDefaults()
	if cfg.Agents.Defaults.Provider == "" {
		cfg.Agents.Defaults.Provider = defs.Provider
	}
	if cfg.Agents.Defaults.Model == "" {
		cfg.Agents.Defaults.Model = defs.Model
	}
	if cfg.Agents.Defaults.MaxOutputTokens == 0 {
		cfg.Agents.Defaults.MaxOutputTokens = defs.MaxOutputTokens
	}
	if cfg.Agents.Defaults.MaxInputTokens == 0 {
		cfg.Agents.Defaults.MaxInputTokens = defs.MaxInputTokens
	}
	if cfg.Agents.Defaults.Temperature == 0 {
		cfg.Agents.Defaults.Temperature = defs.Temperature
	}
	if cfg.Agents.Defaults.Timeout == "" {
		cfg.Agents.Defaults.Timeout = defs.Timeout
	}
	if cfg.Agents.Loop.MaxIterations <= 0 {
		cfg.Agents.Loop.MaxIterations = DefaultAgentLoopConfig().MaxIterations
	}
	if cfg.Agents.Loop.TotalTimeout == "" {
		cfg.Agents.Loop.TotalTimeout = DefaultAgentLoopConfig().TotalTimeout
	}
	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = "change-this-in-production"
	}
	if cfg.Auth.JWTExpiration == "" {
		cfg.Auth.JWTExpiration = "24h"
	}
	if cfg.Auth.DefaultAdminPassword == "" {
		cfg.Auth.DefaultAdminPassword = "admin123"
	}
	if cfg.Debug.ConversationLog.MaxContentChars == 0 && !cfg.Debug.ConversationLog.Enabled {
		cfg.Debug.ConversationLog = DefaultConversationLogConfig()
	} else if cfg.Debug.ConversationLog.MaxContentChars == 0 {
		cfg.Debug.ConversationLog.MaxContentChars = DefaultConversationLogConfig().MaxContentChars
	}
	applyBackendDefaults(&cfg.Agents.Backends)
}

// applyBackendDefaults ensures the implicit `internal` builtin backend exists and
// is the default when none is set. Non-write tasks always use internal regardless.
func applyBackendDefaults(backends *AgentBackendsConfig) {
	if backends.Default == "" {
		backends.Default = "internal"
	}
	if backends.Backends == nil {
		backends.Backends = map[string]BackendConfig{}
	}
	if _, ok := backends.Backends["internal"]; !ok {
		backends.Backends["internal"] = BackendConfig{Type: BackendTypeBuiltin}
	} else if backends.Backends["internal"].Type == "" {
		b := backends.Backends["internal"]
		b.Type = BackendTypeBuiltin
		backends.Backends["internal"] = b
	}
}
