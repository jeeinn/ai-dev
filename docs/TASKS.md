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
- [x] 3.4 gitea/pr.go — PR 操作（创建 PR + 评论 + 获取详情）
- [x] 3.5 gitea/repo.go — 仓库操作（获取仓库信息 + 获取文件内容）

## Phase 4：Agent 管理（模块 4）✅

- [x] 4.1 store/agent.go — Agent CRUD（Create/Get/GetByGiteaUsername/List/Update/Delete）
- [x] 4.2 store/route.go — 路由规则 CRUD（Create/List/Update/Delete）
- [x] 4.3 agents/manager.go — Agent 创建（含 Gitea 账号自动注册 + Token 自动生成）
- [x] 4.4 agents/registry.go — 运行时 Agent 注册表（按 ID/Username 查询 + LoadFromDB）
- [x] 4.5 agents/interaction.go — @Mention 互动处理（提取 @username + 触发回调）

## Phase 5：LLM 调用层（模块 5）✅

- [x] 5.1 llm/provider.go — Provider 接口定义（ChatCompletion）
- [x] 5.2 llm/openai.go — OpenAI 兼容 Provider（覆盖 DeepSeek/Qwen/Zhipu/Moonshot/Ollama）
- [x] 5.3 llm/anthropic.go — Anthropic Provider（Claude API，system 单独传）
- [x] 5.4 llm/registry.go — Provider 注册表（根据配置自动创建 Provider 实例）
- [x] 5.5 推理模型支持（reasoning_content 字段）

## Phase 6：Dispatcher + Agent 执行（模块 6）✅

- [x] 6.1 dispatcher/router.go — Label+Assignee 双条件路由（事件/动作/标签/分配人/提及匹配）
- [x] 6.2 dispatcher/queue.go — 任务队列（内存 channel + SQLite 持久化 + 启动加载 pending 任务）
- [x] 6.3 dispatcher/executor.go — Agent 执行器（信号量并发控制 + 超时 + 重试 + LLM 调用）
- [x] 6.4 store/task.go — 任务记录 CRUD（Create/UpdateStatus/Get/List/ListPending）
- [x] 6.5 store/log.go — 操作日志（LogOperation + ListOperationLogs）
- [x] 6.6 dispatcher/dispatcher.go — Dispatcher 组合（Router + TaskQueue + Executor）
- [x] 6.7 结果回写到 Gitea Issue/PR 评论

## Phase 7：核心 Agent 实现（模块 7）⬜

### 7.1 只读型 Agent（P1 优先级）

- [ ] 7.1.1 gitea/repo.go — 获取 PR Diff API
- [ ] 7.1.2 gitea/issue.go — 获取 Issue/PR 评论历史 API
- [ ] 7.1.3 agents/review_runner.go — PR 审查 Runner（获取 Diff → LLM 审查 → 评论报告）
- [ ] 7.1.4 agents/interaction_runner.go — @Mention 回复 Runner（加载评论上下文 → LLM 回复）
- [ ] 7.1.5 上下文拼装逻辑（组装 Diff、评论历史、Issue 内容等）

### 7.2 写入型 Agent（P2 优先级，简化沙箱方案）

- [ ] 7.2.1 sandbox/sandbox.go — 轻量级沙箱（目录隔离 + 命令白名单 + 超时控制）
- [ ] 7.2.2 sandbox/executor.go — 命令执行器（白名单验证 + 输出捕获 + 资源限制）
- [ ] 7.2.3 sandbox/git.go — Git 操作封装（clone/branch/commit/push + 分支限制）
- [ ] 7.2.4 sandbox/workspace.go — 工作目录管理（创建/清理/磁盘监控）
- [ ] 7.2.5 store/audit.go — 命令审计日志（记录所有执行的命令和输出）
- [ ] 7.2.6 agents/dev_runner.go — 研发 Runner（读 Issue → 分析 → 写代码 → 提 PR）
- [ ] 7.2.7 agents/bugfix_runner.go — Bug 修复 Runner（读 Bug Issue → 定位 → 修复 → 提 PR）

### 7.3 安全限制

- [ ] 7.3.1 命令白名单：git, go, python, node, npm, make, cargo 等
- [ ] 7.3.2 命令黑名单：rm -rf /, curl *, wget *, nc * 等
- [ ] 7.3.3 分支限制：只能 push 到 `ai/*` 分支，不允许 push 到默认分支
- [ ] 7.3.4 PR 必须人工 review 后才能合并
- [ ] 7.3.5 工作目录限制：只能在 `workspace/{task_id}/` 内操作

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

## Phase 11：队列可靠性（模块 11）⬜

- [ ] 11.1 pending task 后台扫描机制（每 60 秒扫描一次）
- [ ] 11.2 stale running task 恢复（超过 10 分钟重置为 pending）
- [ ] 11.3 Task 状态更新时间修复（started_at/finished_at）
- [ ] 11.4 队列满时策略（阻塞等待或 DB 后台扫描）

## Phase 12：前端 Web UI（模块 12）⬜（可选）

- [ ] 12.1 Vue 3 + Element Plus 项目初始化
- [ ] 12.2 Dashboard 仪表盘（任务统计、成功率、系统状态）
- [ ] 12.3 Agent 管理页面（创建/编辑/列表/启用禁用）
- [ ] 12.4 任务列表页面（查看/取消/重试）
- [ ] 12.5 Prompt 编辑页面（System Prompt + User Template + 历史版本）
- [ ] 12.6 系统配置页面（Gitea 连接、LLM Provider、路由规则）
- [ ] 12.7 go:embed 打包前端资源

## Phase 13：集成测试 + 收尾（模块 13）

