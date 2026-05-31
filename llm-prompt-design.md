# LLM 调用层 & Prompt 管理设计

## 一、Provider 架构

只实现两种 Provider，覆盖所有主流模型。

### 1.1 Provider 接口

```go
type LLMProvider interface {
    ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    ChatCompletionStream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
}

type ChatRequest struct {
    Model       string    `json:"model"`
    Messages    []Message `json:"messages"`
    MaxTokens   int       `json:"max_tokens"`
    Temperature float64   `json:"temperature"`
    Stream      bool      `json:"stream"`
}

type Message struct {
    Role    string `json:"role"`    // system / user / assistant
    Content string `json:"content"`
}

type ChatResponse struct {
    Content string `json:"content"`
    Usage   Usage  `json:"usage"`
}

type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}

type StreamChunk struct {
    Delta string `json:"delta"`
    Done  bool   `json:"done"`
}
```

### 1.2 两种 Provider 实现

**Provider A：OpenAI 兼容（覆盖 90% 场景）**

```go
type OpenAICompatibleProvider struct {
    BaseURL string
    APIKey  string
    Client  *http.Client
}

// 通过换 base_url 覆盖以下所有模型：
// - OpenAI:       https://api.openai.com/v1
// - DeepSeek:     https://api.deepseek.com/v1
// - 通义千问:      https://dashscope.aliyuncs.com/compatible-mode/v1
// - 智谱 GLM:     https://open.bigmodel.cn/api/paas/v4
// - Moonshot:     https://api.moonshot.cn/v1
// - 零一万物:      https://api.lingyiwanwu.com/v1
// - 火山引擎:      https://ark.cn-beijing.volces.com/api/v3
// - 本地 Ollama:  http://localhost:11434/v1
// - 任何 OpenAI 兼容接口
```

**Provider B：Anthropic（Claude 专用）**

```go
type AnthropicProvider struct {
    APIKey string
    Client *http.Client
}

// Claude API 格式与 OpenAI 不同：
// - 使用 x-api-key 头部认证
// - messages 格式不同（system 单独传）
// - 响应结构不同
// 必须单独实现
```

### 1.3 Provider 注册表

```go
// 运行时根据配置自动创建 Provider 实例
type ProviderRegistry struct {
    providers map[string]LLMProvider
}

func (r *ProviderRegistry) Get(name string) (LLMProvider, error) {
    p, ok := r.providers[name]
    if !ok {
        return nil, fmt.Errorf("provider %q not found", name)
    }
    return p, nil
}

// 根据配置初始化
func NewProviderRegistry(cfg *config.Config) *ProviderRegistry {
    registry := &ProviderRegistry{providers: make(map[string]LLMProvider)}
    
    for name, pcfg := range cfg.LLM.Providers {
        if name == "claude" || name == "anthropic" {
            registry.providers[name] = NewAnthropicProvider(pcfg.APIKey)
        } else {
            registry.providers[name] = NewOpenAICompatibleProvider(
                pcfg.BaseURL, pcfg.APIKey,
            )
        }
    }
    return registry
}
```

### 1.4 配置格式

```yaml
llm:
  providers:
    # ---------- OpenAI 兼容系列 ----------
    openai:
      base_url: "https://api.openai.com/v1"
      api_key: "${OPENAI_API_KEY}"

    deepseek:
      base_url: "https://api.deepseek.com/v1"
      api_key: "${DEEPSEEK_API_KEY}"

    qwen:
      base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
      api_key: "${QWEN_API_KEY}"

    zhipu:
      base_url: "https://open.bigmodel.cn/api/paas/v4"
      api_key: "${ZHIPU_API_KEY}"

    ollama:
      base_url: "http://localhost:11434/v1"
      api_key: "ollama"  # ollama 不需要 key，但字段必须填

    # ---------- Anthropic 独立 ----------
    claude:
      api_key: "${ANTHROPIC_API_KEY}"
```

### 1.5 Agent 绑定 Provider + Model

```yaml
agents:
  analyze:
    provider: "deepseek"
    model: "deepseek-chat"
    max_tokens: 4096
    temperature: 0.3

  dev:
    provider: "claude"
    model: "claude-sonnet-4-20250514"
    max_tokens: 8192
    temperature: 0.2

  bugfix:
    provider: "deepseek"
    model: "deepseek-chat"
    max_tokens: 8192
    temperature: 0.1

  review:
    provider: "deepseek"
    model: "deepseek-chat"
    max_tokens: 4096
    temperature: 0.2
```

