# LLM Provider 模型选择与 Token 配置扩展方案

> 日期：2026-07-10
> 状态：已实施（Phase 1/2 核心完成；Phase 3 部分完成，见文末进度）
> 版本：v1.1
> 更新：2026-07-14 查漏补缺修复

---

## 一、背景与目标

### 1.1 当前问题

| 问题 | 现状 | 影响 |
|------|------|------|
| 模型手动输入 | Agent 页面模型字段是纯文本输入 | 用户需记住模型名，容易拼错 |
| Token 配置与模型脱节 | `max_input_tokens` / `max_output_tokens` 全局固定 | 不区分模型上下文窗口，可能超限或浪费 |
| Token 估算粗糙 | `len(s) / 4` 统一估算 | 中英文混排误差 30%-50% |
| 无模型能力感知 | 不知道模型是否支持工具调用 | 推理模型配置 tool_calls 导致 API 报错 |

### 1.2 目标

1. **模型选择可视化**：Provider + Model 级联下拉，支持自动发现和手动输入
2. **Token 配置自适应**：根据模型上下文窗口动态调整 token 限制
3. **估算精度提升**：区分中英文 token 估算，后续支持精确 tokenizer
4. **向后兼容**：不破坏现有配置和 API 契约

---

## 二、数据结构设计

### 2.1 ProviderConfig 扩展

```go
type ProviderConfig struct {
    BaseURL       string                  `yaml:"base_url" json:"base_url"`
    APIKey        string                  `yaml:"api_key" json:"api_key"`
    Type          string                  `yaml:"type" json:"type"` // openai_compatible | anthropic
    DefaultParams ModelParams             `yaml:"default_params" json:"default_params"`
    Models        []ModelDefinition       `yaml:"models" json:"models"`
    // models 取值规则：
    //   - 未设置（nil）：使用内置模型目录
    //   - 空数组（[]）：启用动态发现，调用 /models API 获取
    //   - 非空数组：使用用户自定义列表
}
```

### 2.2 ModelDefinition 模型定义

```go
type ModelDefinition struct {
    ID             string      `yaml:"id" json:"id"`                         // 模型 ID，如 "deepseek-v4-flash"
    Name           string      `yaml:"name" json:"name"`                     // 显示名，如 "DeepSeek V4 Flash"
    ContextWindow  int         `yaml:"context_window" json:"context_window"` // 上下文窗口大小（tokens）
    MaxOutput      int         `yaml:"max_output" json:"max_output"`         // 最大输出 tokens
    SupportsTools  bool        `yaml:"supports_tools" json:"supports_tools"` // 是否支持工具调用
    IsReasoning    bool        `yaml:"is_reasoning" json:"is_reasoning"`     // 是否推理模型
    DefaultParams  ModelParams `yaml:"default_params" json:"default_params"` // 模型级默认参数（覆盖 Provider 级）
    Description    string      `yaml:"description" json:"description"`       // 可选：模型描述
    InputPrice     float64     `yaml:"input_price" json:"input_price"`       // 可选：输入价格 $/1K tokens
    OutputPrice    float64     `yaml:"output_price" json:"output_price"`     // 可选：输出价格 $/1K tokens
}
```

### 2.3 ModelParams 参数

```go
type ModelParams struct {
    Temperature      *float64 `yaml:"temperature" json:"temperature,omitempty"`
    TopP             *float64 `yaml:"top_p" json:"top_p,omitempty"`
    MaxOutputTokens  *int     `yaml:"max_output_tokens" json:"max_output_tokens,omitempty"`
    FrequencyPenalty *float64 `yaml:"frequency_penalty" json:"frequency_penalty,omitempty"`
    PresencePenalty  *float64 `yaml:"presence_penalty" json:"presence_penalty,omitempty"`
}
```

### 2.4 配置示例（config.yaml）

