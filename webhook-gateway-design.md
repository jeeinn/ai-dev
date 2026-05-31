# Webhook Gateway 详细设计

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
│   │   └── signature.go       # HMAC 签名验证
│   ├── dispatcher/            # Agent 调度器
│   │   ├── router.go          # 标签→Agent 路由
│   │   ├── queue.go           # 任务队列
│   │   └── executor.go        # Agent 执行器
│   ├── agents/                # Agent 实现
│   │   ├── interface.go       # Agent 接口定义
│   │   ├── analyze.go         # 需求分析 Agent
│   │   ├── dev.go             # 研发 Agent
│   │   ├── bugfix.go          # Bug 修复 Agent
│   │   ├── review.go          # PR 审查 Agent
│   │   └── test.go            # 测试 Agent
│   ├── gitea/                 # Gitea API 客户端
│   │   ├── client.go          # API 封装
│   │   ├── issue.go           # Issue 操作
│   │   ├── pr.go              # PR 操作
│   │   └── repo.go            # 仓库操作
│   ├── llm/                   # LLM 调用抽象
│   │   ├── provider.go        # Provider 接口
│   │   ├── openai.go          # OpenAI 兼容接口
│   │   └── anthropic.go       # Claude API
│   ├── config/                # 配置管理
│   │   ├── config.go          # 配置加载
│   │   └── schema.go          # 配置结构定义
│   ├── store/                 # 数据存储
│   │   ├── sqlite.go          # SQLite 初始化
│   │   ├── task.go            # 任务记录
│   │   └── log.go             # 操作日志
│   └── api/                   # 管理 API
│       ├── router.go          # API 路由
│       ├── config.go          # 配置管理接口
│       ├── task.go            # 任务查询接口
│       └── stats.go           # 统计接口
├── web/                       # 前端源码
│   ├── src/
│   │   ├── App.vue
│   │   ├── views/
│   │   │   ├── Dashboard.vue      # 仪表盘
│   │   │   ├── Config.vue         # 可视化配置
│   │   │   ├── Tasks.vue          # 任务列表
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
│  签名验证        │ ← HMAC-SHA256
│  事件类型过滤    │ ← 只处理 issues/PR/push
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  事件解析        │ ← 提取 repo/issue/label/action
│  标签匹配        │ ← 匹配 agent:* 标签
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
│  调用 LLM        │ ← 生成代码/分析/审查
│  回写 Gitea      │ ← 评论/打标签/创建 PR
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

    // 2. 解析事件类型
    event := r.Header.Get("X-Gitea-Event")
    payload, _ := io.ReadAll(r.Body)

    // 3. 根据事件类型解析
    switch event {
    case "issues":
        handleIssuesEvent(payload)
    case "pull_request":
        handlePREvent(payload)
    case "issue_comment":
        handleCommentEvent(payload)
    default:
        w.WriteHeader(200) // 忽略不关心的事件
        return
    }

    // 4. 立即返回 200（异步处理）
    w.WriteHeader(200)
    json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}
```

## 五、路由规则设计

```yaml
# config.yaml 中的路由配置
routes:
  # Issue 被打标签时触发
  - event: "issues"
    action: "label"
    match:
      label: "agent:analyze"
    agent: "analyze"
    priority: 1

  - event: "issues"
    action: "label"
    match:
      label: "agent:dev"
    agent: "dev"
    priority: 2

  - event: "issues"
    action: "label"
    match:
      label: "agent:bugfix"
    agent: "bugfix"
    priority: 2

  # PR 创建/更新时自动触发审查
  - event: "pull_request"
    action: ["opened", "synchronize"]
    match:
      issue_labels: ["agent:dev", "agent:bugfix"]
    agent: "review"
    priority: 3