## 二、Prompt 两层管理

**原则：DB > 配置文件，零硬编码。代码只从配置文件解析，DB 覆盖配置文件。**

### 2.1 优先级

```
DB（Web UI 修改后持久化）
  ↓ 覆盖
config.yaml（预置默认值，随版本更新）
```

系统启动时：
1. 从 config.yaml 加载所有 Agent 的 prompt 作为基线
2. 查询 DB，如果有覆盖记录则替换
3. 运行时使用的 prompt = DB 有值 ? DB : config.yaml

### 2.2 Prompt 结构

```go
type AgentPrompt struct {
    AgentName    string `json:"agent_name" yaml:"agent_name"`
    SystemPrompt string `json:"system_prompt" yaml:"system_prompt"`
    UserTemplate string `json:"user_template" yaml:"user_template"`
}
```

- `SystemPrompt`：系统提示词，定义 Agent 角色和行为规范
- `UserTemplate`：用户消息模板，Go template 语法，变量来自 Issue/PR 数据

### 2.3 配置文件中的 Prompt（基线）

```yaml
agents:
  analyze:
    provider: "deepseek"
    model: "deepseek-chat"
    max_tokens: 4096
    temperature: 0.3
    prompt:
      system_prompt: |
        你是一个需求分析专家。你的任务是分析用户提交的 Issue，评估需求的完整性和可行性。
        
        分析维度：
        1. 需求是否清晰、无歧义
        2. 是否缺少验收标准
        3. 是否有技术约束需要确认
        4. 是否有潜在风险
        
        输出格式：Markdown 结构化报告。
      user_template: |
        请分析以下 Issue：
        
        ## Issue #{{.Issue.Number}}
        - 标题: {{.Issue.Title}}
        - 作者: {{.Issue.Author}}
        - 标签: {{.Issue.Labels}}
        
        ## 描述
        {{.Issue.Body}}
        
        请输出结构化的需求分析报告。

  dev:
    provider: "claude"
    model: "claude-sonnet-4-20250514"
    max_tokens: 8192
    temperature: 0.2
    prompt:
      system_prompt: |
        你是一个高级研发工程师。根据需求分析报告，编写高质量的代码实现。
        
        规范：
        - 遵循项目现有代码风格
        - 不引入不必要的依赖
        - 编写清晰的提交信息
        - PR 描述说明实现思路
      user_template: |
        请根据以下需求实现代码：
        
        ## Issue #{{.Issue.Number}}: {{.Issue.Title}}
        {{.Issue.Body}}
        
        ## 需求分析报告
        {{.AnalysisReport}}
        
        ## 仓库信息
        - 语言: {{.Repo.Language}}
        - 默认分支: {{.Repo.DefaultBranch}}
        
        请创建分支 "{{.BranchName}}"，实现代码并提交 PR。

  bugfix:
    provider: "deepseek"
    model: "deepseek-chat"
    max_tokens: 8192
    temperature: 0.1
    prompt:
      system_prompt: |
        你是一个 Bug 修复专家。分析 Bug 描述，定位问题根因，编写修复代码。
        
        规范：
        - 先分析根因，再写修复代码
        - 修复要最小化变更，不重构无关代码
        - 说明修复原理
      user_template: |
        请修复以下 Bug：
        
        ## Issue #{{.Issue.Number}}: {{.Issue.Title}}
        {{.Issue.Body}}
        
        ## 仓库信息
        - 语言: {{.Repo.Language}}
        
        请定位问题、编写修复代码并提交 PR。

  review:
    provider: "deepseek"
    model: "deepseek-chat"
    max_tokens: 4096
    temperature: 0.2
    prompt:
      system_prompt: |
        你是一个资深代码审查专家。审查 PR 的代码变更，输出结构化报告。
        
        审查维度：
        1. 逻辑正确性
        2. 安全风险
        3. 性能影响
        4. 代码风格一致性
        5. 测试覆盖
        
        严重程度：🔴 必须修复 / 🟡 建议修复 / 🟢 可选优化
      user_template: |
        请审查以下 PR：
        
        ## PR #{{.PR.Number}}: {{.PR.Title}}
        {{.PR.Body}}
        
        ## 变更文件
        {{range .PR.ChangedFiles}}
        - ` + "`{{.Filename}}`" + ` (+{{.Additions}} -{{.Deletions}})
        {{end}}
        
        ## Diff
        ` + "```" + `
        {{.PR.Diff}}
        ` + "```" + `
        
        请输出结构化审查报告。
```

