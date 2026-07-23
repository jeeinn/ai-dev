package llm

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jeeinn/matea/internal/config"
)

// Registry holds configured LLM providers.
type Registry struct {
	mu                  sync.RWMutex
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
		if isAnthropicProvider(name, pcfg) {
			providers[name] = NewAnthropicProvider(pcfg.APIKey)
		} else {
			providers[name] = NewOpenAICompatibleProvider(pcfg.BaseURL, pcfg.APIKey)
		}
	}
	r.providers = providers
}

// isAnthropicProvider chooses the Anthropic adapter from ProviderConfig.Type.
// Name-based matching (claude/anthropic) is only a legacy fallback when type is empty.
func isAnthropicProvider(name string, pcfg config.ProviderConfig) bool {
	switch strings.ToLower(strings.TrimSpace(pcfg.Type)) {
	case "anthropic":
		return true
	case "openai_compatible", "openai":
		return false
	case "":
		return strings.EqualFold(name, "claude") || strings.EqualFold(name, "anthropic")
	default:
		// Unknown type: do not guess Anthropic from name — prefer OpenAI-compatible.
		return false
	}
}

// SetRateLimitBackoff configures 429 retry behavior for all providers returned by Get.
func (r *Registry) SetRateLimitBackoff(backoffSec, maxRetries int) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}
	return NewRateLimitRetryProvider(p, r.rateLimitBackoffSec, r.rateLimitMaxRetries), nil
}

// Register adds a provider to the registry (useful for testing).
func (r *Registry) Register(name string, provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers == nil {
		r.providers = make(map[string]Provider)
	}
	r.providers[name] = provider
}

// Reload rebuilds the registry from updated config.
func (r *Registry) Reload(cfg *config.LLMConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadProviders(cfg)
}

// SupportsTools reports whether the provider can handle tool/function calling.
// The built-in Anthropic adapter does not implement tools yet.
func SupportsTools(p Provider) bool {
	for p != nil {
		switch t := p.(type) {
		case *AnthropicProvider:
			return false
		case *rateLimitRetryProvider:
			p = t.inner
		default:
			return true
		}
	}
	return true
}
