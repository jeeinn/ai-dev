package config

// copyConfig deep-copies a Config struct.
func copyConfig(src *Config) *Config {
	dst := *src
	// Deep copy map fields
	if src.LLM.Providers != nil {
		dst.LLM.Providers = make(map[string]ProviderConfig, len(src.LLM.Providers))
		for k, v := range src.LLM.Providers {
			dst.LLM.Providers[k] = deepCopyProviderConfig(v)
		}
	}
	if src.Agents.Templates != nil {
		dst.Agents.Templates = make(map[string]AgentTemplateConfig, len(src.Agents.Templates))
		for k, v := range src.Agents.Templates {
			dst.Agents.Templates[k] = v
		}
	}
	return &dst
}

func deepCopyProviderConfig(src ProviderConfig) ProviderConfig {
	dst := src
	if len(src.Models) > 0 {
		dst.Models = make([]ModelDefinition, len(src.Models))
		copy(dst.Models, src.Models)
	}
	return dst
}
