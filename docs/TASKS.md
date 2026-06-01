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

## Phase 6：Dispatcher + Agent 执行（模块 6）✅

- [x] 6.1 dispatcher/router.go — Label+Assignee 双条件路由（事件/动作/标签/分配人/提及匹配）
- [x] 6.2 dispatcher/queue.go — 任务队列（内存 channel + SQLite 持久化 + 启动加载 pending 任务）
- [x] 6.3 dispatcher/executor.go — Agent 执行器（信号量并发控制 + 超时 + 重试 + LLM 调用）
- [x] 6.4 store/task.go — 任务记录 CRUD（Create/UpdateStatus/Get/List/ListPending）
- [x] 6.5 store/log.go — 操作日志（LogOperation + ListOperationLogs）

## Phase 7：核心 Agent 实现（模块 7）⬜

- [ ] 7.1 agents/base.go — Agent 基础逻辑（clone/分支/推送/创建PR/评论回写）
- [ ] 7.2 agents/analyze.go — 需求分析 Agent（读 Issue → 分析 → 评论报告 → 打标签）
- [ ] 7.3 agents/dev.go — 研发 Agent（读 Issue + 分析报告 → 写代码 → 提 PR）
- [ ] 7.4 agents/bugfix.go — Bug 修复 Agent（读 Bug Issue → 定位 → 修复 → 提 PR）
- [ ] 7.5 agents/review.go — PR 审查 Agent（读 Diff → 审查 → 评论报告）
- [ ] 7.6 agents/interaction_agent.go — @Mention 互动 Agent（读评论上下文 → 回复）

## Phase 8：Prompt 管理（模块 8）⬜

- [ ] 8.1 store/prompt.go — Prompt 历史版本存储（prompt_history 表 CRUD）
- [ ] 8.2 agents/prompt.go — Prompt 加载（DB > config.yaml 两层覆盖）

## Phase 9：管理 API（模块 9）✅

- [x] 9.1 api/router.go — API 路由注册（RESTful 风格，Go 1.22 method patterns）
- [x] 9.2 api/agent.go — Agent CRUD 接口（GET/POST/PUT/DELETE /api/agents）
- [x] 9.3 api/task.go — 任务查询接口（GET /api/tasks, GET /api/tasks/{id}）
- [x] 9.4 api/route.go — 路由规则接口（GET/POST/DELETE /api/routes）
- [x] 9.5 api/stats.go — 统计数据（GET /api/stats）
- [x] 9.6 api/log.go — 操作日志（GET /api/logs）

> 注：模块 9 合并为单文件 api/router.go 实现，包含 Agent/Task/Route/Log/Stats 全部接口。

## Phase 10：前端 Web UI（模块 10）⬜

- [ ] 10.1 Vue 3 + Element Plus 项目初始化
- [ ] 10.2 Dashboard 仪表盘（任务统计、成功率、系统状态）
- [ ] 10.3 Agent 管理页面（创建/编辑/列表/启用禁用）
- [ ] 10.4 任务列表页面（查看/取消/重试）
- [ ] 10.5 Prompt 编辑页面（System Prompt + User Template + 历史版本）
- [ ] 10.6 系统配置页面（Gitea 连接、LLM Provider、路由规则）
- [ ] 10.7 go:embed 打包前端资源

## Phase 11：集成测试 + 收尾（模块 11）

- [x] 11.1 config.example.yaml 示例配置
- [ ] 11.2 README.md 项目说明
- [ ] 11.3 Dockerfile
- [ ] 11.4 端到端测试（手动）
- [x] 11.5 编译验证（go build，单二进制 9.4 MB）

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
| 模块 10：前端 Web UI | ⬜ 未开始 | - |
| 模块 11：集成收尾 | 🔶 部分完成 | `7097091` |

**总体进度：7/11 模块完成（64%）**
