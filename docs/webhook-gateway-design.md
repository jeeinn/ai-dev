# Webhook Gateway 详细设计（v2 动态 Agent 版）

## 一、技术选型

| 维度 | 选择 | 理由 |
|------|------|------|
| 语言 | **Go 1.22+** | 单二进制、零依赖、高并发 |
| 前端 | **Vue 3 + Element Plus** | `go:embed` 打包进二进制 |
| 配置存储 | **SQLite** | 单文件、无需额外数据库服务 |
| 队列 | **内存队列 + SQLite 持久化** | 无需 Redis，重启不丢任务 |
| 部署 | **单二进制** | `scp` 过去直接跑 |

## 二、目录结构

```
gitea-agent-gateway/
├── main.go                    # 入口
├── go.mod
├── go.sum
├── internal/
│   ├── webhook/               # Webhook 接收 & 签名验证
│   │   ├── handler.go         # HTTP Handler
│   │   ├── parser.go          # 事件解析
│   │   ├── signature.go       # HMAC 签名验证
│   │   └── dedup.go           # 幂等去重（X-Gitea-Delivery）
│   ├── dispatcher/            # Agent 调度器
│   │   ├── router.go          # Label+Assignee 双条件路由
│   │   ├── queue.go           # 任务队列
│   │   └── executor.go        # Agent 执行器
│   ├── agents/                # Agent 管理
│   │   ├── manager.go         # Agent CRUD + Gitea 账号自动注册
│   │   ├── registry.go        # Agent 注册表（运行时）
│   │   └── interaction.go     # @Mention 互动处理
│   ├── gitea/                 # Gitea API 客户端
│   │   ├── client.go          # API 封装
│   │   ├── issue.go           # Issue 操作
│   │   ├── pr.go              # PR 操作
│   │   ├── repo.go            # 仓库操作
│   │   └── admin.go           # Admin API（账号注册、Token 生成）
│   ├── llm/                   # LLM 调用抽象
│   │   ├── provider.go        # Provider 接口
│   │   ├── openai.go          # OpenAI 兼容接口
│   │   └── anthropic.go       # Claude API
│   ├── config/                # 配置管理
│   │   ├── config.go          # 配置加载
│   │   └── schema.go          # 配置结构定义
│   ├── store/                 # 数据存储
│   │   ├── sqlite.go          # SQLite 初始化
│   │   ├── agent.go           # Agent 注册表 CRUD
│   │   ├── route.go           # 路由规则 CRUD
│   │   ├── task.go            # 任务记录
│   │   ├── prompt.go          # Prompt 管理（DB 层）
│   │   └── log.go             # 操作日志
│   └── api/                   # 管理 API
│       ├── router.go          # API 路由
│       ├── agent.go           # Agent 管理接口
│       ├── config.go          # 配置管理接口
│       ├── task.go            # 任务查询接口
│       ├── prompt.go          # Prompt 管理接口
│       └── stats.go           # 统计接口
├── web/                       # 前端源码
│   ├── src/
│   │   ├── App.vue
│   │   ├── views/
│   │   │   ├── Dashboard.vue      # 仪表盘
│   │   │   ├── Agents.vue         # Agent 管理（核心页面）
│   │   │   ├── AgentCreate.vue    # 创建 Agent
│   │   │   ├── Tasks.vue          # 任务列表
│   │   │   ├── Prompts.vue        # Prompt 编辑
│   │   │   ├── Routes.vue         # 路由规则管理
│   │   │   ├── Config.vue         # 系统配置
│   │   │   └── Logs.vue           # 操作日志
│   │   └── components/
│   └── dist/                    # 构建产物（go:embed）
├── embed.go                   # go:embed 前端资源
└── config.example.yaml        # 示例配置
```

## 三、核心流程

```
Gitea Webhook POST /webhook/gitea
        │
        ▼
┌─────────────────┐
│  签名验证        │ ← HMAC-SHA256 (X-Gitea-Signature)
│  幂等去重        │ ← X-Gitea-Delivery 唯一 ID
│  事件类型过滤    │ ← 只处理 issues/PR/comment
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  事件解析        │ ← 提取 repo/issue/label/assignee/action/mention
│  双条件路由      │ ← Label + Assignee 匹配 Agent
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  任务入队        │ ← SQLite 持久化 + 内存队列
│  返回 200        │ ← 立即响应 Gitea
└────────┬────────┘
         │ (异步)
         ▼
┌─────────────────┐
│  Agent 执行器    │ ← 从队列取任务
│  加载 Agent 配置  │ ← 模型/Prompt/Token（从 SQLite）
│  调用 LLM        │ ← 生成代码/分析/审查
│  用 Agent Token  │ ← 回写 Gitea（评论/PR/标签）
│  更新任务状态    │ ← SQLite
└─────────────────┘
```

