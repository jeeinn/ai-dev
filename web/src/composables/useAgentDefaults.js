import { ref, computed } from 'vue'
import api from '../api'

export const DEFAULT_AGENT_MAX_TOKENS = 2048

const DEFAULT_LOOP_CONFIG = {
  max_iterations: 20,
  max_tokens: 4096,
  timeout: '5m',
  total_timeout: '30m'
}

export function normalizeProviders(raw) {
  const out = {}
  for (const [name, cfg] of Object.entries(raw || {})) {
    if (!cfg || typeof cfg !== 'object') continue
    out[name] = {
      base_url: cfg.base_url || cfg.BaseURL || '',
      api_key: cfg.api_key || cfg.APIKey || ''
    }
  }
  return out
}

function resolveDefaultProvider(data, providerKeys) {
  const agentsProvider = (data['agents.defaults.provider'] || '').trim()
  const llmProvider = (data['llm.defaults.provider'] || '').trim()

  if (agentsProvider && providerKeys.includes(agentsProvider)) return agentsProvider
  if (llmProvider && providerKeys.includes(llmProvider)) return llmProvider
  if (agentsProvider) return agentsProvider
  if (llmProvider) return llmProvider
  if (providerKeys.length > 0) return providerKeys[0]
  return 'deepseek'
}

export function useAgentDefaults() {
  const providers = ref({})
  const agentDefaults = ref({
    provider: 'deepseek',
    model: 'deepseek-chat',
    max_tokens: DEFAULT_AGENT_MAX_TOKENS,
    temperature: 0.3
  })

  const providerNames = computed(() => Object.keys(providers.value))

  const loadAgentConfig = async () => {
    try {
      const data = await api.get('/config')
      providers.value = normalizeProviders(data?.['llm.providers'])
      const keys = Object.keys(providers.value)

      agentDefaults.value = {
        provider: resolveDefaultProvider(data, keys),
        model: (data['agents.defaults.model'] || data['llm.defaults.model'] || 'deepseek-chat').trim(),
        max_tokens: Number(data['agents.defaults.max_tokens']) || DEFAULT_AGENT_MAX_TOKENS,
        temperature: data['agents.defaults.temperature'] !== undefined
          ? Number(data['agents.defaults.temperature'])
          : 0.3
      }
    } catch {
      providers.value = {}
    }
  }

  const effectiveProviderNames = (currentProvider) => {
    const names = [...providerNames.value]
    const current = (currentProvider || '').trim()
    if (current && !names.includes(current)) {
      names.unshift(current)
    }
    return names
  }

  const createEmptyAgentForm = () => ({
    name: '',
    gitea_username: '',
    role: 'analyze',
    provider: agentDefaults.value.provider,
    model: agentDefaults.value.model,
    max_tokens: agentDefaults.value.max_tokens,
    temperature: agentDefaults.value.temperature,
    system_prompt: '',
    user_template: '',
    status: 'active',
    repos: [],
    loop_config: { ...DEFAULT_LOOP_CONFIG }
  })

  return {
    providers,
    providerNames,
    agentDefaults,
    loadAgentConfig,
    effectiveProviderNames,
    createEmptyAgentForm,
    defaultLoopConfig: DEFAULT_LOOP_CONFIG
  }
}
