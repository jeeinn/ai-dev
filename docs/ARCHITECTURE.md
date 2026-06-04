# Gitea Agent Gateway — 技术架构

## 概述

Gitea Agent Gateway 是一个 Go 服务，接收 Gitea Webhook 事件，通过路由规则匹配 AI Agent，执行 LLM 驱动的任务（分析、审查、开发、修复），并将结果写回 Gitea（评论或创建 PR）。

核心设计原则：
- **事件驱动** — 所有任务由 Gitea Webhook 触发，无轮询
- **策略模式** — 5 种 Runners 对应 5 种任务类型，各自封装执行逻辑
- **沙箱隔离** — Dev/Bugfix 在目录隔离沙箱中执行，命令白名单安全控制
- **持久化队列** — SQLite 作为任务持久化层，崩溃恢复不丢任务

## 高速请求流

```
┌─────────────┐     POST /webhook/gitea
│   Gitea     │ ──────────────────────────► ┌──────────────────┐
│   Webhook   │                              │  webhook.Handler  │
└─────────────┘                              │  验签 + 去重 + 解析 │
      ▲                                     └────────┬─────────┘
      │                                              │ callback
      │                                              ▼
      │                                     ┌──────────────────┐
      │                                     │ Dispatcher       │
      │                                     │ HandleEvent()    │
      │                                     └──┬─────┬──────┬──┘
      │                                        │     │      │
      │               Router.Match(evt) ───────┘     │      │
      │               ↓ 按 priority 遍历 routes      │      │
      │               ↓ 匹配 event/action/label      │      │
      │               ↓ 返回 {Agent, Route}         │      │
      │                                        │     │      │
      │               determineTaskType(evt) ──┘     │      │
      │               ↓ 事件→任务类型映射            │      │
      │                                        │     │      │
      │               TaskQueue.Enqueue ───────┘     │      │
      │               ↓ 持久化到 SQLite             │      │
      │               ↓ 投递到 chan                 │      │
      │                                        │     │      │
      │               Executor.Worker ───────────────┘      │
      │               ↓ 并发 worker 消费 chan              │
      │               ↓ RunnerFactory.GetRunner            │
      │                                        │            │
      │               Runner.Run ───────────────────────────┘
      │               ├─ AnalyzeRunner:    单次 LLM 调用 → 评论
      │               ├─ ReviewRunner:     获取 PR diff → LLM 审查 → 评论
      │               ├─ InteractionRunner: 评论历史 → LLM 回复 → 评论
      │               ├─ DevRunner:        沙箱 → 克隆 → Agent Loop → 提交 → 创建 PR
      │               └─ BugfixRunner:     同上，不同 SystemPrompt
      │
      │               writeBackToGitea
      │               ↓ agent token → Gitea API → IssueComment
      │
      └─────────────────────────────────────────────────┘
                     用户在 Gitea 看到结果
```

## 组件详解

### 1. `internal/webhook` — Webhook 入口

**文件**: `handler.go`, `parser.go`, `signature.go`, `dedup.go`

```
HTTP POST → ReadBody → VerifySignature → ParseEvent → Dedup → callback
```

- `Handler.ServeHTTP` 处理 `POST /webhook/gitea`
- 支持 `X-Gitea-Event` / `X-Gitea-Delivery` / `X-Hub-Signature-256` 头部
- `Deduplicator` 通过 SQLite `processed_deliveries` 表去重（delivery_id 唯一）
- `ParseEvent` 将 JSON payload 解析为 `WebhookEvent` 统一结构体，合并 Issue、PR、Comment 等类型
- 异步调用 callback（Dispatcher.HandleEvent），立即返回 `200 {"status":"accepted"}`

### 2. `internal/dispatcher` — 调度核心

**文件**: `dispatcher.go`, `router.go`, `queue.go`, `executor.go`, `template.go`

#### Router — 事件到 Agent 的匹配

