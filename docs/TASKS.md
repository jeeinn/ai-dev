# 任务执行文档

> 每完成一个任务打勾 `[x]`，每完成一个模块提交一次代码。

## Phase 1：项目骨架（模块 1）✅

- [x] 1.1 Go 环境安装（Go 1.26.3）
- [x] 1.2 go mod init（gitea-agent-gateway）
- [x] 1.3 目录结构创建（internal/、web/、cmd/）
- [x] 1.4 main.go 入口（HTTP 服务器 + graceful shutdown）
- [x] 1.5 config 模块（config.yaml 加载 + 结构定义 + 环境变量展开 + 默认值）
- [x] 1.6 store 模块（SQLite 初始化 + 6 张表自动建表）

## Phase 2：Webhook 接收（模块 2）✅

- [x] 2.1 webhook/handler.go — HTTP Handler（签名验证 + 去重 + 异步回调）
- [x] 2.2 webhook/signature.go — HMAC-SHA256 签名验证（X-Gitea-Signature）
- [x] 2.3 webhook/dedup.go — X-Gitea-Delivery 幂等去重（SQLite 存储）
- [x] 2.4 webhook/parser.go — 事件解析（issues/PR/comment + HasLabel/HasAssignee/HasMention）
- [x] 2.5 路由注册（POST /webhook/gitea → main.go 集成）

## Phase 3：Gitea API 客户端（模块 3）✅

- [x] 3.1 gitea/client.go — HTTP 客户端封装（token 认证 + JSON 请求/响应）
- [x] 3.2 gitea/admin.go — Admin API（创建用户 AdminCreateUser + 生成 Token AdminCreateToken）
- [x] 3.3 gitea/issue.go — Issue 操作（评论 + 添加/移除标签 + 获取详情）
- [x] 3.4 gitea/pr.go — PR 操作（创建 PR + 评论 + 获取详情 + 获取 Diff）
- [x] 3.5 gitea/repo.go — 仓库操作（获取仓库信息 + 获取文件内容）

## Phase 4：Agent 管理（模块 4）✅

- [x] 4.1 store/agent.go — Agent CRUD（Create/Get/GetByGiteaUsername/List/Update/Delete）
- [x] 4.2 store/route.go — 路由规则 CRUD（Create/List/Update/Delete）
- [x] 4.3 agents/manager.go — Agent 创建（含 Gitea 账号自动注册 + Token 自动生成）
- [x] 4.4 agents/registry.go — 运行时 Agent 注册表（按 ID/Username 查询 + LoadFromDB）
- [x] 4.5 agents/interaction.go — @Mention 互动处理（提取 @username + 触发回调）

## Phase 5：LLM 调用层（模块 5）✅

- [x] 5.1 llm/provider.go — Provider 接口定义（ChatCompletion + Function Calling）
- [x] 5.2 llm/openai.go — OpenAI 兼容 Provider（覆盖 DeepSeek/Qwen/Zhipu/Moonshot/Ollama）
- [x] 5.3 llm/anthropic.go — Anthropic Provider（Claude API，system 单独传）
- [x] 5.4 llm/registry.go — Provider 注册表（根据配置自动创建 Provider 实例）
- [x] 5.5 推理模型支持（reasoning_content 字段）
- [x] 5.6 Function Calling 支持（Tool/ToolCall/Function 类型）

## Phase 6：Dispatcher + Agent 执行（模块 6）✅

- [x] 6.1 dispatcher/router.go — Label+Assignee 双条件路由（事件/动作/标签/分配人/提及匹配）
- [x] 6.2 dispatcher/queue.go — 任务队列（内存 channel + SQLite 持久化 + 启动加载 pending 任务 + 后台扫描）
- [x] 6.3 dispatcher/executor.go — Agent 执行器（信号量并发控制 + 超时 + 重试 + LLM 调用）
- [x] 6.4 store/task.go — 任务记录 CRUD（Create/UpdateStatus/Get/List/ListPending）
- [x] 6.5 store/log.go — 操作日志（LogOperation + ListOperationLogs）
- [x] 6.6 dispatcher/dispatcher.go — Dispatcher 组合（Router + TaskQueue + Executor + Label 任务类型）
- [x] 6.7 结果回写到 Gitea Issue/PR 评论
- [x] 6.8 基于 Label 的任务类型判断（ai:solve → solve_issue, ai:fix → fix_bug）

## Phase 7：核心 Agent 实现（模块 7）✅

### 7.1 只读型 Agent ✅

- [x] 7.1.1 gitea/pr.go — 获取 PR Diff API (PRDiff, PRFiles)
- [x] 7.1.2 gitea/pr.go — 获取 Issue/PR 评论历史 API (IssueComments)
- [x] 7.1.3 agents/runners.go — ReviewRunner（获取 Diff → LLM 审查 → 评论报告）
- [x] 7.1.4 agents/runners.go — InteractionRunner（加载评论上下文 → LLM 回复）
- [x] 7.1.5 dispatcher/template.go — 上下文拼装逻辑（Go template 渲染）