```yaml
llm:
  providers:
    deepseek:
      base_url: "https://api.deepseek.com/v1"
      api_key: "${DEEPSEEK_API_KEY}"
      type: "openai_compatible"
      default_params:
        temperature: 0.7
      # 不设置 models → 使用内置目录

    openai:
      base_url: "https://api.openai.com/v1"
      api_key: "${OPENAI_API_KEY}"
      models: []  # 空数组 → 启用动态发现，调用 /models API

    claude:
      api_key: "${ANTHROPIC_API_KEY}"
      type: "anthropic"
      # 不设置 models → 使用内置目录

    ollama:
      base_url: "http://localhost:11434/v1"
      api_key: "ollama"
      models: []  # 空数组 → 调用 /api/tags 动态发现本地模型

  defaults:
    provider: "deepseek"
    model: "deepseek-v4-flash"

agents:
  defaults:
    provider: "deepseek"
    model: "deepseek-v4-flash"
    max_output_tokens: 2048
    max_input_tokens: 65536
    temperature: 0.3
    timeout: "5m"
```

---

## 三、内置模型目录

### 3.1 内置模型列表

```go
var builtinModelCatalog = map[string][]ModelDefinition{
    "deepseek": {
        {
            ID:            "deepseek-v4",
            Name:          "DeepSeek V4",
            ContextWindow: 128000,
            MaxOutput:     16384,
            SupportsTools: true,
            IsReasoning:   false,
            Description:   "DeepSeek V4 旗舰模型，综合能力强",
            InputPrice:    0.015,
            OutputPrice:   0.025,
        },
        {
            ID:            "deepseek-v4-flash",
            Name:          "DeepSeek V4 Flash",
            ContextWindow: 128000,
            MaxOutput:     16384,
            SupportsTools: true,
            IsReasoning:   false,
            Description:   "DeepSeek V4 快速版本，性价比高",
            InputPrice:    0.008,
            OutputPrice:   0.012,
            DefaultParams: ModelParams{Temperature: floatPtr(0.5)},
        },
        {
            ID:            "deepseek-reasoner",
            Name:          "DeepSeek Reasoner",
            ContextWindow: 64000,
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
}
```

---

## 四、动态模型发现机制

### 4.1 设计原则

- **优先自动发现**：选中 Provider 后，优先调用 API 获取模型列表
- **失败降级**：API 调用失败时，降级为自由输入或使用内置目录
- **缓存机制**：发现结果缓存，避免频繁调用
- **通用格式**：OpenAI 兼容 Provider 统一使用 `/v1/models` 接口

### 4.2 后端 API 设计

#### 接口：获取 Provider 可用模型

```
GET /api/config/providers/:name/models
```

**响应格式（成功）**：
```json
{
  "success": true,
  "source": "api",  // api | builtin | custom
  "models": [
    {
      "id": "deepseek-v4",
      "name": "DeepSeek V4",
      "context_window": 128000,
      "max_output": 16384,
      "supports_tools": true,
      "is_reasoning": false
    }
  ]
}
```

**响应格式（失败降级）**：
```json
{
  "success": false,
  "error": "API call failed: connection refused",
  "fallback_source": "builtin",  // builtin | manual_input
  "models": [...] // 内置目录或空数组（表示需手动输入）
}
```

### 4.3 发现策略

| Provider 类型 | 发现方式 | 接口路径 | 说明 |
|--------------|---------|----------|------|
| `openai_compatible` | GET /v1/models | `{base_url}/models` | OpenAI 标准格式 |
| `anthropic` | 内置目录 | - | 无公开 /models API，使用内置目录 |
| `ollama` | GET /api/tags | `{base_url}/../api/tags` | Ollama 本地模型 |
| 其他自定义 | 内置目录 | - | 按名称匹配内置目录 |

### 4.4 OpenAI 兼容 /v1/models 解析

请求：
```
GET {base_url}/models
Authorization: Bearer {api_key}
```

响应解析（标准 OpenAI 格式）：
```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-4o",
      "object": "model",
      "created": 1234567890,
      "owned_by": "openai"
    }
  ]
}
```

转换为 `ModelDefinition`：
```go
func parseOpenAIModels(data []OpenAIModel) []ModelDefinition {
    result := make([]ModelDefinition, 0, len(data))
    for _, m := range data {
        result = append(result, ModelDefinition{
            ID:   m.ID,
            Name: m.ID, // API 不返回显示名，使用 ID 作为显示名
            // 上下文窗口等信息从内置目录补充（如存在）
            // 未找到则留空，由用户手动配置
        })
    }
    return result
}
```

**增强：与内置目录合并**：
- API 返回的模型 ID 如果在内置目录中存在 → 使用内置目录的元数据（上下文窗口、价格等）
- API 返回的模型 ID 如果不在内置目录 → 仅使用 ID，其他字段为空
- 内置目录中存在但 API 未返回的模型 → 不显示（以 API 为准，确保可用性）

