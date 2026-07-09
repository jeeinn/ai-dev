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
				"deepseek": {BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-file"},
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

	meta := display["_meta"].(map[string]interface{})
	sources := meta["sources"].(map[string]string)
	assert.Equal(t, "db", sources["gitea.url"])
	assert.Equal(t, "file", sources["gitea.admin_token"])
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
