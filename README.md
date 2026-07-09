# Gitea Agent Gateway

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue)](#license)
[![Tests](https://img.shields.io/badge/Tests-55+-brightgreen)](#测试)

AI Agent 网关 —— 通过 Gitea Webhook 事件驱动，将 AI Agent 嵌入 Gitea 工作流。支持多种 Agent 类型，通过 Tool-Use（Function Calling）与代码库交互，自动完成代码分析、审查、开发和修复任务。

## 功能特性

- 🤖 **多种 Agent 类型** —— 需求分析、代码审查、评论交互、Issue 开发、Bug 修复
- 🔧 **Tool-Use Agent** —— 基于 LLM Function Calling，通过 read_file / write_file / search_code / run_command / apply_diff 等工具理解和修改代码
- 🔒 **轻量级沙箱** —— 目录隔离 + 命令白名单 + 超时控制 + 审计日志（不依赖 Docker）
- 🎯 **可配置模板** —— 支持自定义 System Prompt 和 User Template，支持 Go template 语法
- 🌐 **Web UI** —— Vue 3 + Element Plus 管理界面，Dashboard / Agent 管理 / 任务列表 / Prompt 编辑
- 📡 **多 LLM 支持** —— OpenAI 兼容（DeepSeek / Qwen / Moonshot / Ollama）+ Anthropic Claude
- ⚙️ **灵活配置** —— Agent 级别 loop_config 覆盖（最大迭代、Token 限制、超时控制）

## 架构概览

```
Gitea Webhook → Handler (签名验证 + 去重)
  → Dispatcher (路由匹配 + 任务队列 + 并发执行)
    → Runner (Analyze / Review / Interaction / Dev / Bugfix)
      → Agent Loop (多轮 LLM 对话 + Tool Call)
    → 写回 Gitea (评论 / PR)
```

### 核心组件

| 包 | 职责 |
|---|------|
| `internal/webhook` | HTTP Handler、签名验证、事件解析、去重 |
| `internal/dispatcher` | Router（事件→Agent 匹配）、TaskQueue（SQLite 持久化）、Executor（并发控制） |
| `internal/agents` | Runner 实现：AnalyzeRunner、ReviewRunner、InteractionRunner、DevRunner、BugfixRunner |
| `internal/agent` | Tool-Use Agent Loop：ToolRegistry + 多轮对话循环 |
| `internal/llm` | Provider 接口 + OpenAI 兼容客户端 + Anthropic 客户端 |
| `internal/store` | SQLite 数据库（WAL 模式）、自动迁移、CRUD |
| `internal/sandbox` | 工作空间隔离、命令白名单、Git 操作、审计日志 |
| `internal/gitea` | Gitea API 客户端（Issue / PR / 评论 / 文件） |
| `internal/api` | 管理 REST API + JWT 认证 |
| `internal/config` | YAML 配置加载 + 环境变量展开 |

## 快速开始

### 前置条件

- Go 1.26+
- Node.js 18+（仅开发/构建前端时需要）
- Gitea 实例
- LLM API Key（DeepSeek / OpenAI / Claude 等）

### 1. 构建并启动

```bash
cp config.example.yaml config.yaml
# 可选：在 config.yaml 中预填 gitea / llm，也可全部通过 Web UI 配置

cd web && npm install && npm run build && cd ..
go build -o gateway .
./gateway -config config.yaml
```

启动后访问 **Web UI**：http://localhost:8080  
默认账号：`admin` / `admin123`

> `config.yaml` 可作为初始默认值；Web UI **系统配置**会优先显示数据库中的值，数据库未设置的项自动回退到 `config.yaml`。

### 2. Web UI 配置（推荐顺序）

按以下顺序在 Web UI 中完成首次配置：

| 步骤 | 页面 | 操作 |
|------|------|------|
| ① | 登录 | 使用 `admin` / `admin123` 登录 |
| ② | **系统配置 → Gitea 连接** | 填写 Gitea 地址、管理员 Token（需 `write:admin`）、Webhook 密钥 → 点击 **测试 Gitea 连接** → **保存全部** |
| ③ | **系统配置 → LLM 配置** | 填写 Provider JSON 与默认模型 → 点击 **测试 LLM 连接** → **保存全部** |
| ④ | **Agent 管理** | 新建 analyze / coder / review 三个 Agent，勾选目标仓库 |
| ⑤ | Gitea 仓库 | 将 Agent 用户加为协作者；配置 Webhook（见下文） |

**Gitea 管理员 Token 所需权限**：`write:admin`（创建 Agent 用户）、`write:repository`、`read:repository`。

### 3. 配置 Gitea Webhook

在 Gitea 仓库 **Settings → Webhooks** 添加：

| 项 | 值 |
|----|-----|
| URL | `http://<gateway-host>:8080/webhook/gitea` |
| Secret | 与系统配置中的 Webhook 密钥一致 |
| 事件 | Issues、Issue Comment、Pull Request、Pull Request Comment |

> 远程 Gitea 无法访问你本机的 `localhost`，需使用公网 IP、内网穿透，或将 Gateway 部署到 Gitea 同机。

### 4. 验证工作流

1. 在 Gitea 创建 Issue，Assign `analyze-agent` → 等待分析评论  
2. Assign `coder-agent` → 等待 PR 创建  
3. 在 PR 上 Request `review-agent` → 等待审查评论  

详细联调清单见 [docs/v2-gitea-integration-checklist.md](docs/v2-gitea-integration-checklist.md)。

---

### 备选：纯配置文件方式

若不使用 Web UI，可直接编辑 `config.yaml`：

```yaml
gitea:
  url: "https://your-gitea.example.com"
  admin_token: "${GITEA_ADMIN_TOKEN}"
  webhook_secret: "your-webhook-secret"

llm:
  providers:
    deepseek:
      base_url: "https://api.deepseek.com/v1"
      api_key: "${DEEPSEEK_API_KEY}"
```

配置支持环境变量展开：`${VAR_NAME}` 或 `${VAR_NAME:-default_value}`。

### 构建（仅后端）

```bash
go build -o gateway .
```

### 运行

```bash
./gateway -config config.yaml
```

- **健康检查**: http://localhost:8080/health

## 配置说明

完整配置参见 [config.example.yaml](config.example.yaml)，主要配置段：

| 配置段 | 说明 |
|--------|------|
| `server` | 监听地址和端口 |
| `gitea` | Gitea 连接信息（URL、管理员 Token、Webhook 密钥） |
| `workspace` | Agent 工作目录配置 |
| `dispatcher` | 并发数、重试、429 退避、队列大小（无全局任务超时） |
| `llm` | LLM Provider 与连通性默认（provider/model） |
| `agents` | Agent 默认预算（tokens/timeout/temperature）与 Loop 总超时 |
| `auth` | JWT 认证配置 |
| `api` | 管理 API 认证 Token |

### Agent LLM 预算与超时

```yaml
agents:
  defaults:
    max_output_tokens: 2048   # 每次调用输出上限（单次 + Loop 每轮共用）
    max_input_tokens: 8192    # 每次请求输入上限（含 tools；估算为字符数/4）
    temperature: 0.3
    timeout: "5m"             # 单次任务总超时（analyze/review/reply）
  loop:
    max_iterations: 20
    total_timeout: "30m"      # 仅多轮任务总超时（solve/fix_bug）
    iteration_interval: 3
```

任务超时由 Agent 配置控制（不再使用 `dispatcher.timeout`）。单个 Agent 可覆盖 `max_output_tokens` / `max_input_tokens` / `timeout` / `loop_config`。

## 开发

### 前后端分离开发

```bash
# 终端 1: 后端
go build -o gateway . && ./gateway -config config.yaml

# 终端 2: 前端（热更新）
cd web && npm run dev
```

前端开发服务器运行在 `http://localhost:3001`，API 请求自动代理到后端。

### 测试

```bash
# 全部测试
go test ./... -count=1

# 单元测试
go test ./internal/... -v -count=1

# 集成测试
go test ./tests/integration/ -v -count=1

# 单个包
go test ./internal/sandbox/ -v -count=1

# 覆盖率
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out
```

测试框架：`testify`（assert + require）。集成测试使用 `TestEnv` 提供内存 SQLite、Mock Gitea 和 Mock LLM。

### 代码质量

```bash
go fmt ./...
go vet ./...
```

## 项目结构

```
├── main.go                 # 入口：HTTP 服务器 + graceful shutdown
├── config.example.yaml     # 示例配置
├── internal/
│   ├── agent/              # Tool-Use Agent Loop + 工具定义
│   ├── agents/             # Runner 实现 + Manager + Registry
│   ├── api/                # 管理 REST API + 认证中间件
│   ├── auth/               # JWT + bcrypt
│   ├── config/             # YAML 配置加载 + 环境变量展开
│   ├── dispatcher/         # TaskQueue + Executor + v2 流水线
│   ├── workflow/           # Event Resolver + 状态机 + 门禁 + Session + 生命周期
│   ├── gitea/              # Gitea API 客户端
│   ├── llm/                # LLM Provider 接口 + 实现
│   ├── sandbox/            # 沙箱（目录隔离 + 命令执行 + Git 操作）
│   ├── store/              # SQLite 数据库 + 自动迁移
│   └── webhook/            # Webhook HTTP Handler
├── web/                    # Vue 3 + Element Plus 前端
├── tests/                  # 集成测试
├── docs/                   # 设计文档
└── scripts/                # 工具脚本
```

## Agent 角色（v2 Assign 模型）

在 Gateway 中注册多个功能性 Agent，每个 Agent 设置 `role` 并在 Gitea 上作为协作者：

| role | 触发方式 | 说明 |
|------|----------|------|
| `analyze` | Issue 上 **Assign** analyze Agent | 需求/Bug 分析，输出评论报告 |
| `coder` | Issue 上 **Assign** coder Agent | 实现或修复（Issue 带 `bug` 标签时用 fix 系 Prompt），提 PR |
| `review` | PR 上 **Request Reviewer** review Agent | 代码审查，输出审查评论 |

**续作**：在 Issue/PR 评论中 **@Agent用户名**；`/dev`、`/reply`、`/force` 控制行为。  
**重置**：评论 `/gateway reset` 或 `POST /api/sessions/reset?repo=&issue=`。

> v2 已弃用 `ai:analyze` / `ai:solve` 等 Label 触发及 routes 配置。迁移见 [设计文档 §11.2](docs/trigger-rules-and-workflow-improvement.md#112-从-label-触发迁移到-assign)。

## 文档

- [Agent 开发决策](docs/agent-development-decisions.md)
- [任务清单](docs/TASKS.md)
- [Assign 工作流 v2 设计](docs/trigger-rules-and-workflow-improvement.md)
- [v2 完成总览](docs/assign-workflow-progress.md)
- [Web UI 设计](docs/web-ui-design.md)
- [部署指南](docs/DEPLOYMENT.md)
- [测试指南](scripts/TESTING.md)

## License

MIT