### 4.5 Ollama /api/tags 解析

请求：
```
GET {base_url}/../api/tags
```

响应解析：
```json
{
  "models": [
    {
      "name": "qwen2.5:7b",
      "model": "qwen2.5:7b",
      "modified_at": "2024-01-01T00:00:00Z",
      "size": 1234567890,
      "digest": "sha256:..."
    }
  ]
}
```

### 4.6 缓存策略

```go
type ModelDiscoveryCache struct {
    provider  string
    models    []ModelDefinition
    source    string // api | builtin | custom
    expiresAt time.Time
}

// 缓存有效期：1 小时
const modelDiscoveryCacheTTL = 1 * time.Hour

func (m *ConfigManager) GetProviderModels(providerName string) ([]ModelDefinition, string, error) {
    // 1. 检查缓存
    if cache, ok := m.modelCache[providerName]; ok && time.Now().Before(cache.expiresAt) {
        return cache.models, cache.source, nil
    }

    // 2. 获取 Provider 配置
    providerCfg, ok := m.config.LLM.Providers[providerName]
    if !ok {
        return nil, "", fmt.Errorf("provider not found: %s", providerName)
    }

    var models []ModelDefinition
    var source string

    // 3. 判断 models 字段状态
    switch {
    case providerCfg.Models == nil:
        // 未设置 → 使用内置目录
        models = builtinModelCatalog[providerName]
        source = "builtin"

    case len(providerCfg.Models) == 0:
        // 空数组 → 尝试动态发现
        var err error
        models, err = discoverModels(providerCfg)
        if err != nil {
            // 发现失败 → 降级到内置目录
            models = builtinModelCatalog[providerName]
            source = "builtin"
            // 返回错误信息供前端展示
            return models, source, fmt.Errorf("dynamic discovery failed: %w", err)
        }
        // 发现成功 → 与内置目录合并元数据
        models = mergeWithBuiltin(providerName, models)
        source = "api"

    default:
        // 非空数组 → 使用用户自定义
        models = providerCfg.Models
        source = "custom"
    }

    // 4. 写入缓存
    m.modelCache[providerName] = ModelDiscoveryCache{
        provider:  providerName,
        models:    models,
        source:    source,
        expiresAt: time.Now().Add(modelDiscoveryCacheTTL),
    }

    return models, source, nil
}
```

---

## 五、前端交互方案

### 5.1 Agent 页面模型选择

**当前**：Provider 下拉 + Model 文本输入

**改造后**：Provider 下拉 + Model 智能选择器

```
Provider:  [deepseek    ▼]  :  [DeepSeek V4 Flash (128K)  ▼]
                                 ┌─────────────────────────────┐
                                 │ 🔍 搜索模型...               │
                                 ├─────────────────────────────┤
                                 │ ✅ 已从 API 加载 12 个模型   │
                                 ├─────────────────────────────┤
                                 │ DeepSeek V4 (128K)    🔧    │
                                 │ DeepSeek V4 Flash (128K) 🔧⚡│
                                 │ DeepSeek Reasoner (64K) 🧠  │
                                 │ ...                         │
                                 ├─────────────────────────────┤
                                 │ [+ 手动输入模型 ID]          │
                                 └─────────────────────────────┘
```

**交互流程**：

```
选择 Provider
    ↓
调用 /api/config/providers/:name/models
    ├─ 成功（source=api）
    │   → 显示下拉选择 + 搜索
    │   → 选项显示：模型名 + 上下文大小 + 标签(🔧/🧠/⚡)
    │
    ├─ 失败降级（source=builtin）
    │   → 显示内置目录下拉
    │   → 顶部提示："API 获取失败，使用内置模型目录"
    │
    └─ 无可用模型
        → 显示文本输入框
        → 提示："请输入模型 ID"
```

**高级配置联动**：

选择模型后，自动填充/提示默认参数：

