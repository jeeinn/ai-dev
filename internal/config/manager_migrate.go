package config

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

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
