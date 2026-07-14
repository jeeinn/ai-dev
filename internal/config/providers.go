package config

import (
	"encoding/json"
	"fmt"
)

// ProviderConfigFromMap normalizes provider fields from UI/DB JSON objects.
// Accepts both snake_case (base_url, api_key) and legacy PascalCase (BaseURL, APIKey).
func ProviderConfigFromMap(m map[string]interface{}) ProviderConfig {
	pc := ProviderConfig{
		BaseURL: pickString(m, "base_url", "BaseURL"),
		APIKey:  pickString(m, "api_key", "APIKey"),
		Type:    pickString(m, "type", "Type"),
	}

	// Parse default_params
	if params, ok := m["default_params"]; ok {
		if pm, ok := params.(map[string]interface{}); ok {
			pc.DefaultParams = parseModelParams(pm)
		}
	}
	if params, ok := m["DefaultParams"]; ok {
		if pm, ok := params.(map[string]interface{}); ok {
			pc.DefaultParams = parseModelParams(pm)
		}
	}

	// Parse models
	if models, ok := m["models"]; ok {
		pc.Models = parseModelDefinitions(models)
	}
	if models, ok := m["Models"]; ok {
		pc.Models = parseModelDefinitions(models)
	}

	return pc
}

func parseModelParams(m map[string]interface{}) ModelParams {
	var mp ModelParams
	if v, ok := pickFloat64(m, "temperature", "Temperature"); ok {
		mp.Temperature = &v
	}
	if v, ok := pickFloat64(m, "top_p", "TopP"); ok {
		mp.TopP = &v
	}
	if v, ok := pickInt(m, "max_output_tokens", "MaxOutputTokens"); ok {
		mp.MaxOutputTokens = &v
	}
	if v, ok := pickFloat64(m, "frequency_penalty", "FrequencyPenalty"); ok {
		mp.FrequencyPenalty = &v
	}
	if v, ok := pickFloat64(m, "presence_penalty", "PresencePenalty"); ok {
		mp.PresencePenalty = &v
	}
	return mp
}

func parseModelDefinitions(raw interface{}) []ModelDefinition {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]ModelDefinition, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		md := ModelDefinition{
			ID:            pickString(m, "id", "ID"),
			Name:          pickString(m, "name", "Name"),
			Description:   pickString(m, "description", "Description"),
			ContextWindow: pickIntDefault(m, 0, "context_window", "ContextWindow"),
			MaxOutput:     pickIntDefault(m, 0, "max_output", "MaxOutput"),
			SupportsTools: pickBoolDefault(m, false, "supports_tools", "SupportsTools"),
			IsReasoning:   pickBoolDefault(m, false, "is_reasoning", "IsReasoning"),
		}
		if v, ok := pickFloat64(m, "input_price", "InputPrice"); ok {
			md.InputPrice = v
		}
		if v, ok := pickFloat64(m, "output_price", "OutputPrice"); ok {
			md.OutputPrice = v
		}
		if params, ok := m["default_params"]; ok {
			if pm, ok := params.(map[string]interface{}); ok {
				md.DefaultParams = parseModelParams(pm)
			}
		}
		if md.ID != "" {
			result = append(result, md)
		}
	}
	return result
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

func pickFloat64(m map[string]interface{}, keys ...string) (float64, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch t := v.(type) {
			case float64:
				return t, true
			case int:
				return float64(t), true
			case int64:
				return float64(t), true
			}
		}
	}
	return 0, false
}

func pickInt(m map[string]interface{}, keys ...string) (int, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch t := v.(type) {
			case int:
				return t, true
			case int64:
				return int(t), true
			case float64:
				return int(t), true
			}
		}
	}
	return 0, false
}

func pickIntDefault(m map[string]interface{}, def int, keys ...string) int {
	if v, ok := pickInt(m, keys...); ok {
		return v
	}
	return def
}

func pickBoolDefault(m map[string]interface{}, def bool, keys ...string) bool {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch t := v.(type) {
			case bool:
				return t
			}
		}
	}
	return def
}

func floatPtr(f float64) *float64 {
	return &f
}

func intPtr(i int) *int {
	return &i
}