```
┌─ 高级配置 ───────────────────────────────────────────────┐
│                                                          │
│  最大输出 Tokens： [ 2048 ]                              │
│      提示：模型默认 16384，上限 16384                     │
│                                                          │
│  最大输入 Tokens： [ 65536 ]                             │
│      提示：模型默认 128000，上限 115200 (90% of 128K)    │
│                                                          │
│  Temperature： [ 0.3 ]                                   │
│      提示：模型默认 0.5                                   │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

### 5.2 SystemConfig 页面 Provider 配置

**第一阶段**：保留 textarea 编辑 JSON，增加辅助功能

```
┌─ Provider 配置 ────────────────────────────────────────┐
│  [textarea - JSON 格式]                                  │
│                                                         │
│  ┌─ 辅助工具 ────────────────────────────────────────┐  │
│  │  Provider:  [deepseek ▼]  [🔄 获取模型列表]       │  │
│  │                                                    │  │
│  │  可用模型（从 API 获取）：                          │  │
│  │  ☑ deepseek-v4                                     │  │
│  │  ☑ deepseek-v4-flash                               │  │
│  │  ☐ deepseek-reasoner                               │  │
│  │  ...                                               │  │
│  │  [插入选中模型到配置]                               │  │
│  └────────────────────────────────────────────────────┘  │
│                                                         │
│  密钥参考（脱敏）：...                                   │
└─────────────────────────────────────────────────────────┘
```

**第二阶段**：结构化可视化编辑（见原方案）

---

## 六、Token 估算优化

### 6.1 第一阶段：中英文区分估算

```go
import "regexp"

var cjkRegex = regexp.MustCompile(`[\p{Han}\p{Katakana}\p{Hiragana}\p{Hangul}]+`)

func estimateTokens(s string) int {
    matches := cjkRegex.FindAllString(s, -1)
    cjkLen := 0
    for _, m := range matches {
        cjkLen += len(m)
    }
    otherLen := len(s) - cjkLen
    
    // 中日韩文约 2 字符/token，英文约 4 字符/token
    return (cjkLen / 2) + (otherLen / 4)
}
```

### 6.2 第二阶段：精确 Tokenizer（可选）

引入 `github.com/pkoukk/tiktoken-go`，根据模型选择对应 tokenizer：

```go
func estimateTokensForModel(s, model string) int {
    tk, err := tiktoken.EncodingForModel(model)
    if err != nil {
        return estimateTokens(s) // fallback
    }
    return len(tk.Encode(s, nil, nil))
}
```

---

## 七、上下文窗口自适应

### 7.1 计算逻辑

```go
// resolveMaxInputTokens 计算有效的输入 token 上限
// 优先级：Agent 配置 > 模型 context_window(90%) > 默认值
func resolveMaxInputTokens(agentMaxInput int, modelMeta *ModelDefinition) int {
    // 1. 如果 Agent 有显式配置，使用该值（但不超过模型上限）
    if agentMaxInput > 0 {
        if modelMeta != nil && modelMeta.ContextWindow > 0 {
            modelLimit := int(float64(modelMeta.ContextWindow) * 0.9) // 预留 10% 给输出
            if agentMaxInput > modelLimit {
                return modelLimit
            }
        }
        return agentMaxInput
    }
    
    // 2. 使用模型上下文窗口的 90%
    if modelMeta != nil && modelMeta.ContextWindow > 0 {
        return int(float64(modelMeta.ContextWindow) * 0.9)
    }
    
    // 3. 最终 fallback
    return 65536
}