### 7.2 写入型 Agent ✅

- [x] 7.2.1 sandbox/sandbox.go — 轻量级沙箱（目录隔离 + 命令白名单 + 超时控制 + 输出限制）
- [x] 7.2.2 sandbox/sandbox.go — 命令执行器（白名单验证 + 输出捕获 + 资源限制）
- [x] 7.2.3 sandbox/git.go — Git 操作封装（clone/branch/commit/push + 分支限制）
- [x] 7.2.4 sandbox/sandbox.go — 工作目录管理（创建/清理）
- [x] 7.2.5 sandbox/audit.go — 命令审计日志（记录所有执行的命令和输出）
- [x] 7.2.6 agents/runners.go — DevRunner（读 Issue → 分析 → 写代码 → 提 PR）
- [x] 7.2.7 agents/runners.go — BugfixRunner（读 Bug Issue → 定位 → 修复 → 提 PR）

### 7.3 安全限制 ✅

- [x] 7.3.1 命令白名单：git, sh, bash, go, python, node, npm, make, cargo 等
- [x] 7.3.2 命令黑名单：rm, dd, mkfs, shutdown, reboot 等
- [x] 7.3.3 分支限制：只能 push 到 `ai/*` 分支，不允许 push 到默认分支
- [x] 7.3.4 分支名验证：ValidateBranchName + GenerateBranchName
- [x] 7.3.5 工作目录限制：只能在 `workspace/{task_id}/` 内操作

### 7.4 Tool-Use Agent (v0.3.1) ✅

- [x] 7.4.1 agent/tools.go — Tool 定义与注册（ToolDef, ToolRegistry）
- [x] 7.4.2 agent/tools.go — 6 个基础工具实现
  - [x] read_file — 读取文件内容
  - [x] write_file — 写入/创建文件
  - [x] list_files — 列出目录结构
  - [x] search_code — 搜索代码内容 (grep)
  - [x] run_command — 执行命令 (受限)
  - [x] apply_diff — 应用 Diff 补丁
- [x] 7.4.3 agent/loop.go — Agent Loop 核心逻辑（多轮对话 + Tool Call 执行）
- [x] 7.4.4 agent/context.go — 代码库上下文加载（目录结构 + 关键文件 + Token 限制）
- [x] 7.4.5 agents/runners.go — DevRunner/BugfixRunner 改用 Agent Loop

## Phase 8：Prompt 管理（模块 8）⬜

- [ ] 8.1 store/prompt.go — Prompt 历史版本存储（prompt_history 表 CRUD）
- [ ] 8.2 agents/prompt.go — Prompt 加载（DB > config.yaml > 内置兜底）
- [ ] 8.3 Prompt API 和历史版本管理

## Phase 9：管理 API（模块 9）✅

- [x] 9.1 api/router.go — API 路由注册（RESTful 风格，Go 1.22 method patterns）
- [x] 9.2 api/agent.go — Agent CRUD 接口（GET/POST/PUT/DELETE /api/agents）
- [x] 9.3 api/task.go — 任务查询接口（GET /api/tasks, GET /api/tasks/{id}）
- [x] 9.4 api/route.go — 路由规则接口（GET/POST/DELETE /api/routes）
- [x] 9.5 api/stats.go — 统计数据（GET /api/stats）
- [x] 9.6 api/log.go — 操作日志（GET /api/logs）
- [x] 9.7 api/auth.go — API 认证（Bearer Token 中间件）
- [x] 9.8 API 响应隐藏 gitea_token（使用 AgentDTO）
- [x] 9.9 /api/templates 端点（查看可用模板）

## Phase 10：配置化模板（模块 10）✅

- [x] 10.1 config/schema.go — AgentsConfig, AgentTemplateConfig 结构定义
- [x] 10.2 dispatcher/template.go — Go template 渲染引擎
- [x] 10.3 config.example.yaml — 预置 analyze/review/reply 三种模板
- [x] 10.4 模板优先级：Agent 自定义 > config 模板 > 默认构建器
- [x] 10.5 模板变量支持：Issue, PR, Comment, Repo, Sender

## Phase 11：队列可靠性（模块 11）✅

- [x] 11.1 dispatcher/queue.go — pending task 后台扫描机制（每 60 秒扫描一次）
- [x] 11.2 store/task.go — stale running task 恢复（超过 10 分钟重置为 pending）
- [x] 11.3 store/task.go — Task 状态更新时间修复（started_at/finished_at）
- [x] 11.4 dispatcher/queue.go — 队列满时策略（DB 后台扫描兜底）

