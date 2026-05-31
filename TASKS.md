# 任务执行文档

> 每完成一个任务打勾 `[x]`，每完成一个模块提交一次代码。

## Phase 1：项目骨架（模块 1）

- [x] 1.1 Go 环境安装（Go 1.26.3）
- [x] 1.2 go mod init（gitea-agent-gateway）
- [ ] 1.3 目录结构创建（internal/、web/、config/）
- [ ] 1.4 main.go 入口（HTTP 服务器 + graceful shutdown）
- [ ] 1.5 config 模块（config.yaml 加载 + 结构定义）
- [ ] 1.6 store 模块（SQLite 初始化 + 表创建）

## Phase 2：Webhook 接收（模块 2）

- [ ] 2.1 webhook/handler.go — HTTP Handler
- [ ] 2.2 webhook/signature.go — HMAC-SHA256 签名验证
- [ ] 2.3 webhook/dedup.go — X-Gitea-Delivery 幂等去重
- [ ] 2.4 webhook/parser.go — 事件解析（issues/PR/comment）
- [ ] 2.5 路由注册（POST /webhook/gitea）

## Phase 3：Gitea API 客户端（模块 3）

- [ ] 3.1 gitea/client.go — HTTP 客户端封装
- [ ] 3.2 gitea/admin.go — Admin API（创建用户、生成 Token）
- [ ] 3.3 gitea/issue.go — Issue 操作（评论、打标签、获取详情）
- [ ] 3.4 gitea/pr.go — PR 操作（创建、评论、获取 Diff）
- [ ] 3.5 gitea/repo.go — 仓库操作（获取文件、克隆地址）

## Phase 4：Agent 管理（模块 4）

- [ ] 4.1 store/agent.go — Agent CRUD（SQLite）
- [ ] 4.2 store/route.go — 路由规则 CRUD（SQLite）
- [ ] 4.3 agents/manager.go — Agent 创建（含 Gitea 账号自动注册）
- [ ] 4.4 agents/registry.go — 运行时 Agent 注册表
- [ ] 4.5 agents/interaction.go — @Mention 互动处理

## Phase 5：LLM 调用层（模块 5）

- [ ] 5.1 llm/provider.go — Provider 接口定义
- [ ] 5.2 llm/openai.go — OpenAI 兼容 Provider
- [ ] 5.3 llm/anthropic.go — Anthropic Provider
- [ ] 5.4 llm/registry.go — Provider 注册表

## Phase 6：Dispatcher + Agent 执行（模块 6）

- [ ] 6.1 dispatcher/router.go — Label+Assignee 双条件路由
- [ ] 6.2 dispatcher/queue.go — 任务队列（内存 + SQLite 持久化）
- [ ] 6.3 dispatcher/executor.go — Agent 执行器（并发控制）
- [ ] 6.4 store/task.go — 任务记录 CRUD
- [ ] 6.5 store/log.go — 操作日志

## Phase 7：核心 Agent 实现（模块 7）

- [ ] 7.1 agents/base.go — Agent 基础逻辑（clone/分支/推送/创建PR）
- [ ] 7.2 agents/analyze.go — 需求分析 Agent
- [ ] 7.3 agents/dev.go — 研发 Agent
- [ ] 7.4 agents/bugfix.go — Bug 修复 Agent
- [ ] 7.5 agents/review.go — PR 审查 Agent
- [ ] 7.6 agents/interaction_agent.go — @Mention 互动 Agent

## Phase 8：Prompt 管理（模块 8）

- [ ] 8.1 store/prompt.go — Prompt 历史版本存储
- [ ] 8.2 agents/prompt.go — Prompt 加载（DB > config.yaml）

## Phase 9：管理 API（模块 9）

- [ ] 9.1 api/router.go — API 路由 + 中间件
- [ ] 9.2 api/agent.go — Agent CRUD 接口
- [ ] 9.3 api/task.go — 任务查询/取消/重试
- [ ] 9.4 api/prompt.go — Prompt 管理接口
- [ ] 9.5 api/stats.go — 统计数据
- [ ] 9.6 api/config.go — 配置管理接口

## Phase 10：前端 Web UI（模块 10）

- [ ] 10.1 Vue 3 + Element Plus 项目初始化
- [ ] 10.2 Dashboard 仪表盘
- [ ] 10.3 Agent 管理页面（创建/编辑/列表）
- [ ] 10.4 任务列表页面
- [ ] 10.5 Prompt 编辑页面
- [ ] 10.6 系统配置页面
- [ ] 10.7 go:embed 打包前端资源

## Phase 11：集成测试 + 收尾（模块 11）

- [ ] 11.1 config.example.yaml 示例配置
- [ ] 11.2 README.md 项目说明
- [ ] 11.3 Dockerfile
- [ ] 11.4 端到端测试（手动）
- [ ] 11.5 编译验证（go build）

---

## 进度追踪

| 模块 | 状态 | 开始时间 | 完成时间 |
|------|------|----------|----------|
| 模块 1：项目骨架 | 进行中 | 2026-06-01 | - |
| 模块 2：Webhook 接收 | 未开始 | - | - |
| 模块 3：Gitea API | 未开始 | - | - |
| 模块 4：Agent 管理 | 未开始 | - | - |
| 模块 5：LLM 调用层 | 未开始 | - | - |
| 模块 6：Dispatcher | 未开始 | - | - |
| 模块 7：核心 Agent | 未开始 | - | - |
| 模块 8：Prompt 管理 | 未开始 | - | - |
| 模块 9：管理 API | 未开始 | - | - |
| 模块 10：前端 Web UI | 未开始 | - | - |
| 模块 11：集成收尾 | 未开始 | - | - |