// BuiltinModelCatalog is the built-in model metadata for common providers.
// Users can override or extend via ProviderConfig.Models.
var BuiltinModelCatalog = map[string][]ModelDefinition{
	"deepseek": {
		{
			ID:            "deepseek-v4",
			Name:          "DeepSeek V4",
			ContextWindow: 1000000,
			MaxOutput:     32768,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "DeepSeek V4 旗舰模型，综合能力强，百万上下文",
			InputPrice:    0.015,
			OutputPrice:   0.025,
		},
		{
			ID:            "deepseek-v4-flash",
			Name:          "DeepSeek V4 Flash",
			ContextWindow: 1000000,
			MaxOutput:     32768,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "DeepSeek V4 快速版本，性价比高，百万上下文",
			InputPrice:    0.008,
			OutputPrice:   0.012,
			DefaultParams: ModelParams{Temperature: floatPtr(0.5)},
		},
		{
			ID:            "deepseek-v4-lite",
			Name:          "DeepSeek V4 Lite",
			ContextWindow: 512000,
			MaxOutput:     16384,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "DeepSeek V4 轻量版，极致性价比",
			InputPrice:    0.003,
			OutputPrice:   0.006,
		},
		{
			ID:            "deepseek-reasoner",
			Name:          "DeepSeek Reasoner",
			ContextWindow: 128000,
			MaxOutput:     8192,
			SupportsTools: false,
			IsReasoning:   true,
			Description:   "推理专用模型，支持思维链输出",
			InputPrice:    0.02,
			OutputPrice:   0.03,
		},
	},
	"openai": {
		{
			ID:            "gpt-4o",
			Name:          "GPT-4o",
			ContextWindow: 128000,
			MaxOutput:     4096,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "OpenAI 旗舰多模态模型",
			InputPrice:    0.005,
			OutputPrice:   0.015,
		},
		{
			ID:            "gpt-4o-mini",
			Name:          "GPT-4o Mini",
			ContextWindow: 128000,
			MaxOutput:     16384,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "GPT-4o 轻量版，性价比极高",
			InputPrice:    0.0015,
			OutputPrice:   0.006,
		},
		{
			ID:            "o3-mini",
			Name:          "o3 Mini",
			ContextWindow: 200000,
			MaxOutput:     8192,
			SupportsTools: true,
			IsReasoning:   true,
			Description:   "推理优化模型，支持思考模式",
			InputPrice:    0.003,
			OutputPrice:   0.012,
		},
	},
	"claude": {
		{
			ID:            "claude-sonnet-4-5",
			Name:          "Claude Sonnet 4.5",
			ContextWindow: 200000,
			MaxOutput:     4096,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "Claude 旗舰模型，长上下文能力强",
			InputPrice:    0.003,
			OutputPrice:   0.015,
		},
		{
			ID:            "claude-haiku-4-5",
			Name:          "Claude Haiku 4.5",
			ContextWindow: 200000,
			MaxOutput:     4096,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "Claude 轻量版，速度快成本低",
			InputPrice:    0.00075,
			OutputPrice:   0.003,
		},
	},
	"qwen": {
		{
			ID:            "qwen-2.5-1m",
			Name:          "Qwen 2.5-1M",
			ContextWindow: 1000000,
			MaxOutput:     32768,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "通义千问 2.5-1M，百万上下文旗舰模型",
			InputPrice:    0.002,
			OutputPrice:   0.006,
		},
		{
			ID:            "qwen-2.5-turbo",
			Name:          "Qwen 2.5 Turbo",
			ContextWindow: 128000,
			MaxOutput:     16384,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "通义千问 2.5 Turbo，高性价比",
			InputPrice:    0.001,
			OutputPrice:   0.003,
		},
		{
			ID:            "qwen-2.5-plus",
			Name:          "Qwen 2.5 Plus",
			ContextWindow: 128000,
			MaxOutput:     8192,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "通义千问 2.5 Plus，平衡性能与成本",
			InputPrice:    0.0015,
			OutputPrice:   0.0045,
		},
	},
	"sensenova": {
		{
			ID:            "nova-6.7-flash-lite",
			Name:          "SenseNova 6.7 Flash-Lite",
			ContextWindow: 128000,
			MaxOutput:     16384,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "商汤日日新 6.7 Flash-Lite，免费公测",
			InputPrice:    0,
			OutputPrice:   0,
		},
		{
			ID:            "nova-6.7",
			Name:          "SenseNova 6.7",
			ContextWindow: 128000,
			MaxOutput:     8192,
			SupportsTools: true,
			IsReasoning:   false,
			Description:   "商汤日日新 6.7 旗舰模型",
			InputPrice:    0.002,
			OutputPrice:   0.006,
		},
	},
}