- [x] 13.1 config.example.yaml 示例配置
- [ ] 13.2 README.md 项目说明
- [ ] 13.3 部署文档
- [ ] 13.4 端到端测试完善
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
| 模块 7：核心 Agent | ⬜ 未开始 | - |
| 模块 8：Prompt 管理 | ⬜ 未开始 | - |
| 模块 9：管理 API | ✅ 完成 | `d4c48fb` |
| 模块 10：配置化模板 | ✅ 完成 | `2f74746` |
| 模块 11：队列可靠性 | ⬜ 未开始 | - |
| 模块 12：前端 Web UI | ⬜ 未开始（可选） | - |
| 模块 13：集成收尾 | 🔶 部分完成 | - |

**总体进度：8/13 模块完成（62%）**

---

## 推荐开发路径

### v0.2：只读型 Agent ✅ 已完成

```
状态: 已完成 (tag: v0.2)
提交: 2d378bd

完成内容:
├── PR Diff API
├── 评论历史 API
├── ReviewRunner
├── InteractionRunner
├── 队列可靠性增强
└── 集成测试
```

### v0.3：写入型 Agent 基础 ✅ 已完成

```
状态: 已完成 (tag: v0.3)
提交: b0fefd9

完成内容:
├── 轻量级沙箱 (目录隔离 + 命令白名单 + 超时控制)
├── Git 操作封装 (clone/branch/commit/push)
├── 命令执行器
├── 命令审计日志
├── DevRunner / BugfixRunner (基础版)
└── 集成测试框架 (testify)
```

### v0.3.1：Go 原生 Tool-Use Agent（下一步）

```
优先级: P0 (当前)
预计工作量: 1-2 周
目标: 实现真正的 AI 代码修改能力

核心思路:
  LLM 通过 Function Calling 调用工具来理解和修改代码
  而不是一次性生成所有代码

架构:
  Gateway
    └── Agent Loop
         ├── 1. 加载代码库上下文 (目录结构 + 关键文件)
         ├── 2. 发送任务 + 工具定义给 LLM
         ├── 3. LLM 返回 tool_calls
         ├── 4. 执行工具 (读文件/写文件/搜索/运行命令)
         ├── 5. 将结果返回 LLM
         └── 6. 重复直到 LLM 返回 stop 或达到最大轮次

任务清单:
├── 1. LLM 层扩展
│   ├── 1.1 llm/provider.go — 扩展 ChatRequest 支持 Tools 字段
│   ├── 1.2 llm/provider.go — ChatResponse 支持 ToolCalls 字段
│   ├── 1.3 llm/openai.go — 实现 Function Calling 请求/响应解析
│   └── 1.4 llm/anthropic.go — 实现 Anthropic Tool Use 支持
│
├── 2. Tool 定义与注册
│   ├── 2.1 agent/tool.go — Tool 接口定义
│   ├── 2.2 agent/tool_read_file.go — 读取文件内容
│   ├── 2.3 agent/tool_write_file.go — 写入/创建文件
│   ├── 2.4 agent/tool_list_files.go — 列出目录结构
│   ├── 2.5 agent/tool_search_code.go — 搜索代码内容 (grep)
│   ├── 2.6 agent/tool_run_command.go — 执行命令 (受限)
│   └── 2.7 agent/tool_apply_diff.go — 应用 Diff 补丁
│
├── 3. Agent Loop 实现
│   ├── 3.1 agent/loop.go — Agent Loop 核心逻辑
│   │   ├── 多轮对话管理
│   │   ├── Tool Call 解析与执行
│   │   ├── 结果收集与返回
│   │   └── 终止条件判断 (stop / max_iterations / error)
│   ├── 3.2 agent/context.go — 代码库上下文加载
│   │   ├── 目录结构扫描 (tree)
│   │   ├── 关键文件识别 (go.mod, main.go, README)
│   │   └── Token 限制下的内容截断
│   └── 3.3 agent/prompt.go — System Prompt 构建
│       ├── 角色定义 (高级工程师)
│       ├── 任务描述模板
│       ├── 工具使用说明
│       └── 输出格式要求
│
├── 4. Runner 改造
│   ├── 4.1 agents/dev_runner.go — 改用 Agent Loop
│   │   ├── 克隆仓库
│   │   ├── 加载代码上下文
│   │   ├── 调用 Agent Loop 生成修改
│   │   ├── 验证修改 (go build / go test)
│   │   └── 提交并创建 PR
│   └── 4.2 agents/bugfix_runner.go — 改用 Agent Loop
│       ├── 克隆仓库
│       ├── 分析 Bug 描述
│       ├── 定位问题代码
│       ├── 调用 Agent Loop 生成修复
│       ├── 运行测试验证
│       └── 提交并创建 PR
│
├── 5. 验证与测试
│   ├── 5.1 集成测试 — Issue → Agent Loop → PR
│   ├── 5.2 端到端测试 — 真实 Gitea 环境
│   └── 5.3 边界测试 — 错误处理、超时、Token 限制
│
└── 6. 文档更新
    ├── 6.1 更新 TASKS.md
    ├── 6.2 更新 agent-development-decisions.md
    └── 6.3 API 文档

关键设计决策:
├── LLM 支持: DeepSeek / OpenAI 兼容 (Function Calling)
├── 工具数量: 初始 6 个基础工具
├── 最大轮次: 20 轮 (防止无限循环)
├── 上下文窗口: 最多 8K tokens 的代码上下文
├── 验证策略: 每次修改后运行 go build 验证
└── 错误处理: 工具执行错误返回给 LLM 自行修复
```

### v0.4：增强与优化

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