```go
// 按 priority DESC, id ASC 排序，遍历所有 routes
// 第一条满足所有条件的即命中（first-match-wins）
type Route struct {
    Event    string  // "issues" | "pull_request" | "issue_comment"
    Action   string  // "assigned" | "labeled" | ""(通配)
    Label    string  // Issue 必须有此 label
    Assignee string  // Issue 必须分配给此人
    Mention  string  // 评论必须 @此人
    AgentID  int64   // → 目标 Agent
    Priority int     // 越大越优先
}
```

匹配顺序：event → action → label → assignee → mention。Agent 必须是 `active` 状态才命中。

#### TaskQueue — 持久化队列

- **内存 buffer**: buffered chan（容量由 `queue_size` 配置）
- **持久化**: 每次 Enqueue 先写入 SQLite，再投递到 chan
- **崩溃恢复**: `LoadPending()` 在启动时将 DB 中 `status=pending` 的任务重新入队
- **Scanner**: 后台协程每 60s 扫描 pending 任务 + 重置超时的 running 任务（10 分钟阀值）

#### Executor — 并发执行器

- `MaxConcurrent` 个 goroutine workers，通过 semaphore chan 控制并发
- 每次 `execute` 从队列取 task → 更新状态为 `running` → `runTask` → 更新状态为 `success/failed`
- 失败后按 `retry_count` 重试（间隔 5s）
- 成功后调用 `writeBackToGitea` 将结果以 Agent 身份评论到 Issue/PR

#### determineTaskType — 事件到任务类型的硬编码映射

| 事件 | 条件 | Task Type | Runner |
|---|---|---|---|
| `issues` | label `ai:solve` | `solve_issue` | DevRunner |
| `issues` | label `ai:fix` | `fix_bug` | BugfixRunner |
| `issues` | action `assigned` / `labeled` | `analyze_issue` | AnalyzeRunner |
| `issues` | 其他 | `trigger` | AnalyzeRunner |
| `pull_request` | — | `review_pr` | ReviewRunner |
| `issue_comment` | — | `reply_comment` | InteractionRunner |
| `pull_request_comment` | — | `reply_comment` | InteractionRunner |

#### Template — 上下文模板

- 支持 Go `text/template` 语法
- 优先级：Agent 自己的 `user_template` > 配置文件的 `templates[taskType].user_template` > 默认上下文构建器
- 模板数据包含 `Event`, `Issue`, `PR`, `Comment`, `Repo`, `Sender`, `Task` 等字段

### 3. `internal/agents` — Runner 策略层

**文件**: `runners.go`, `manager.go`, `registry.go`, `interaction.go`, `prompt.go`

#### Runner 接口

```go
type Runner interface {
    Run(ctx context.Context, task *store.Task, agent *store.Agent) (*Result, error)
}

type Result struct {
    Content    string
    Action     string                 // "comment" | "pr"
    ActionData map[string]interface{}
}
```

#### RunnerFactory

```go
type RunnerFactory struct {
    llmRegistry      *llm.Registry
    giteaFactory     GiteaClientFactory
    sandboxCfg       sandbox.Config
    db               *store.DB
    defaultMaxTokens int
    defaultTemp      float64
}
```

工厂方法根据 task type 返回对应的 Runner：
- `analyze_issue` / `trigger` → **AnalyzeRunner**: 单次 LLM 调用，返回评论
- `review_pr` → **ReviewRunner**: 获取 PR diff + 文件列表 → LLM 审查 → 评论
- `reply_comment` → **InteractionRunner**: 获取最近 10 条评论历史 → LLM 回复 → 评论
- `solve_issue` → **DevRunner**: 调用 `runWriteTask(task, agent, factory, "dev")`
- `fix_bug` → **BugfixRunner**: 调用 `runWriteTask(task, agent, factory, "bugfix")`

#### runWriteTask（DevRunner / BugfixRunner 共享实现）