## Phase 12：前端 Web UI（模块 12）⬜（可选）

- [ ] 12.1 Vue 3 + Element Plus 项目初始化
- [ ] 12.2 Dashboard 仪表盘（任务统计、成功率、系统状态）
- [ ] 12.3 Agent 管理页面（创建/编辑/列表/启用禁用）
- [ ] 12.4 任务列表页面（查看/取消/重试）
- [ ] 12.5 Prompt 编辑页面（System Prompt + User Template + 历史版本）
- [ ] 12.6 系统配置页面（Gitea 连接、LLM Provider、路由规则）
- [ ] 12.7 go:embed 打包前端资源

## Phase 13：集成测试 + 收尾（模块 13）🔶

- [x] 13.1 config.example.yaml 示例配置
- [ ] 13.2 README.md 项目说明
- [ ] 13.3 部署文档
- [x] 13.4 端到端测试完善
  - [x] testify 集成测试框架
  - [x] 单元测试 (41 tests)
  - [x] 集成测试 (14 tests)
  - [x] 端到端测试 (Issue → AI 分析, PR → AI 审查, Issue → Tool-Use Agent)
- [x] 13.5 编译验证（go build，单二进制）
- [ ] 13.6 性能测试和优化

---

## 进度追踪

| 模块 | 状态 | 提交 |
|------|------|------|
| 模块 1：项目骨架 | ✅ 完成 | `2967349` |
| 模块 2：Webhook 接收 | ✅ 完成 | `0dceb5d` |
| 模块 3：Gitea API | ✅ 完成 | `3ff6482` |
| 模块 4：Agent 管理 | ✅ 完成 | `e89aa9e` |
| 模块 5：LLM 调用层 | ✅ 完成 | `bc24087` |
| 模块 6：Dispatcher | ✅ 完成 | `1957a1b` |
| 模块 7：核心 Agent | ✅ 完成 | `5dd518d` |
| 模块 8：Prompt 管理 | ⬜ 未开始 | - |
| 模块 9：管理 API | ✅ 完成 | `d4c48fb` |
| 模块 10：配置化模板 | ✅ 完成 | `2f74746` |
| 模块 11：队列可靠性 | ✅ 完成 | `b0fefd9` |
| 模块 12：前端 Web UI | ⬜ 未开始（可选） | - |
| 模块 13：集成收尾 | 🔶 部分完成 | - |

**总体进度：10/13 模块完成（77%）**

---

## 版本历史

### v0.1：项目骨架 ✅

```
基础模块搭建
```

### v0.2：只读型 Agent ✅

```
tag: v0.2
提交: 2d378bd

完成内容:
├── PR Diff API
├── 评论历史 API
├── ReviewRunner
├── InteractionRunner
├── 队列可靠性增强
└── 集成测试框架 (testify)
```

### v0.3：写入型 Agent 基础 ✅

```
tag: v0.3
提交: b0fefd9

完成内容:
├── 轻量级沙箱 (目录隔离 + 命令白名单 + 超时控制)
├── Git 操作封装 (clone/branch/commit/push)
├── 命令执行器
├── 命令审计日志
├── DevRunner / BugfixRunner (基础版)
└── 集成测试框架 (testify)
```

### v0.3.1：Go 原生 Tool-Use Agent ✅

```
tag: v0.3.1
提交: 5dd518d

完成内容:
├── LLM Function Calling 支持
├── 6 个基础工具 (read_file, write_file, list_files, search_code, run_command, apply_diff)
├── Agent Loop 多轮对话
├── 代码库上下文加载
├── DevRunner / BugfixRunner 改造
└── 端到端测试验证通过
```

### v0.4：增强与优化（下一步）

```
优先级: P1
预计工作量: 2-3 周
目标: 增强 Agent 能力，提升可靠性

任务:
├── 多文件修改支持 (结构化输出解析)
├── 测试生成与验证
├── 代码审查集成
├── Token 使用统计
├── 执行过程可视化
└── 可选: 集成 Aider 处理复杂场景
```

### v0.5+：长期演进

```
优先级: P2+
预计工作量: 持续迭代

方向:
├── 多 Agent 协作
├── 代码重构能力
├── 文档自动生成
├── CI/CD 集成
├── Web UI
└── 性能优化
```

---

## 关键原则

1. **架构简单化**：避免过度设计，switch 够用就不上接口。
2. **可配置化**：Prompt、模型、权限都通过配置管理。
3. **轻量安全可控**：不依赖 Docker，自实现软隔离。
4. **渐进式演进**：先只读型，再写入型，逐步增强。
5. **安全优先**：命令白名单、分支限制、PR 人工 review。
6. **工具化思维**：LLM 通过工具与代码库交互，而非一次性生成。