## 四、Webhook Handler 设计

```go
// internal/webhook/handler.go
func HandleWebhook(w http.ResponseWriter, r *http.Request) {
    // 1. 验证签名
    if !verifySignature(r, config.WebhookSecret) {
        http.Error(w, "Invalid signature", 401)
        return
    }

    // 2. 幂等去重
    deliveryID := r.Header.Get("X-Gitea-Delivery")
    if store.IsProcessed(deliveryID) {
        w.WriteHeader(200)
        return
    }

    // 3. 解析事件类型
    event := r.Header.Get("X-Gitea-Event")
    payload, _ := io.ReadAll(r.Body)

    // 4. 根据事件类型解析
    switch event {
    case "issues":
        handleIssuesEvent(payload, deliveryID)
    case "pull_request":
        handlePREvent(payload, deliveryID)
    case "issue_comment":
        handleCommentEvent(payload, deliveryID)  // @Mention 互动
    default:
        w.WriteHeader(200)
        return
    }

    // 5. 标记已处理
    store.MarkProcessed(deliveryID)

    // 6. 立即返回 200（异步处理）
    w.WriteHeader(200)
    json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}
```

## 五、双条件路由设计

```go
// internal/dispatcher/router.go
type Route struct {
    ID       int64  `json:"id"`
    Event    string `json:"event"`     // issues / pull_request / issue_comment
    Action   string `json:"action"`    // labeled / assigned / opened / created
    Label    string `json:"label"`     // 匹配标签（可选）
    Assignee string `json:"assignee"`  // 匹配分配人（可选）
    Mention  string `json:"mention"`   // 匹配 @Mention（可选）
    AgentID  int64  `json:"agent_id"`  // 关联的 Agent
    Priority int    `json:"priority"`
}

// 路由匹配：Label + Assignee 双条件
func (r *Router) Match(event string, payload *WebhookPayload) *Route {
    for _, route := range r.routes {
        if route.Event != event {
            continue
        }
        // 检查 action
        if route.Action != "" && route.Action != payload.Action {
            continue
        }
        // 检查 label（如果配置了）
        if route.Label != "" && !hasLabel(payload.Issue.Labels, route.Label) {
            continue
        }
        // 检查 assignee（如果配置了）
        if route.Assignee != "" && !hasAssignee(payload.Issue.Assignees, route.Assignee) {
            continue
        }
        // 检查 mention（如果配置了）
        if route.Mention != "" && !containsMention(payload.Comment.Body, route.Mention) {
            continue
        }
        return &route
    }
    return nil
}
```

### 路由规则配置（数据库）

```sql
CREATE TABLE routes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    event       TEXT NOT NULL,              -- issues / pull_request / issue_comment
    action      TEXT,                       -- labeled / assigned / opened / created
    label       TEXT,                       -- 匹配标签（可选）
    assignee    TEXT,                       -- 匹配分配人（可选）
    mention     TEXT,                       -- 匹配 @Mention（可选）
    agent_id    INTEGER NOT NULL,           -- 关联的 Agent
    priority    INTEGER DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
```

## 六、Agent 管理

### 6.1 Agent 注册表

