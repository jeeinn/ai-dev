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
- Node.js 18+（仅开发前端时需要）
- Gitea 实例（需创建 Webhook）
- LLM API Key（DeepSeek / OpenAI / Claude 等）

### 1. 配置

```bash
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，至少配置以下内容：

```yaml
gitea:
  url: "https://your-gitea.example.com"
  admin_token: "your-admin-token"        # Gitea 管理员 Token
  webhook_secret: "your-webhook-secret"  # Webhook 签名密钥

llm:
  providers:
    deepseek:
      base_url: "https://api.deepseek.com/v1"
      api_key: "sk-xxx"
```

配置支持环境变量展开：`${VAR_NAME}` 或 `${VAR_NAME:-default_value}`。

### 2. 构建

```bash
# 仅构建后端
go build -o gateway .

# 构建含前端的完整版本（需要先构建前端）
cd web && npm install && npm run build && cd ..
go build -o gateway .
```

### 3. 运行

```bash
./gateway -config config.yaml
```

启动后访问：
- **Web UI**: http://localhost:8080
- **默认账号**: `admin` / `admin123`
- **健康检查**: http://localhost:8080/health

### 4. 配置 Gitea Webhook

在 Gitea 仓库设置中添加 Webhook：
- **URL**: `http://your-server:8080/webhook/gitea`
- **Secret**: 与 `config.yaml` 中的 `webhook_secret` 一致
- **触发事件**: Issues、Pull Request、Issue Comment

## 配置说明

完整配置参见 [config.example.yaml](config.example.yaml)，主要配置段：

| 配置段 | 说明 |
|--------|------|
| `server` | 监听地址和端口 |
| `gitea` | Gitea 连接信息（URL、管理员 Token、Webhook 密钥） |
| `workspace` | Agent 工作目录配置 |
| `dispatcher` | 并发数、重试、超时、队列大小 |
| `llm` | LLM Provider 配置（可配多个） |
| `agents` | Agent 模板、默认参数、loop 配置 |
| `auth` | JWT 认证配置 |
| `api` | 管理 API 认证 Token |

### Agent Loop 配置

Agent 的迭代控制支持全局默认和单 Agent 覆盖：

```yaml
agents:
  loop:
    max_iterations: 20      # 最大迭代轮次
    max_tokens: 4096        # 单次 LLM 最大 tokens
    timeout: "5m"           # 单轮超时
    total_timeout: "30m"    # 总超时
```

单个 Agent 可通过 API 设置 `loop_config` 字段覆盖全局配置。

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
│   ├── dispatcher/         # Router + TaskQueue + Executor
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

## Agent 类型

| 类型 | 触发方式 | 说明 |
|------|----------|------|
| `analyze` | Issue 添加 `ai:analyze` 标签 | 需求分析，输出可行性报告 |
| `review` | PR 添加 `ai:review` 标签 | 代码审查，输出审查报告 |
| `interaction` | @提及 Agent 用户名 | 评论互动，上下文回复 |
| `solve` | Issue 添加 `ai:solve` 标签 | Tool-Use Agent 读 Issue → 写代码 → 提 PR |
| `fix` | Issue 添加 `ai:fix` 标签 | Tool-Use Agent 定位 Bug → 修复 → 提 PR |

## 文档

- [Agent 开发决策](docs/agent-development-decisions.md)
- [任务清单](docs/TASKS.md)
- [Web UI 设计](docs/web-ui-design.md)
- [部署指南](docs/DEPLOYMENT.md)
- [测试指南](scripts/TESTING.md)

## License

MIT
