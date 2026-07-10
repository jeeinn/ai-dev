package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockConfigStore struct {
	data map[string]string
}

func (m *mockConfigStore) GetConfig(key string) (string, error) {
	return m.data[key], nil
}

func (m *mockConfigStore) SetConfig(key, value string) error {
	m.data[key] = value
	return nil
}

func (m *mockConfigStore) DeleteConfig(key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockConfigStore) ListConfigs() (map[string]string, error) {
	out := make(map[string]string, len(m.data))
	for k, v := range m.data {
		out[k] = v
	}
	return out, nil
}

func TestGetDisplayMapPrefersDBAndFallsBackToFile(t *testing.T) {
	fileCfg := &Config{
		Gitea: GiteaConfig{
			URL:           "http://file-gitea:3000",
			AdminToken:    "file-token",
			WebhookSecret: "file-secret",
		},
		LLM: LLMConfig{
			Defaults: LLMDefaultsConfig{
				Provider: "deepseek",
				Model:    "deepseek-chat",
			},
			Providers: map[string]ProviderConfig{
				"deepseek": {BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-file-key-12345"},
			},
		},
		Dispatcher: DispatcherConfig{MaxConcurrent: 2, TaskRetryCount: 1},
		Agents: AgentsConfig{
			Defaults: AgentDefaultsConfig{
				Provider:        "deepseek",
				Model:           "deepseek-chat",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.3,
				Timeout:         "5m",
			},
		},
	}

	m := NewConfigManager(fileCfg)
	m.SetStore(&mockConfigStore{
		data: map[string]string{
			"gitea.url": "http://db-gitea:3000",
		},
	})

	display, err := m.GetDisplayMap()
	require.NoError(t, err)

	assert.Equal(t, "http://db-gitea:3000", display["gitea.url"])
	assert.Equal(t, "file-token", display["gitea.admin_token"])
	assert.Equal(t, "deepseek-chat", display["llm.defaults.model"])

	// Providers in display should be REAL values (not masked) — for editing
	providers := display["llm.providers"].(map[string]ProviderConfig)
	assert.Equal(t, "sk-file-key-12345", providers["deepseek"].APIKey, "display providers should have real api_key for editing")
	assert.Equal(t, "https://api.deepseek.com/v1", providers["deepseek"].BaseURL)

	meta := display["_meta"].(map[string]interface{})
	sources := meta["sources"].(map[string]string)
	assert.Equal(t, "db", sources["gitea.url"])
	assert.Equal(t, "file", sources["gitea.admin_token"])
	assert.Equal(t, "file", sources["llm.providers"])
}


func TestGetDisplayMapTreatsEmptyDBValueAsUnset(t *testing.T) {
	fileCfg := &Config{
		Gitea: GiteaConfig{URL: "http://file-gitea:3000"},
		LLM: LLMConfig{
			Defaults: LLMDefaultsConfig{Provider: "deepseek", Model: "deepseek-chat"},
		},
		Dispatcher: DispatcherConfig{MaxConcurrent: 2, TaskRetryCount: 1},
		Agents: AgentsConfig{
			Defaults: AgentDefaultsConfig{
				Provider:        "deepseek",
				Model:           "deepseek-chat",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.3,
				Timeout:         "5m",
			},
		},
	}

	m := NewConfigManager(fileCfg)
	m.SetStore(&mockConfigStore{
		data: map[string]string{
			"gitea.url": "",
		},
	})

	display, err := m.GetDisplayMap()
	require.NoError(t, err)
	assert.Equal(t, "http://file-gitea:3000", display["gitea.url"])

	meta := display["_meta"].(map[string]interface{})
	sources := meta["sources"].(map[string]string)
	assert.Equal(t, "file", sources["gitea.url"])
}

func TestGetDisplayMapSemanticEmptyJSONFallsBack(t *testing.T) {
	fileCfg := &Config{
		LLM: LLMConfig{
			Defaults: LLMDefaultsConfig{Provider: "deepseek", Model: "deepseek-chat"},
			Providers: map[string]ProviderConfig{
				"deepseek": {BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-file-abcde"},
				"openai":   {BaseURL: "https://api.openai.com/v1", APIKey: "sk-open-key"},
			},
		},
		Dispatcher: DispatcherConfig{MaxConcurrent: 2, TaskRetryCount: 1},
		Agents: AgentsConfig{
			Defaults: AgentDefaultsConfig{
				Provider:        "deepseek",
				Model:           "deepseek-chat",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.3,
				Timeout:         "5m",
			},
		},
	}

	tests := []struct {
		name    string
		dbVal   string
		wantSrc string
	}{
		{"empty JSON object {}", "{}", "file"},
		{"null string", "null", "file"},
		{"whitespace-padded empty JSON", "  {}  ", "file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewConfigManager(fileCfg)
			m.SetStore(&mockConfigStore{
				data: map[string]string{
					"llm.providers": tt.dbVal,
				},
			})

			display, err := m.GetDisplayMap()
			require.NoError(t, err)

			meta := display["_meta"].(map[string]interface{})
			sources := meta["sources"].(map[string]string)
			assert.Equal(t, tt.wantSrc, sources["llm.providers"], "source for llm.providers")

			// Display should show file config providers with REAL api_key (for editing)
			providers := display["llm.providers"].(map[string]ProviderConfig)
			assert.Equal(t, 2, len(providers))
			assert.Equal(t, "sk-file-abcde", providers["deepseek"].APIKey)
			assert.Equal(t, "sk-open-key", providers["openai"].APIKey)
		})
	}
}


func TestGetDisplayMapDBProvidersMasked(t *testing.T) {
	fileCfg := &Config{
		LLM: LLMConfig{
			Defaults: LLMDefaultsConfig{Provider: "deepseek", Model: "deepseek-chat"},
			Providers: map[string]ProviderConfig{
				"deepseek": {BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-file-key"},
			},
		},
		Dispatcher: DispatcherConfig{MaxConcurrent: 2, TaskRetryCount: 1},
		Agents: AgentsConfig{
			Defaults: AgentDefaultsConfig{
				Provider:        "deepseek",
				Model:           "deepseek-chat",
				MaxOutputTokens: 4096,
				MaxInputTokens:  8192,
				Temperature:     0.3,
				Timeout:         "5m",
			},
		},
	}

	m := NewConfigManager(fileCfg)
	m.SetStore(&mockConfigStore{
		data: map[string]string{
			"llm.providers": `{"deepseek":{"base_url":"https://api.deepseek.com/v1","api_key":"sk-db-long-key-1234567890"}}`,
		},
	})

	display, err := m.GetDisplayMap()
	require.NoError(t, err)

	// Display should have REAL values (not masked) — for editing
	providers := display["llm.providers"].(map[string]ProviderConfig)
	assert.Equal(t, "sk-db-long-key-1234567890", providers["deepseek"].APIKey, "display should show real api_key for editing")
	assert.Equal(t, "https://api.deepseek.com/v1", providers["deepseek"].BaseURL)

	meta := display["_meta"].(map[string]interface{})
	sources := meta["sources"].(map[string]string)
	assert.Equal(t, "db", sources["llm.providers"])
}




func TestIsSemanticallyEmptyConfigValue(t *testing.T) {
	tests := []struct {
		key   string
		val   string
		want  bool
	}{
		{"llm.providers", "{}", true},
		{"llm.providers", "null", true},
		{"llm.providers", "  {}  ", true},
		{"llm.providers", `{"deepseek":{"base_url":"https://api.deepseek.com"}}`, false},
		{"llm.providers", "", false}, // empty string is handled by TrimSpace check, not this func
		{"gitea.url", "{}", false},   // non-JSON key — not semantically empty
		{"gitea.url", "http://example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.val, func(t *testing.T) {
			assert.Equal(t, tt.want, isSemanticallyEmptyConfigValue(tt.key, tt.val))
		})
	}
}

func TestDebugConversationLogConfigRoundTrip(t *testing.T) {
	fileCfg := &Config{
		Debug: DebugConfig{ConversationLog: DefaultConversationLogConfig()},
		LLM: LLMConfig{
			Defaults: LLMDefaultsConfig{Provider: "deepseek", Model: "deepseek-chat"},
		},
		Dispatcher: DispatcherConfig{MaxConcurrent: 2, TaskRetryCount: 1},
		Agents: AgentsConfig{
			Defaults: DefaultAgentDefaults(),
			Loop:     DefaultAgentLoopConfig(),
		},
	}

	m := NewConfigManager(fileCfg)
	m.SetStore(&mockConfigStore{data: map[string]string{}})

	require.NoError(t, m.Update("debug.conversation_log.enabled", "true"))
	require.NoError(t, m.Update("debug.conversation_log.max_content_chars", "5000"))

	display, err := m.GetDisplayMap()
	require.NoError(t, err)
	assert.Equal(t, true, display["debug.conversation_log.enabled"])
	assert.Equal(t, 5000, display["debug.conversation_log.max_content_chars"])
	assert.True(t, m.Get().Debug.ConversationLog.Enabled)
}

func TestGetProviderModelsReturnsBuiltinCatalog(t *testing.T) {
	fileCfg := &Config{
		LLM: LLMConfig{
			Providers: map[string]ProviderConfig{
				"deepseek": {BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-test"},
			},
		},
	}
	m := NewConfigManager(fileCfg)

	models, source, err := m.GetProviderModels("deepseek")
	require.NoError(t, err)
	assert.Equal(t, "builtin", source)
	require.GreaterOrEqual(t, len(models), 1, "deepseek should have builtin models")

	// Verify known model exists
	found := false
	for _, mm := range models {
		if mm.ID == "deepseek-v4" {
			found = true
			assert.Equal(t, "DeepSeek V4", mm.Name)
			assert.Equal(t, 128000, mm.ContextWindow)
			assert.True(t, mm.SupportsTools)
			assert.False(t, mm.IsReasoning)
		}
	}
	assert.True(t, found, "deepseek-v4 should be in builtin catalog")
}

func TestGetProviderModelsReturnsCustomModels(t *testing.T) {
	customModels := []ModelDefinition{
		{ID: "custom-model", Name: "Custom Model", ContextWindow: 32000, SupportsTools: true},
	}
	fileCfg := &Config{
		LLM: LLMConfig{
			Providers: map[string]ProviderConfig{
				"myprovider": {
					BaseURL: "https://example.com",
					APIKey:  "sk-test",
					Models:  customModels,
				},
			},
		},
	}
	m := NewConfigManager(fileCfg)

	models, source, err := m.GetProviderModels("myprovider")
	require.NoError(t, err)
	assert.Equal(t, "custom", source)
	require.Len(t, models, 1)
	assert.Equal(t, "custom-model", models[0].ID)
	assert.Equal(t, "Custom Model", models[0].Name)
}

func TestGetProviderModelsReturnsEmptyForUnknownProvider(t *testing.T) {
	fileCfg := &Config{
		LLM: LLMConfig{
			Providers: map[string]ProviderConfig{},
		},
	}
	m := NewConfigManager(fileCfg)

	_, _, err := m.GetProviderModels("unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetDisplayMapIncludesModelsMeta(t *testing.T) {
	fileCfg := &Config{
		LLM: LLMConfig{
			Providers: map[string]ProviderConfig{
				"deepseek": {BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-test"},
				"openai":   {BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"},
			},
		},
		Agents: AgentsConfig{
			Defaults: DefaultAgentDefaults(),
			Loop:     DefaultAgentLoopConfig(),
		},
	}
	m := NewConfigManager(fileCfg)

	display, err := m.GetDisplayMap()
	require.NoError(t, err)

	meta := display["_meta"].(map[string]interface{})
	modelsMeta := meta["models"].(map[string][]ModelDefinition)
	require.NotNil(t, modelsMeta)
	assert.GreaterOrEqual(t, len(modelsMeta["deepseek"]), 1, "deepseek models should be present")
	assert.GreaterOrEqual(t, len(modelsMeta["openai"]), 1, "openai models should be present")
}