```
1. 获取 repo 信息（克隆 URL）
2. 创建 Sandbox（目录隔离工作区）
3. 克隆仓库（shallow clone --depth 1）
4. 创建分支（ai/dev/task-N 或 ai/bugfix/task-N）
5. 加载代码上下文（目录结构 + 关键文件）
6. 构建 Agent Loop（Provider + ToolRegistry + Messages）
7. 多轮工具调用循环（read_file, write_file, run_command, 等）
8. 检查 Git 变更 → 无变更则只评论，有变更则 commit + push + 创建 PR
```

#### Agent Manager

Agent 创建流程：
1. 用 Admin Token 在 Gitea 上创建用户（随机密码）
2. 用该用户的用户名+密码创建 API Token（Gitea 1.26+ 要求）
3. 将 Agent 信息（含 token）存入 SQLite
4. 删除 Agent 时可选择同时删除 Gitea 用户

#### PromptManager

SystemPrompt 加载优先级：
1. **DB prompt_history 表**中 `is_active=1` 的版本（UI 维护的版本历史）
2. Agent 自身 `system_prompt` 字段
3. 配置文件 `agents.templates[taskType].system_prompt`
4. 内置模板（Go 代码中注册的默认 prompt）

### 4. `internal/agent` — Agent Loop（多轮工具调用）

**文件**: `loop.go`, `tools.go`, `context.go`

```
AgentLoop.Run(messages):
  for i ← 0; i < maxIterations; i++:
    resp ← provider.ChatCompletion(messages + tools[])
    if resp.ToolCalls 为空:
      return resp.Content          ← 结束
    messages ← append(assistant message with tool calls)
    for each tool call:
      result ← ToolRegistry.ExecuteTool(call)
      messages ← append(tool result message)
  return error("max iterations reached")
```

#### 默认工具箱（DefaultTools）

| 工具 | 作用 |
|---|---|
| `read_file` | 读取工作区文件 |
| `write_file` | 写入/创建文件 |
| `list_files` | 列出目录结构（`find -maxdepth 3`） |
| `search_code` | grep 搜索代码 |
| `run_command` | 执行 shell 命令（构建/测试） |
| `apply_diff` | 应用 unified diff patch |
| `tree` | 显示目录树 |
| `git_log` | Git 提交历史 |
| `git_blame` | 文件行溯源 |

所有工具操作限制在 sandbox 工作区内，通过路径遍历防护。

### 5. `internal/sandbox` — 工作区沙箱

**文件**: `sandbox.go`, `git.go`, `audit.go`

非 Docker 的轻量沙箱：

- **命令白名单**: git, go, python, node, cat, ls, grep 等安全命令
- **路径隔离**: 所有文件操作验证在 `WorkDir` 内，防止 `../../etc/passwd`
- **超时控制**: 单命令 `CommandTimeout`（默认 5m），总任务 `TaskTimeout`（默认 30m）
- **输出限制**: stdout/stderr 各 `MaxOutput`（默认 1MB）
- **文件大小限制**: `MaxFileSize`（默认 1MB）

两种工作区模式：
- `fixed`: 固定目录 `baseDir/task_{id}`（生产推荐）
- `temp`: `os.MkdirTemp` 自动创建临时目录

#### Git 操作封装

`Git` 结构体封装了沙箱内的 Git 操作：`Clone`、`CreateBranch`、`Add`、`Commit`、`Push`。
分支名强制以 `ai/` 开头（`ValidateBranchName`），防止注入。

#### Audit Logger

所有命令执行记录到 SQLite `operation_logs` 表，包含：命令、参数、退出码、stdout/stderr 截断、耗时。

### 6. `internal/llm` — LLM Provider 抽象

**文件**: `provider.go`, `registry.go`, `openai.go`, `anthropic.go`

```go
type Provider interface {
    ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}
```

#### OpenAICompatibleProvider

兼容 OpenAI API 格式的 provider。支持 DeepSeek、Qwen、Zhipu、Moonshot、Ollama 等。
- 请求 `POST {baseURL}/chat/completions`
- 支持 Tool Calls（function calling 格式）
- 支持 DeepSeek-R1 等推理模型的 `reasoning_content` 字段回退

#### AnthropicProvider

