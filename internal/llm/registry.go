package llm

import (
	"fmt"
	"strings"

	"gitea-agent-gateway/internal/config"
)

// Registry holds configured LLM providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates a provider registry from configuration.
func NewRegistry(cfg *config.LLMConfig) *Registry {
	r := &Registry{
		providers: make(map[string]Provider),
	}

	for name, pcfg := range cfg.Providers {
		if strings.EqualFold(name, "claude") || strings.EqualFold(name, "anthropic") {
			r.providers[name] = NewAnthropicProvider(pcfg.APIKey)
		} else {
			r.providers[name] = NewOpenAICompatibleProvider(pcfg.BaseURL, pcfg.APIKey)
		}
	}

	return r
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}
	return p, nil
}

// Register adds a provider to the registry (useful for testing).
func (r *Registry) Register(name string, provider Provider) {
	if r.providers == nil {
		r.providers = make(map[string]Provider)
	}
	r.providers[name] = provider
}

// Reload rebuilds the registry from updated config.
func (r *Registry) Reload(cfg *config.LLMConfig) {
	newProviders := make(map[string]Provider)
	for name, pcfg := range cfg.Providers {
		if strings.EqualFold(name, "claude") || strings.EqualFold(name, "anthropic") {
			newProviders[name] = NewAnthropicProvider(pcfg.APIKey)
		} else {
			newProviders[name] = NewOpenAICompatibleProvider(pcfg.BaseURL, pcfg.APIKey)
		}
	}
	r.providers = newProviders
}