### 2.4 DB 存储结构

```sql
CREATE TABLE agent_prompts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_name  TEXT NOT NULL UNIQUE,           -- analyze / dev / bugfix / review
    system_prompt TEXT NOT NULL,
    user_template TEXT NOT NULL,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_by  TEXT DEFAULT 'system'           -- system / admin
);

CREATE TABLE prompt_history (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_name  TEXT NOT NULL,
    system_prompt TEXT NOT NULL,
    user_template TEXT NOT NULL,
    version     INTEGER NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_by  TEXT DEFAULT 'admin'
);
```

- `agent_prompts`：当前生效的 prompt（每 agent 一条）
- `prompt_history`：历史版本，方便回溯和回滚

### 2.5 启动加载流程

```go
func LoadPrompts(cfg *config.Config, db *sql.DB) map[string]*AgentPrompt {
    prompts := make(map[string]*AgentPrompt)

    // 第一层：从配置文件加载基线
    for name, agentCfg := range cfg.Agents {
        prompts[name] = &AgentPrompt{
            AgentName:    name,
            SystemPrompt: agentCfg.Prompt.SystemPrompt,
            UserTemplate: agentCfg.Prompt.UserTemplate,
        }
    }

    // 第二层：DB 覆盖（如果存在）
    rows, _ := db.Query("SELECT agent_name, system_prompt, user_template FROM agent_prompts")
    defer rows.Close()
    for rows.Next() {
        var p AgentPrompt
        rows.Scan(&p.AgentName, &p.SystemPrompt, &p.UserTemplate)
        prompts[p.AgentName] = &p  // DB 覆盖配置文件
    }

    return prompts
}
```

### 2.6 Web UI Prompt 编辑

API：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/prompts` | 获取所有 Agent 的当前 prompt |
| GET | `/api/prompts/:agent` | 获取单个 Agent 的 prompt |
| PUT | `/api/prompts/:agent` | 更新 prompt（写 DB + 记历史） |
| POST | `/api/prompts/:agent/reset` | 重置为配置文件默认值（删 DB 记录） |
| GET | `/api/prompts/:agent/history` | 查看历史版本 |
| POST | `/api/prompts/:agent/preview` | 预览渲染效果（传入模板变量） |

Web UI 页面：

```
┌──────────────────────────────────────────────────────────────┐
│ Agent: [研发 Agent ▼]                    [重置为默认] [保存]  │
│                                                              │
│ ┌─ System Prompt ──────────────────────────────────────────┐ │
│ │ 你是一个高级研发工程师。根据需求分析报告，编写高质量...    │ │
│ │                                                          │ │
│ │ [编辑模式] 字数: 256                                      │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ ┌─ User Template ──────────────────────────────────────────┐ │
│ │ 请根据以下需求实现代码：                                   │ │
│ │ ## Issue #{{.Issue.Number}}: {{.Issue.Title}}             │ │
│ │ ...                                                       │ │
│ │                                                          │ │
│ │ 可用变量: .Issue.Number .Issue.Title .Issue.Body          │ │
│ │          .Repo.Language .BranchName .AnalysisReport       │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ [预览渲染] [查看历史]                                         │
└──────────────────────────────────────────────────────────────┘
```

## 三、模型切换策略

不同 Agent 可以用不同模型，按需选择：

| Agent | 推荐模型 | 理由 |
|-------|---------|------|
| 需求分析 | DeepSeek Chat | 分析任务，性价比高 |
| 研发 | Claude Sonnet | 代码生成质量最佳 |
| Bug 修复 | DeepSeek Chat / Claude | 定位+修复，看复杂度 |
| PR 审查 | DeepSeek Chat | 审查任务，不需要最强模型 |
| 测试 | DeepSeek Chat | 生成测试用例 |

切换只需改配置文件中的 `provider` + `model`，重启生效。