```sql
CREATE TABLE agents (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,              -- 显示名："前端研发 Agent"
    gitea_username  TEXT NOT NULL UNIQUE,       -- Gitea 账号：ai-frontend-dev
    gitea_token     TEXT NOT NULL,              -- 该账号的 API Token
    avatar_url      TEXT,                       -- 头像 URL
    provider        TEXT NOT NULL DEFAULT 'deepseek',
    model           TEXT NOT NULL DEFAULT 'deepseek-chat',
    max_tokens      INTEGER DEFAULT 4096,
    temperature     REAL DEFAULT 0.3,
    system_prompt   TEXT NOT NULL,
    user_template   TEXT NOT NULL,
    status          TEXT DEFAULT 'active',      -- active / disabled
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 6.2 Agent 创建流程（API）

```go
// internal/agents/manager.go
func (m *Manager) CreateAgent(req CreateAgentRequest) (*Agent, error) {
    // 1. 调用 Gitea Admin API 注册账号
    giteaUser, err := m.gitea.AdminCreateUser(gitea.CreateUserRequest{
        LoginName:  req.GiteaUsername,
        Username:   req.GiteaUsername,
        Email:      req.GiteaUsername + "@gateway.local",
        Password:   generateRandomPassword(),
        SendNotify: false,
    })
    if err != nil {
        return nil, fmt.Errorf("创建 Gitea 账号失败: %w", err)
    }

    // 2. 为该账号生成 API Token
    token, err := m.gitea.AdminCreateToken(giteaUser.ID, "gateway-agent-token")
    if err != nil {
        return nil, fmt.Errorf("生成 Token 失败: %w", err)
    }

    // 3. 存储到 SQLite
    agent := &Agent{
        Name:           req.Name,
        GiteaUsername:  req.GiteaUsername,
        GiteaToken:     token.SHA1,
        Provider:       req.Provider,
        Model:          req.Model,
        MaxTokens:      req.MaxTokens,
        Temperature:    req.Temperature,
        SystemPrompt:   req.SystemPrompt,
        UserTemplate:   req.UserTemplate,
        Status:         "active",
    }
    if err := m.store.CreateAgent(agent); err != nil {
        return nil, err
    }

    // 4. 创建默认路由规则
    if req.DefaultRoute != nil {
        req.DefaultRoute.AgentID = agent.ID
        m.store.CreateRoute(req.DefaultRoute)
    }

    return agent, nil
}
```

### 6.3 @Mention 互动处理

```go
// internal/agents/interaction.go
func (h *InteractionHandler) HandleComment(payload *WebhookPayload) {
    // 从评论中提取所有 @用户名
    mentions := extractMentions(payload.Comment.Body)

    for _, username := range mentions {
        // 查找是否是我们的 Agent
        agent, err := h.store.GetAgentByGiteaUsername(username)
        if err != nil || agent == nil {
            continue // 不是 Agent，跳过
        }

        // 创建互动任务
        task := &Task{
            Event:    "issue_comment",
            Repo:     payload.Repository.FullName,
            IssueID:  payload.Issue.Number,
            AgentID:  agent.ID,
            Type:     "interaction",
            Context:  payload.Comment.Body,  // 评论内容作为上下文
        }
        h.dispatcher.Enqueue(task)
    }
}
```

## 七、可视化配置界面

### 7.1 Agent 管理页面（核心）

```
┌──────────────────────────────────────────────────────────────────────┐
│ AI Agent 管理                                    [+ 创建新 Agent]    │
│                                                                      │
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │ 头像 │ 名称          │ Gitea 账号        │ 模型     │ 状态 │ 操作│ │
│ │ 🤖  │ 需求分析 Agent │ ai-analyze        │ deepseek │ 启用  │ ✏️🗑│ │
│ │ 👨‍💻 │ 前端研发 Agent  │ ai-frontend-dev   │ deepseek │ 启用  │ ✏️🗑│ │
│ │ 👩‍💻 │ 后端研发 Agent  │ ai-backend-dev    │ claude   │ 启用  │ ✏️🗑│ │
│ │ 🔍  │ Code Review    │ ai-reviewer       │ deepseek │ 启用  │ ✏️🗑│ │
│ │ 🐛  │ Bug 修复 Agent │ ai-bugfix         │ deepseek │ 禁用  │ ✏️🗑│ │
│ └──────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────┘
```

### 7.2 创建 Agent 页面

```
┌──────────────────────────────────────────────────────────────────┐
│ 创建 AI Agent                                                     │
│                                                                   │
│ 基本信息                                                          │
│ ┌─────────────────────────────────────────────────────────────┐  │
│ │ Agent 名称:    [前端研发 Agent_______________]               │  │
│ │ Gitea 用户名:  [ai-frontend-dev_____________]  [检查可用]    │  │
│ │ 头像:          [上传] [使用默认]                              │  │
│ └─────────────────────────────────────────────────────────────┘  │
│                                                                   │
│ 模型配置                                                          │
│ ┌─────────────────────────────────────────────────────────────┐  │
│ │ Provider:    [deepseek ▼]                                    │  │
│ │ Model:       [deepseek-chat ▼]                               │  │
│ │ Max Tokens:  [8192]                                          │  │
│ │ Temperature: [0.2]                                           │  │
│ └─────────────────────────────────────────────────────────────┘  │
│                                                                   │
│ Prompt 配置                                                       │
│ ┌─────────────────────────────────────────────────────────────┐  │
│ │ System Prompt:                                               │  │
│ │ ┌─────────────────────────────────────────────────────────┐ │  │
│ │ │ 你是一个高级前端研发工程师...                              │ │  │
│ │ └─────────────────────────────────────────────────────────┘ │  │
│ │ User Template:                                               │  │
│ │ ┌─────────────────────────────────────────────────────────┐ │  │
│ │ │ 请根据以下需求实现代码：                                   │ │  │
│ │ │ ## Issue #{{.Issue.Number}}...                           │ │  │
│ │ └─────────────────────────────────────────────────────────┘ │  │
│ └─────────────────────────────────────────────────────────────┘  │
│                                                                   │
│ 默认路由规则                                                      │
│ ┌─────────────────────────────────────────────────────────────┐  │
│ │ 触发事件:  [issues ▼]                                        │  │
│ │ 动作:      [assigned ▼]                                      │  │
│ │ 匹配标签:  [agent:dev:frontend]                               │  │
│ │ 优先级:    [1]                                               │  │
│ └─────────────────────────────────────────────────────────────┘  │
│                                                                   │
│ [测试 Agent]                              [创建] [取消]            │
└──────────────────────────────────────────────────────────────────┘
```

## 八、任务队列设计

```go
type Task struct {
    ID        int64      `json:"id"`
    Event     string     `json:"event"`       // issues / pull_request / issue_comment
    Repo      string     `json:"repo"`        // owner/repo
    IssueID   int64      `json:"issue_id"`    // Issue 或 PR 编号
    AgentID   int64      `json:"agent_id"`    // 关联的 Agent（动态）
    Type      string     `json:"type"`        // trigger / interaction
    Context   string     `json:"context"`     // 上下文（@Mention 评论内容等）
    Status    TaskStatus `json:"status"`
    Priority  int        `json:"priority"`
    CreatedAt time.Time  `json:"created_at"`
    StartedAt *time.Time `json:"started_at"`
    FinishedAt *time.Time `json:"finished_at"`
    Result    string     `json:"result"`
    Error     string     `json:"error"`
}
```

## 九、配置文件格式

```yaml
server:
  host: "0.0.0.0"
  port: 8080