Claude API 的适配器。将 Messages 中的 system role 提取为顶层的 `system` 参数（Anthropic API 格式）。**暂不支持 Tool Calls**。

#### Registry

从配置文件 `llm.providers` 初始化 provider 映射。根据 provider 名称自动选择适配器（Claude/Anthropic 走 AnthropicProvider，其余走 OpenAICompatibleProvider）。支持运行时热重载（config 变更时 `Reload`）。

### 7. `internal/store` — 数据持久层

**文件**: `sqlite.go`, `agent.go`, `route.go`, `task.go`, `prompt.go`, `user.go`, `system_config.go`, `log.go`

#### 数据库选择

SQLite + WAL 模式。单连接写（`SetMaxOpenConns(1)`），忙等待超时 10s。

#### 表结构

| 表 | 用途 | 关键字段 |
|---|---|---|
| `agents` | AI Agent 配置 | name, gitea_username, provider, model, max_tokens, temperature, system_prompt, loop_config(JSON), status |
| `routes` | 事件路由规则 | event, action, label, assignee, mention, agent_id(FK), priority |
| `tasks` | 执行任务 | event, repo, issue_id, agent_id(FK), task_type, context, status, delivery_id, result, error |
| `prompt_history` | Prompt 版本管理 | agent_id(FK), system_prompt, user_template, version, is_active |
| `processed_deliveries` | Webhook 去重 | delivery_id(PK) |
| `operation_logs` | 操作审计 | agent_id, task_id, action, detail |
| `users` | 管理后台用户 | username(UNIQUE), password_hash, role, is_active |
| `system_config` | 运行时配置覆盖 | key(PK), value |

#### Agent 模型

```go
type Agent struct {
    ID            int64
    Name          string
    GiteaUsername  string    // 对应 Gitea 用户
    GiteaToken     string    // 写回评论/创建 PR 的凭证
    Provider      string    // LLM provider 名称
    Model         string    // 模型名称
    MaxTokens     int       // 0 = 使用默认值
    Temperature   float64   // 0 = 使用默认值
    SystemPrompt  string    // Agent 人格
    UserTemplate  string    // 上下文模板
    LoopConfig    *AgentLoopConfig  // Agent 级别的 Loop 配置
    Status        string    // "active" | "inactive"
}
```

### 8. `internal/config` — 配置系统

**文件**: `config.go`, `schema.go`, `manager.go`

双层配置：**文件配置（YAML）** + **运行时覆盖（DB system_config 表）**

1. 启动时加载 `config.yaml`，支持 `${ENV_VAR}` 和 `${ENV_VAR:-default}` 环境变量展开
2. 自动填充默认值（host=0.0.0.0, port=8080, 等）
3. `ConfigManager.ApplyDBOverrides()` 从 `system_config` 表加载 key-value 覆盖文件配置

### 9. `internal/gitea` — Gitea API 客户端

**文件**: `client.go`, `issue.go`, `pr.go`, `repo.go`, `admin.go`, `types.go`

- 基于 `net/http` 的轻量客户端，无外部依赖
- API 路径前缀 `/api/v1`
- 认证方式：`Authorization: token {token}`
- 支持：创建/删除用户、Issue CRUD/评论/Label、PR 创建/Diff/文件列表、仓库信息等

### 10. `internal/api` — 管理 REST API

**文件**: `router.go`, `auth.go`, `auth_handler.go`, `config.go`, `prompt_templates.go`

两个认证层：
- **Bearer Token**: 简单 API token，用于 Agent 和 Route 管理接口（`api.auth_token` 配置）
- **JWT**: 用户登录认证，用于 User 管理、系统配置、Prompt 管理接口

API 端点概览：

| 方法 | 路径 | 认证 | 用途 |
|---|---|---|---|
| GET | `/api/agents` | Token | 列 Agent |
| POST | `/api/agents` | Token | 创建 Agent |
| GET | `/api/agents/{id}/routes` | Token | Agent 路由 |
| POST | `/api/routes` | Token | 创建路由 |
| GET | `/api/tasks` | Token | 列任务（支持分页/筛选） |
| GET | `/api/stats` | Token | 统计 |
| POST | `/api/auth/login` | 无 | 登录获取 JWT |
| GET | `/api/config` | JWT | 系统配置 |
| PUT | `/api/config` | JWT | 更新配置覆盖 |

