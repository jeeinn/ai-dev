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

	// Providers fallback should be masked
	providers := display["llm.providers"].(map[string]ProviderConfig)
	assert.Equal(t, "sk-f***", providers["deepseek"].APIKey)
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

			// Should show file config providers (masked)
			providers := display["llm.providers"].(map[string]ProviderConfig)
			assert.Equal(t, 2, len(providers))
			assert.Equal(t, "sk-f***", providers["deepseek"].APIKey)
			assert.Equal(t, "sk-o***", providers["openai"].APIKey)
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

	providers := display["llm.providers"].(map[string]ProviderConfig)
	assert.Equal(t, "sk-d***", providers["deepseek"].APIKey, "DB api_key should be masked")
	assert.Equal(t, "https://api.deepseek.com/v1", providers["deepseek"].BaseURL)

	meta := display["_meta"].(map[string]interface{})
	sources := meta["sources"].(map[string]string)
	assert.Equal(t, "db", sources["llm.providers"])
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "***"},
		{"abc", "***"},
		{"sk-", "***"},
		{"sk-x", "***"},
		{"sk-abcd***", "sk-a***"},
		{"sk-long-api-key-12345", "sk-l***"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, maskAPIKey(tt.input))
		})
	}
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
