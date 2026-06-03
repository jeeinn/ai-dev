# Gitea Agent Gateway

AI Agent 网关，通过 Webhook 事件驱动，将 AI Agent 嵌入 Gitea 工作流。

## 功能特性

- 🤖 **多种 Agent 类型**：分析、审查、交互、开发、修复
- 🔧 **Tool-Use Agent**：通过 Function Calling 理解和修改代码
- 🔒 **轻量级沙箱**：目录隔离 + 命令白名单 + 超时控制
- 🎯 **可配置模板**：支持自定义 Prompt 模板
- 🌐 **Web UI**：Vue 3 + Element Plus 管理界面

## 快速开始

### 1. 配置

```bash
cp config.example.yaml config.yaml
# 编辑 config.yaml，配置 Gitea URL、LLM Provider 等
```

### 2. 构建运行

```bash
# 构建
go build -o gateway.exe .

# 运行
./gateway.exe -config config.yaml
```

### 3. 访问

- **Web UI**: http://localhost:8080
- **默认账号**: admin / admin123

## 开发模式

前后端分离开发，支持热更新：

```bash
# 终端 1: 启动后端
./gateway.exe -config config.yaml

# 终端 2: 启动前端开发服务器
cd web
npm run dev
```

访问 `http://localhost:3001`，前端修改自动热更新，API 请求自动代理到后端。

## 测试

```bash
# 运行所有测试
go test ./... -count=1

# 运行单元测试
go test ./internal/... -v -count=1

# 运行集成测试
go test ./tests/integration/ -v -count=1
```

## 项目结构

```
├── internal/
│   ├── agent/        # Tool-Use Agent (Agent Loop + Tools)
│   ├── agents/       # Runner 实现 (Analyze/Review/Dev/Bugfix)
│   ├── api/          # 管理 API + 认证
│   ├── auth/         # JWT + 密码哈希
│   ├── config/       # 配置加载
│   ├── dispatcher/   # Router + Queue + Executor
│   ├── gitea/        # Gitea API 客户端
│   ├── llm/          # LLM Provider (OpenAI/Anthropic)
│   ├── sandbox/      # 轻量级沙箱
│   ├── store/        # SQLite 数据库
│   └── webhook/      # Webhook 处理
├── web/              # Vue 3 前端
├── docs/             # 设计文档
└── tests/            # 集成测试
```

## 文档

- [Agent 开发决策](docs/agent-development-decisions.md)
- [任务清单](docs/TASKS.md)
- [Web UI 设计](docs/web-ui-design.md)
- [测试指南](scripts/TESTING.md)

## License

MIT
