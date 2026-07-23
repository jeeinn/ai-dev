package llm

import (
	"sync"
	"testing"

	"gitea-agent-gateway/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAnthropicProviderTypeWinsOverName(t *testing.T) {
	assert.False(t, isAnthropicProvider("claude", config.ProviderConfig{Type: "openai_compatible"}))
	assert.False(t, isAnthropicProvider("my-claude-proxy", config.ProviderConfig{Type: "openai_compatible"}))
	assert.True(t, isAnthropicProvider("my-claude-proxy", config.ProviderConfig{Type: "anthropic"}))
	assert.True(t, isAnthropicProvider("claude", config.ProviderConfig{})) // legacy name fallback
	assert.True(t, isAnthropicProvider("anthropic", config.ProviderConfig{}))
	assert.False(t, isAnthropicProvider("deepseek", config.ProviderConfig{}))
	assert.False(t, isAnthropicProvider("claude", config.ProviderConfig{Type: "unknown"}))
}

func TestRegistryLoadUsesTypeField(t *testing.T) {
	r := NewRegistry(&config.LLMConfig{
		Providers: map[string]config.ProviderConfig{
			"my-claude-proxy": {Type: "openai_compatible", BaseURL: "http://localhost", APIKey: "k"},
			"corp-claude":     {Type: "anthropic", APIKey: "k"},
		},
	})

	p, err := r.Get("my-claude-proxy")
	require.NoError(t, err)
	assert.True(t, SupportsTools(p), "openai_compatible named like claude must support tools")

	p, err = r.Get("corp-claude")
	require.NoError(t, err)
	assert.False(t, SupportsTools(p))
}

func TestRegistryGetReloadConcurrent(t *testing.T) {
	r := NewRegistry(&config.LLMConfig{
		Providers: map[string]config.ProviderConfig{
			"openai": {Type: "openai_compatible", BaseURL: "http://localhost", APIKey: "k"},
		},
	})
	r.SetRateLimitBackoff(0, 0)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = r.Get("openai")
				r.Reload(&config.LLMConfig{
					Providers: map[string]config.ProviderConfig{
						"openai": {Type: "openai_compatible", BaseURL: "http://localhost", APIKey: "k2"},
					},
				})
			}
		}()
	}
	wg.Wait()
}
