package config

import (
	"encoding/json"
	"fmt"
)

// ProviderConfigFromMap normalizes provider fields from UI/DB JSON objects.
// Accepts both snake_case (base_url, api_key) and legacy PascalCase (BaseURL, APIKey).
func ProviderConfigFromMap(m map[string]interface{}) ProviderConfig {
	return ProviderConfig{
		BaseURL: pickString(m, "base_url", "BaseURL"),
		APIKey:  pickString(m, "api_key", "APIKey"),
	}
}

// ParseProvidersJSON parses llm.providers from a JSON string.
func ParseProvidersJSON(raw string) (map[string]ProviderConfig, error) {
	var items map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return ParseProvidersFromMaps(items), nil
}

// ParseProvidersFromMaps converts raw provider maps to normalized configs.
func ParseProvidersFromMaps(items map[string]map[string]interface{}) map[string]ProviderConfig {
	out := make(map[string]ProviderConfig, len(items))
	for name, fields := range items {
		if fields == nil {
			continue
		}
		out[name] = ProviderConfigFromMap(fields)
	}
	return out
}

// ParseProvidersFromInterface parses llm.providers from API payload shapes.
func ParseProvidersFromInterface(raw interface{}) (map[string]ProviderConfig, error) {
	switch v := raw.(type) {
	case map[string]ProviderConfig:
		return v, nil
	case map[string]interface{}:
		items := make(map[string]map[string]interface{}, len(v))
		for name, item := range v {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			items[name] = m
		}
		return ParseProvidersFromMaps(items), nil
	case string:
		return ParseProvidersJSON(v)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal providers: %w", err)
		}
		return ParseProvidersJSON(string(data))
	}
}

func pickString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch t := v.(type) {
			case string:
				if t != "" {
					return t
				}
			}
		}
	}
	return ""
}
