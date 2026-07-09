package llm

import (
	"fmt"
	"strings"

	"gitea-agent-gateway/internal/config"
)

// Registry holds configured LLM providers.
type Registry struct {
	providers           map[string]Provider
	rateLimitBackoffSec int
	rateLimitMaxRetries int
}

// NewRegistry creates a provider registry from configuration.
func NewRegistry(cfg *config.LLMConfig) *Registry {
	r := &Registry{
		providers: make(map[string]Provider),
	}
	r.loadProviders(cfg)
	return r
}

func (r *Registry) loadProviders(cfg *config.LLMConfig) {
	if cfg == nil {
		r.providers = make(map[string]Provider)
		return
	}
	providers := make(map[string]Provider)
	for name, pcfg := range cfg.Providers {
		if strings.EqualFold(name, "claude") || strings.EqualFold(name, "anthropic") {
			providers[name] = NewAnthropicProvider(pcfg.APIKey)
		} else {
			providers[name] = NewOpenAICompatibleProvider(pcfg.BaseURL, pcfg.APIKey)
		}
	}
	r.providers = providers
}

// SetRateLimitBackoff configures 429 retry behavior for all providers returned by Get.
func (r *Registry) SetRateLimitBackoff(backoffSec, maxRetries int) {
	if backoffSec < 0 {
		backoffSec = 0
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	r.rateLimitBackoffSec = backoffSec
	r.rateLimitMaxRetries = maxRetries
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}
	return NewRateLimitRetryProvider(p, r.rateLimitBackoffSec, r.rateLimitMaxRetries), nil
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
	r.loadProviders(cfg)
}