### 11. Web 前端

**技术栈**: Vue 3 + Element Plus + Vue Router

页面：
- **Dashboard** — 状态概览、统计数据
- **Agents** — Agent 列表、创建、编辑
- **AgentDetail** — Agent 详情、路由管理、Prompt 版本管理、任务历史
- **Tasks** — 任务列表、筛选、详情
- **SystemConfig** — Gitea 连接、LLM Provider、Agent 默认参数
- **Users** — 管理后台用户 CRUD
- **Login** — JWT 登录

前端通过嵌入 Go 二进制 (`//go:embed web/dist/*`) 由同一个进程提供静态文件服务。

## 关键设计决策

### 为什么不用 Docker 沙箱？

项目初期评估 Docker 沙箱的开销和复杂度后，选择**目录隔离 + 命令白名单**的轻量方案：
- 更快的启动速度（毫秒级 vs 秒级）
- 无容器运行时依赖
- 足够的安全边界（白名单 + 路径验证 + 超时控制）
- 缺点：缺乏内核级隔离，不适合运行不可信代码

### Runner 策略模式

5 种 Runners 共享一个 `RunnerFactory`，但各自的执行逻辑差异大（AnalyzeRunner 是简单 LLM 调用，DevRunner 是多轮 agent loop + git 操作）。策略模式让：
- 新增任务类型只需新增 Runner 实现 + 注册到 `GetRunner`
- 各 Runner 独立测试、独立演进
- 共享依赖（LLM Registry、Gitea Client Factory）通过 Factory 注入

### 为什么 Task Type 是硬编码的？

`determineTaskType` 是个不超过 30 行的 switch 函数，将 Gitea 事件域（issues/assigned, pull_request, etc.）映射到系统内部的任务类型。这是**域不变性**的映射，不是**业务规则**：
- 用户不需要关心 `issues/assigned → analyze_issue` 还是 `trigger`
- 用户控制的是"谁来干"（Route → Agent），不是"用什么方式干"（→ Runner）
- label 覆盖机制（`ai:solve` / `ai:fix`）给了用户事件级别的控制权，无需更复杂的配置面

### RunnerFactory 默认值传播

```go
RunnerFactory (defaultMaxTokens=4096, defaultTemp=0.3)
  ├─ resolveMaxTokens(agent.MaxTokens)  // agent > default
  └─ resolveTemperature(agent.Temperature)  // agent > default
```

Agent 的 `MaxTokens`/`Temperature` 设为 0 时，自动使用工厂默认值。注意：这意味着 Temperature=0（确定性输出）无法通过 Agent 设置，需通过代码修改默认值。

## 测试架构

- **单元测试**: 各 package 内，测试单个函数/方法，无外部依赖
- **集成测试**: `tests/integration/`，使用 `TestEnv` 提供 in-memory SQLite + mock Gitea 服务 + mock LLM Provider + 完整 HTTP 测试服务器
- 决策规则：需要 TestEnv（DB/HTTP/Mock） → 集成测试；否则 → 单元测试

## 部署

```bash
# 1. 准备配置文件
cp config.example.yaml config.yaml
# 编辑 config.yaml，至少配置 Gitea URL、Admin Token、API Key

# 2. 构建
go build -o gateway .

# 3. 运行
./gateway -config config.yaml

# 4. 在 Gitea 仓库设置中添加 Webhook：
#    URL: http://your-server:8080/webhook/gitea
#    密钥: 与 config.yaml 中的 webhook_secret 一致
#    触发事件: Issues, Pull Requests, Issue Comments
```

首次启动自动：
1. 创建 SQLite 数据库并执行迁移
2. 从 `system_config` 表加载配置覆盖
3. 从 DB 加载 pending 任务到队列
4. 启动 Scanner + Workers

## 项目文件清单