gitea:
  url: "https://git.example.com"
  admin_token: "${GITEA_ADMIN_TOKEN}"  # 用于自动注册 Agent 账号
  webhook_secret: "${WEBHOOK_SECRET}"

workspace:
  base_dir: "/tmp/gateway-work"
  cleanup_after: "24h"
  max_disk_usage: "10GB"

dispatcher:
  max_concurrent: 3
  retry_count: 1
  timeout: 300
  queue_size: 100

database:
  path: "./data/gateway.db"

logging:
  level: "info"
  path: "./data/logs"
```

## 十、API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/webhook/gitea` | Gitea Webhook 接收 |
| **Agent 管理** | | |
| GET | `/api/agents` | Agent 列表 |
| POST | `/api/agents` | 创建 Agent（自动注册 Gitea 账号） |
| GET | `/api/agents/:id` | Agent 详情 |
| PUT | `/api/agents/:id` | 更新 Agent |
| DELETE | `/api/agents/:id` | 删除 Agent |
| POST | `/api/agents/:id/enable` | 启用 Agent |
| POST | `/api/agents/:id/disable` | 禁用 Agent |
| POST | `/api/agents/:id/test` | 测试 Agent |
| **路由规则** | | |
| GET | `/api/routes` | 路由规则列表 |
| POST | `/api/routes` | 创建路由规则 |
| PUT | `/api/routes/:id` | 更新路由规则 |
| DELETE | `/api/routes/:id` | 删除路由规则 |
| **Prompt 管理** | | |
| GET | `/api/prompts` | 所有 Agent 的 Prompt |
| GET | `/api/prompts/:agent_id` | 单个 Agent 的 Prompt |
| PUT | `/api/prompts/:agent_id` | 更新 Prompt（写 DB） |
| POST | `/api/prompts/:agent_id/reset` | 重置为配置文件默认值 |
| GET | `/api/prompts/:agent_id/history` | 历史版本 |
| **任务管理** | | |
| GET | `/api/tasks` | 任务列表（分页） |
| GET | `/api/tasks/:id` | 任务详情 |
| POST | `/api/tasks/:id/cancel` | 取消任务 |
| POST | `/api/tasks/:id/retry` | 重试任务 |
| **系统** | | |
| GET | `/api/config` | 获取配置 |
| PUT | `/api/config` | 更新配置 |
| POST | `/api/config/test-gitea` | 测试 Gitea 连接 |
| POST | `/api/config/test-llm` | 测试 LLM 连接 |
| GET | `/api/stats` | 统计数据 |
| GET | `/api/logs` | 操作日志 |

## 十一、部署方式

### 方式一：直接运行
```bash
./gitea-agent-gateway -config config.yaml
```

### 方式二：Docker
```dockerfile
FROM scratch
COPY gitea-agent-gateway /gateway
COPY config.yaml /config.yaml
EXPOSE 8080
ENTRYPOINT ["/gateway", "-config", "/config.yaml"]
```

### 方式三：systemd
```ini
[Unit]
Description=Gitea Agent Gateway
After=network.target

[Service]
Type=simple
ExecStart=/opt/gateway/gitea-agent-gateway -config /opt/gateway/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```
