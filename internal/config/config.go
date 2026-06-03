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
	if cfg.Dispatcher.RetryCount == 0 {
		cfg.Dispatcher.RetryCount = 1
	}
	if cfg.Dispatcher.Timeout == 0 {
		cfg.Dispatcher.Timeout = 300
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
		cfg.LLM.Defaults.Model = "deepseek-chat"
	}
	if cfg.LLM.Defaults.MaxTokens == 0 {
		cfg.LLM.Defaults.MaxTokens = 4096
	}
	if cfg.LLM.Defaults.Temperature == 0 {
		cfg.LLM.Defaults.Temperature = 0.3
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
}