```
main.go                              # 入口：HTTP 服务、组件组装、优雅关闭
internal/
├── webhook/                         # Webhook 接收
│   ├── handler.go                   #   HTTP Handler（验签/去重/解析/回调）
│   ├── parser.go                    #   事件类型定义 + JSON 解析
│   ├── signature.go                 #   HMAC-SHA256 签名验证
│   └── dedup.go                     #   delivery_id 去重
├── dispatcher/                      # 调度核心
│   ├── dispatcher.go                #   编排器（Router→Queue→Executor）
│   ├── router.go                    #   路由匹配（event→agent）
│   ├── queue.go                     #   持久化任务队列（chan+SQLite）
│   ├── executor.go                  #   并发执行器（worker+重试+写回）
│   └── template.go                  #   模板渲染引擎
├── agents/                          # Runner 策略层
│   ├── runners.go                   #   Runner 接口 + 5 种实现 + Factory
│   ├── manager.go                   #   Agent 生命周期（Gitea 账号创建）
│   ├── registry.go                  #   Agent 内存注册表（快速查找）
│   ├── interaction.go              #    @Mention 检测
│   └── prompt.go                    #   Prompt 管理器（DB→Agent→Config→内置）
├── agent/                           # Agent Loop
│   ├── loop.go                      #   多轮工具调用循环
│   ├── tools.go                     #   Tool 注册 + 默认工具集
│   └── context.go                   #   代码上下文加载
├── llm/                             # LLM 抽象层
│   ├── provider.go                  #   Provider 接口 + 类型定义
│   ├── registry.go                  #   Provider 注册表
│   ├── openai.go                    #   OpenAI 兼容 API 客户端
│   └── anthropic.go                 #   Claude API 客户端
├── sandbox/                         # 工作区沙箱
│   ├── sandbox.go                   #   目录隔离 + 命令白名单 + 文件操作
│   ├── git.go                       #   Git 操作封装
│   └── audit.go                     #   命令审计日志
├── gitea/                           # Gitea API 客户端
│   ├── client.go                    #   HTTP 客户端（do 方法）
│   ├── issue.go                     #   Issue 操作
│   ├── pr.go                        #   PR 操作
│   ├── repo.go                      #   仓库操作
│   ├── admin.go                     #   管理员操作（创建/删除用户）
│   └── types.go                     #   共享类型
├── store/                           # 数据持久层
│   ├── sqlite.go                    #   SQLite 连接 + 迁移
│   ├── agent.go                     #   Agent CRUD
│   ├── route.go                     #   Route CRUD
│   ├── task.go                      #   Task CRUD + 筛选
│   ├── prompt.go                    #   Prompt 版本管理
│   ├── user.go                      #   用户 CRUD
│   ├── system_config.go             #   运行时配置覆盖
│   └── log.go                       #   操作日志
├── api/                             # 管理 REST API
│   ├── router.go                    #   路由注册 + handler 实现
│   ├── auth.go                      #   Bearer Token 中间件
│   ├── auth_handler.go              #   JWT 登录接口
│   ├── config.go                    #   系统配置 API
│   └── prompt_templates.go          #   Prompt 模板 API
├── config/                          # 配置系统
│   ├── schema.go                    #   配置结构体定义
│   ├── config.go                    #   YAML 加载 + 环境变量展开 + 默认值
│   └── manager.go                   #   配置管理器（文件 + DB 覆盖）
└── auth/                            # 认证
    ├── jwt.go                       #   JWT 创建/验证
    ├── password.go                  #   密码哈希（bcrypt）
    └── auth_test.go                  #   认证测试

web/src/                             # Vue 3 前端
├── App.vue, main.js                  # 入口
├── views/                            # 页面组件
│   ├── Dashboard.vue
│   ├── Agents.vue
│   ├── AgentDetail.vue
│   ├── Tasks.vue
│   ├── SystemConfig.vue
│   ├── Users.vue
│   └── Login.vue
├── components/                       # 共享组件
└── router/                           # 路由
    └── index.js
```