```

## 六、可视化配置界面

### 6.1 仪表盘

- 任务统计：待处理/执行中/已完成/失败
- 最近 10 条任务记录
- Agent 执行成功率图表
- 系统状态（Gitea 连接、LLM 连接）

### 6.2 配置页面

分 Tab 管理：

**Tab 1: Gitea 连接**
```
┌──────────────────────────────────────┐
│ Gitea URL:  [https://git.example.com]│
│ API Token:  [••••••••••••] [测试连接] │
│ Webhook密钥: [••••••••••••]          │
│ 状态: ● 已连接                        │
└──────────────────────────────────────┘
```

**Tab 2: Agent 配置**
```
┌──────────────────────────────────────────────────┐
│ Agent: [需求分析 ▼]                               │
│                                                  │
│ 模型:        [claude-sonnet-4-20250514 ▼]                │
│ API地址:     [https://api.openai.com/v1]          │
│ API密钥:     [••••••••••••]                       │
│ 最大Token:   [4096]                               │
│ 温度:        [0.3]                                │
│ 系统提示词:   [________________] [编辑]            │
│                                                  │
│ [保存] [测试调用]                                 │
└──────────────────────────────────────────────────┘
```

**Tab 3: 路由规则**
```
┌────────────────────────────────────────────────────────────┐
│ [+ 添加规则]                                                │
│                                                            │
│ # │ 事件        │ 动作    │ 匹配标签        │ Agent │ 优先级│
│ 1 │ issues      │ label   │ agent:analyze   │ 需求分析│ 1    │
│ 2 │ issues      │ label   │ agent:dev       │ 研发   │ 2    │
│ 3 │ issues      │ label   │ agent:bugfix    │ Bug修复│ 2    │
│ 4 │ pull_request│ opened  │ agent:dev,bugfix│ PR审查 │ 3    │
│                                                            │
│ [编辑] [删除] [上移] [下移]                                  │
└────────────────────────────────────────────────────────────┘
```

**Tab 4: Webhook 接收地址**
```
┌──────────────────────────────────────────────────┐
│ Webhook 接收地址:                                 │
│ http://your-server:8080/webhook/gitea            │
│ [复制地址]                                       │
│                                                  │
│ 请在 Gitea 仓库设置中添加此 Webhook 地址:        │
│ 1. 进入仓库 → 设置 → Webhooks → 添加 Webhook    │
│ 2. 目标 URL 填写上方地址                          │
│ 3. 密钥填写配置中的 Webhook 密钥                  │
│ 4. 选择事件: Issues, Pull Request, Push          │
└──────────────────────────────────────────────────┘
```

## 七、任务队列设计

```go
// 任务状态机
type TaskStatus string

const (
    TaskStatusPending   TaskStatus = "pending"    // 待处理
    TaskStatusRunning   TaskStatus = "running"    // 执行中
    TaskStatusSuccess   TaskStatus = "success"    // 成功
    TaskStatusFailed    TaskStatus = "failed"     // 失败
    TaskStatusCancelled TaskStatus = "cancelled"  // 取消
)

// 任务记录
type Task struct {
    ID        int64      `json:"id"`
    Event     string     `json:"event"`      // issues / pull_request
    Repo      string     `json:"repo"`       // owner/repo
    IssueID   int64      `json:"issue_id"`   // Issue 或 PR 编号
    Agent     string     `json:"agent"`      // analyze / dev / bugfix / review
    Status    TaskStatus `json:"status"`
    Priority  int        `json:"priority"`
    CreatedAt time.Time  `json:"created_at"`
    StartedAt *time.Time `json:"started_at"`
    FinishedAt *time.Time `json:"finished_at"`
    Result    string     `json:"result"`     // Agent 输出摘要
    Error     string     `json:"error"`      // 失败原因
}
```

### 并发控制

```go
// dispatcher/executor.go
type Executor struct {
    maxConcurrent int               // 最大并发数
    sem           chan struct{}      // 信号量
    queue         *TaskQueue         // 任务队列
}

func (e *Executor) Start() {
    for i := 0; i < e.maxConcurrent; i++ {
        go e.worker()
    }
}

func (e *executor) worker() {
    for task := range e.queue.Dequeue() {
        e.sem <- struct{}{}        // 获取信号量
        e.execute(task)            // 执行任务
        <-e.sem                    // 释放信号量
    }
}
```

## 八、配置文件格式

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 8080

gitea:
  url: "https://git.example.com"
  token: "your-gitea-token"
  webhook_secret: "your-webhook-secret"

llm:
  default_provider: "openai"
  providers:
    openai:
      base_url: "https://api.openai.com/v1"
      api_key: "${OPENAI_API_KEY}"
    anthropic:
      api_key: "${ANTHROPIC_API_KEY}"

agents:
  analyze:
    provider: "openai"
    model: "claude-sonnet-4-20250514"
    max_tokens: 4096
    temperature: 0.3
    system_prompt: |
      你是一个需求分析专家...

  dev:
    provider: "openai"
    model: "claude-sonnet-4-20250514"
    max_tokens: 8192
    temperature: 0.2
    system_prompt: |
      你是一个高级研发工程师...

  bugfix:
    provider: "openai"
    model: "claude-sonnet-4-20250514"
    max_tokens: 8192
    temperature: 0.1

  review:
    provider: "openai"
    model: "claude-sonnet-4-20250514"
    max_tokens: 4096
    temperature: 0.2

dispatcher:
  max_concurrent: 3
  retry_count: 2
  timeout: 300
  queue_size: 100

database:
  path: "./data/gateway.db"

logging:
  level: "info"
  path: "./data/logs"
```

## 九、部署方式

### 方式一：直接运行
```bash
# 下载二进制
wget https://release.example.com/gitea-agent-gateway-linux-amd64
chmod +x gitea-agent-gateway-linux-amd64

# 准备配置
cp config.example.yaml config.yaml
# 编辑 config.yaml

# 运行
./gitea-agent-gateway-linux-amd64 -config config.yaml
```

### 方式二：Docker
```dockerfile
FROM scratch
COPY gitea-agent-gateway /gateway
COPY config.yaml /config.yaml
EXPOSE 8080
ENTRYPOINT ["/gateway", "-config", "/config.yaml"]
```

### 方式三：systemd 服务
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

## 十、API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/webhook/gitea` | Gitea Webhook 接收 |
| GET | `/api/tasks` | 任务列表（分页） |
| GET | `/api/tasks/:id` | 任务详情 |
| POST | `/api/tasks/:id/cancel` | 取消任务 |
| POST | `/api/tasks/:id/retry` | 重试任务 |
| GET | `/api/config` | 获取配置 |
| PUT | `/api/config` | 更新配置 |
| POST | `/api/config/test-gitea` | 测试 Gitea 连接 |
| POST | `/api/config/test-llm` | 测试 LLM 连接 |
| GET | `/api/stats` | 统计数据 |
| GET | `/api/logs` | 操作日志 |