// resolveMaxOutputTokens 计算有效的输出 token 上限
// 优先级：Agent 配置 > 模型 max_output > 默认值
func resolveMaxOutputTokens(agentMaxOutput int, modelMeta *ModelDefinition) int {
    if agentMaxOutput > 0 {
        if modelMeta != nil && modelMeta.MaxOutput > 0 {
            if agentMaxOutput > modelMeta.MaxOutput {
                return modelMeta.MaxOutput
            }
        }
        return agentMaxOutput
    }
    
    if modelMeta != nil && modelMeta.MaxOutput > 0 {
        return modelMeta.MaxOutput
    }
    
    return 2048
}
```

### 7.2 模型感知的截断优化

```go
func TruncateMessages(messages []llm.Message, tools []llm.Tool,
    maxInputTokens int, modelMeta *ModelDefinition) ([]llm.Message, error) {
    
    // 如果模型不支持工具调用，移除 tool_calls 和 tools
    if modelMeta != nil && !modelMeta.SupportsTools {
        for i := range messages {
            messages[i].ToolCalls = nil
        }
        tools = nil
    }
    
    // ... 原有截断逻辑 ...
}
```

---

## 八、API 变更

### 8.1 GetDisplayMap 扩展

在 `_meta` 中新增 `models` 字段：

```json
{
  "_meta": {
    "sources": { ... },
    "masked": { ... },
    "models": {
      "deepseek": [
        {"id": "deepseek-v4", "name": "DeepSeek V4", "context_window": 128000, ...},
        {"id": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", ...}
      ],
      "openai": [...]
    }
  }
}
```

### 8.2 新增接口：获取 Provider 模型列表

```
GET /api/config/providers/:name/models
```

用于前端动态获取模型列表（支持动态发现的 Provider 会调用 API）。

---

## 九、实施路线图

### Phase 1：模型目录 + 下拉选择（最小可用）

| # | 任务 | 优先级 | 涉及文件 | 说明 |
|---|------|--------|----------|------|
| 1.1 | 新增 `ModelDefinition` / `ModelParams` 结构体 | P0 | `internal/config/schema.go` | 数据结构定义 |
| 1.2 | 新增内置模型目录 | P0 | `internal/config/providers.go` | 常用模型元数据 |
| 1.3 | `ProviderConfig` 新增 `Models` / `Type` 字段 | P0 | `internal/config/schema.go` | 向后兼容 |
| 1.4 | 实现 `GetProviderModels` 方法 | P0 | `internal/config/manager.go` | 内置目录 + 缓存 |
| 1.5 | `GetDisplayMap` 返回 `_meta.models` | P0 | `internal/config/manager.go` | 供前端使用 |
| 1.6 | 新增 `/api/config/providers/:name/models` 接口 | P0 | `internal/api/config.go` | 第一阶段仅返回内置目录 |
| 1.7 | 前端 Agent 页面：模型级联下拉 | P0 | `web/src/views/Agents.vue` | Provider + Model 联动 |
| 1.8 | 前端 Agent 页面：高级配置显示模型上限提示 | P1 | `web/src/views/Agents.vue` | 显示默认值和上限 |
| 1.9 | Token 估算：中英文区分 | P1 | `internal/agent/context.go` | 提升估算精度 |
| 1.10 | 单元测试覆盖 | P0 | `internal/config/*_test.go` | 新增功能测试 |

### Phase 2：动态模型发现 + 可视化编辑

| # | 任务 | 优先级 | 涉及文件 | 说明 |
|---|------|--------|----------|------|
| 2.1 | 实现动态模型发现（OpenAI /models） | P0 | `internal/config/manager.go` | 调用 API 获取模型列表 |
| 2.2 | 实现 Ollama 动态发现（/api/tags） | P1 | `internal/config/manager.go` | 本地模型发现 |
| 2.3 | 模型元数据合并（API + 内置目录） | P0 | `internal/config/manager.go` | 补充上下文窗口等信息 |
| 2.4 | 前端：模型选择器加载态和失败降级 | P0 | `web/src/views/Agents.vue` | 加载中/失败/手动输入 |
| 2.5 | 上下文窗口自适应逻辑 | P0 | `internal/agents/runners.go` | 动态调整 token 限制 |
| 2.6 | SystemConfig：Provider 可视化编辑 | P1 | `web/src/views/SystemConfig.vue` | 结构化表单 |
| 2.7 | SystemConfig：模型编辑弹窗 | P1 | `web/src/views/SystemConfig.vue` | 增删改模型元数据 |
| 2.8 | 模型感知的截断优化 | P2 | `internal/agent/truncate.go` | 工具调用支持检查 |

### Phase 3：成本统计 + 智能优化

| # | 任务 | 优先级 | 涉及文件 | 说明 |
|---|------|--------|----------|------|
| 3.1 | Token 使用统计记录 | P1 | `internal/store/task.go` | 记录每次调用 usage |
| 3.2 | 任务成本估算 | P1 | `internal/agents/manager.go` | 根据价格计算成本 |
| 3.3 | 前端任务详情页显示 token 统计 | P1 | `web/src/views/Tasks.vue` | 展示用量和成本 |
| 3.4 | 精确 tokenizer（可选） | P2 | `internal/agent/context.go` | 引入 tiktoken |
| 3.5 | 语义摘要功能（可选） | P2 | `internal/agent/summarize.go` | 对旧对话摘要 |
| 3.6 | 成本预算控制（可选） | P2 | `internal/agents/manager.go` | 单任务预算限制 |

---

## 十、向后兼容性

| 变更 | 兼容方式 | 说明 |
|------|----------|------|
| `ProviderConfig` 新增字段 | 所有新增字段均为可选（零值可用） | 旧配置正常工作 |
| `models` 未设置 | 使用内置目录作为 fallback | 用户无需手动配置 |
| `max_input/output_tokens` | 保留兜底默认值（115200/8192，≈128K×90% / 主流输出） | Agent=0 优先模型元数据 |
| 数据库 schema | 无需变更 | Provider 配置存 JSON，字段扩展不影响 |
| Agent 页面模型输入 | 下拉选择 + 手动输入兼容 | 支持自由输入模型 ID |

---

## 十一、风险与注意事项

### 11.1 风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| /models API 调用超时/失败 | 前端无法选择模型 | 降级到内置目录或手动输入 |
| API 返回的模型 ID 不在内置目录 | 缺少元数据（上下文窗口等） | 仅显示 ID，参数使用默认值 |
| Ollama 模型上下文窗口不确定 | 无法精确限制 | 允许用户在模型配置中手动设置 |
| token 估算仍有误差 | 可能超限或浪费 | 预留安全余量（90%），第一阶段先用估算 |

### 11.2 注意事项

1. **缓存一致性**：修改 Provider 配置后需清除模型发现缓存
2. **API Key 安全**：动态发现调用 API 时使用配置中的 API Key，注意日志脱敏
3. **模型 ID 格式**：不同 Provider 的模型 ID 格式差异较大，前端需支持任意字符串输入
4. **Anthropic 特殊处理**：Anthropic 没有标准 /models API，始终使用内置目录

---

## 十二、总结

本方案整合了**模型选择可视化**和**Token 配置优化**两大需求，核心亮点：

1. **三层模型配置**：内置目录 → 动态发现 → 用户自定义，灵活适配各种场景
2. **动态发现优先**：选中 Provider 后优先调用 API 获取模型列表，失败则降级
3. **Token 自适应**：根据模型元数据自动调整输入/输出 token 上限
4. **估算精度提升**：中英文区分估算，后续可升级精确 tokenizer
5. **平滑演进**：分三阶段实施，第一阶段即可获得核心价值，不影响现有功能

---

## 十三、实施进度（2026-07-14）

### DeepSeek 官方模型说明

据 [DeepSeek API Docs](https://api-docs.deepseek.com/)（2026-04-24 起）：

| 模型 ID | 状态 | 说明 |
|---------|------|------|
| `deepseek-v4-pro` | 当前 | 旗舰 |
| `deepseek-v4-flash` | 当前（默认） | 高性价比 |
| `deepseek-chat` | 遗留别名 | 指向 v4-flash 非思考模式；**2026-07-24 停用** |
| `deepseek-reasoner` | 遗留别名 | 指向 v4-flash 思考模式；**2026-07-24 停用** |

默认模型已从 `deepseek-chat` 切换为 `deepseek-v4-flash`；内置目录仍保留遗留别名以便过渡期元数据解析。

### 完成度

| 阶段 | 状态 | 备注 |
|------|------|------|
| Phase 1 | ✅ 完成 | 目录、下拉、CJK 估算、API、单测 |
| Phase 2 | ✅ 核心完成 | 动态发现、自适应 token、工具感知截断+请求层、可视化编辑 |
| Phase 3.1–3.3 | ✅ 完成 | usage 记录、成本（$/1K）、任务页展示 |
| Phase 3.4–3.6 | ❌ 未做 | tiktoken / 语义摘要 / 成本预算（可选） |

### 2026-07-14 查漏补缺

- `resolveMaxInput/OutputTokens`：agent=0 → 模型元数据 → agents.defaults
- `GetModelMeta`：发现失败时仍使用 fallback 列表查元数据
- `AgentLoop`：`SupportsTools=false` 时不向 API 发送 Tools
- Agents 页始终调用发现 API（去掉 catalog 短路）
- `GetDisplayMap._meta.models` 基于 active providers
- 成本公式按 $/1K tokens 除以 1000
- 内置目录对齐官方 ID；`config.example.yaml` 补充 type/models 示例
- 系统硬编码兜底由 2048/65536 调整为 **8192 / 115200**（对齐主流 ~128K 上下文模型）
