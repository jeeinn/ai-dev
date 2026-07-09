# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Agent 对话持久化（调试）**: 新增 `task_conversation_logs` 表；系统配置「调试」页可开启 `debug.conversation_log.enabled`，将 Agent Loop 每轮 LLM 消息与 tool call 写入 SQLite（默认关闭）

### Fixed
- **沙箱工具跨平台**: `list_files` / `tree` / `search_code` 在 Windows 走 PowerShell，在 Unix 走 `find`/`grep`；`run_command` 在 Windows 用 `cmd /C`，Unix 用 `sh -c`；修复 Windows 上 Agent Loop 因无 `find`/`sh` 空转耗尽迭代的问题
- **任务卡住**: Ctrl+C 后 `running` 任务残留导致「已有任务正在处理」；启动时自动将孤儿 `running` 标为 failed；任务列表增加「重置」操作（`POST /api/tasks/{id}/reset`）

### Changed (Agent LLM 预算与超时统一)
- **Token**：`max_tokens` → `max_output_tokens`；新增 `max_input_tokens`；删除 `loop_config.max_tokens` 与 `llm.defaults.max_tokens`
- **默认 `max_input_tokens`**：`8192` → `65536`（缓解 tool-use 多轮后上下文被截断导致重复读文件）
- **重试拆分**：`dispatcher.retry_count` → `dispatcher.task_retry_count`（整任务）+ `llm.rate_limit_retries`（仅 429）；启动时自动迁移旧 key
- **优雅退出**：Ctrl+C / SIGTERM 时取消 in-flight Agent Loop / LLM 请求，避免任务长期卡在 `running`
- **超时**：删除 `dispatcher.timeout` 与 `loop.timeout`；单次任务用 `agents.defaults.timeout` / `agent.timeout`；Loop 仅用 `total_timeout`
- **Temperature**：迁至 `agents.defaults.temperature`（LLM Tab 不再配置）
- **截断**：发请求前按 `max_input_tokens` 截断 messages（含 tools JSON）；估算为字符数/4
- **迁移**：启动时回填 `max_output_tokens = max(旧 max_tokens, loop.max_tokens)`，并清理 system_config 旧 key

### Added (Assign Workflow v2 — Phase 16)
- **Agent role 字段**: `analyze` | `coder` | `review`，决定触发后的任务类型
- **Event Resolver** (`internal/workflow/resolver.go`): 替代 Router.Match + determineTaskType
  - `issues.assigned`: 通过 payload 中单个 `assignee` 查找 Registry Agent → 按 role 映射 task_type
  - `pull_request` + `review_requested`: 在 reviewers 中查找 review 角色 Agent
  - `issues.labeled` / `unassigned`: 忽略（v2 不再使用 Label 触发）
- **WorkflowContext 状态机** (`internal/workflow/context.go`):
  - 阶段: `idle → analyzing → analyzed → developing → reviewing → done`
  - Task 完成回调: analyze→analyzed, solve→developing(写入 PR ID)
- **L1 结构性门禁** (`internal/workflow/gate_l1.go`):
  - `l1.review_requires_pr`: review Agent 需要有 open PR
  - `l1.review_on_closed_pr`: PR 已关闭 → hard 拒绝
- **Dispatcher v2 流水线**: sender 过滤 → Resolver → L1 门禁 → WorkflowContext → in-flight 锁 → 入队
- **新数据表**: `workflow_contexts`, `agent_sessions`
- **tasks 表扩展**: `session_id`, `role` 字段
- **18 个 store 单元测试** + **16 个 resolver 测试** + **8 个集成测试**

### Breaking Changes (v2)
- **Label 触发已移除**: `issues.labeled` / `pull_request.labeled` 事件不再触发任务
- **Router.Label 匹配已移除**: `determineTaskType()` 中 `ai:solve` / `ai:fix` 等 Label 分支已删除
- **迁移**: 使用 Label (`ai:analyze`, `ai:solve`) 触发的用户需改为 Assign Agent

### Planned
- Phase 14: 沙箱增强（详见 sandbox-roadmap.md）
- Phase 17: Session 续作 + WorkflowPolicy L2/L3

## [0.7.0] - 2026-06-05

### Added
- 系统配置页面 (SystemConfig.vue)
  - 标签页布局: Gitea 连接 / LLM 配置 / 任务调度 / Agent 默认参数 / Prompt 模板
  - ConfigManager: DB 配置 > 文件配置 > 默认值
  - GET/PUT/DELETE /api/config 端点（含 key 校验）
  - LLM Registry 热更新
  - Prompt 模板管理（查看/新增/删除自定义模板，DB 持久化）
  - 配置项说明 tips（MaxTokens/Temperature 含义区分）
- Agent 详情页 (AgentDetail.vue)
  - 基本信息编辑 + 模板变量说明
  - 触发规则管理（Route CRUD + 快捷配置 + 预计执行行为）
  - Prompt 版本历史（详情查看 + 回滚 + 删除）
- Agent 创建增强
  - 表单分组折叠（核心字段直接展示，高级配置折叠）
  - 模板选择下拉框（从 /api/prompt-templates 动态加载）
  - Provider 下拉从配置动态读取
  - 创建表单从 agents.defaults 读取默认值
- 触发规则增强
  - 预计执行行为列（根据 event+action+label 自动推断，图标+中文描述）
  - 防重复规则（CreateRoute 唯一性检查）
  - 优先级说明（值越大越优先）
- 任务列表增强
  - 服务端分页（limit/offset + total）
  - 筛选：状态 / 任务类型 / Agent
  - Agent 名称显示（非 ID）
- Dashboard 优化
  - 新用户引导卡片（无 Agent 时显示，三步跳转）
  - 最近任务 / Agent 列表限 10 条 + 查看全部链接
- 用户管理 API
  - GET/POST/PUT/DELETE /api/users（JWT 认证）
- 配置值生效链路
  - RunnerFactory 持有 defaultMaxTokens / defaultTemp
  - runners 所有 LLM 调用使用 resolveMaxTokens / resolveTemperature
  - Agent.MaxTokens 为 0 时回退到 agents.defaults.max_tokens
- 共享组件
  - TemplateHelp.vue: 模板变量说明弹窗（三处复用）
- 文档
  - ARCHITECTURE.md 校正 + mermaid 图
  - README.md 重写
  - DEPLOYMENT.md 部署指南
  - 端到端测试报告

### Changed
- Prompt 管理拆分: 内置模板→系统配置，自定义版本→Agent 详情页
- 删除独立 Prompts.vue 页面及菜单
- 所有弹窗禁用点击外部关闭（close-on-click-modal=false）
- Menu 顺序调整: 仪表盘、任务列表、Agent 管理、用户管理、系统配置

### Fixed
- 用户管理页面返回 HTML（添加 /api/users 端点）
- 内置模板为空（/api/prompt-templates 返回内置 + 自定义模板）
- Prompt 版本记录（Agent 编辑时自动创建 prompt_history）
- AgentDetail 页面空白（form 初始化 + 错误处理）
- 启动时 prompt.templates 警告（配置校验 + 详细 WARN 提示）
- Dashboard /api/tasks 返回格式适配
- SQLite 迁移顺序（ALTER TABLE 在 CREATE TABLE 之后）
- Agent 列表模板加载改用 /api/prompt-templates

## [0.6.0] - 2026-06-03

### Added
- Web UI (Vue 3 + Element Plus)
  - Login.vue: 登录页面
  - Dashboard.vue: 仪表盘 (统计/最近任务)
  - Agents.vue: Agent 管理 (CRUD)
  - Tasks.vue: 任务列表 (详情查看)
  - Prompts.vue: Prompt 管理 (版本/回滚)
  - Users.vue: 用户管理 (admin)
- 认证系统
  - store/user.go: users 表 + CRUD
  - auth/jwt.go: JWT 认证
  - auth/password.go: bcrypt 密码哈希
  - api/auth_handler.go: 登录/登出 API
- 前端构建
  - Vue 3 + Element Plus + Vite
  - Pinia 状态管理
  - Vue Router 路由守卫
  - Axios API 客户端 (JWT 拦截器)
- 打包部署
  - go:embed 嵌入前端资源
  - SPA 路由支持

## [0.5.0] - 2026-06-03

### Added
- Prompt 历史版本管理 (store/prompt.go)
  - CreatePromptVersion: 创建新版本
  - GetPromptVersion: 获取指定版本
  - GetActivePrompt: 获取活跃版本
  - ListPromptVersions: 列出所有版本
  - ActivatePromptVersion: 激活指定版本 (回滚)
  - DeletePromptVersion: 删除版本
- Prompt 加载管理 (agents/prompt.go)
  - 优先级: DB > Agent > Config > Built-in
  - 6 个内置模板: default, analyze_issue, review_pr, reply_comment, solve_issue, fix_bug
- Prompt API 端点
  - GET /api/agents/{id}/prompts: 列出版本
  - POST /api/agents/{id}/prompts: 创建版本
  - GET /api/agents/{id}/prompts/active: 获取活跃版本
  - POST /api/prompts/{id}/activate: 激活版本 (回滚)
  - DELETE /api/prompts/{id}: 删除版本
- 数据库迁移: prompt_history 表添加 is_active 和 note 字段

## [0.4.0] - 2026-06-02

### Added
- SandboxConfig 结构定义
  - Mode: 工作目录模式 (temp | fixed)
  - CommandTimeout: 单命令超时
  - TaskTimeout: 总任务超时
  - MaxOutput: 最大输出字节数
  - MaxFileSize: 最大文件大小
  - CleanupAfter: 失败任务保留时间
- 临时目录模式 (ModeTemp)
  - os.MkdirTemp 自动创建临时目录
  - CleanupWithDelay 延迟清理
- 更丰富的上下文工具
  - tree: 目录结构展示
  - git_log: Git 提交历史
  - git_blame: 文件修改历史
- AgentLoopConfig 结构定义
  - MaxIterations: 最大迭代轮次
  - MaxTokens: 单次 LLM 调用最大 tokens
  - Timeout: 单轮超时
  - TotalTimeout: 总超时
- NewAgentLoopWithConfig: 从配置创建 AgentLoop

### Changed
- Sandbox 使用 SandboxConfig 替代旧 Config
- 路径验证支持大小写不敏感比较 (Windows 兼容)

### Fixed
- 路径穿越攻击防护
- 文件大小限制验证

## [0.3.1] - 2026-06-02

### Added
- LLM Function Calling 支持 (Tool/ToolCall/Function 类型)
- `internal/agent` 包：Tool-Use Agent 实现
  - tools.go: Tool 定义与注册，6 个基础工具
    - `read_file`: 读取文件内容
    - `write_file`: 写入/创建文件
    - `list_files`: 列出目录结构
    - `search_code`: 搜索代码内容 (grep)
    - `run_command`: 执行命令 (受限)
    - `apply_diff`: 应用 Diff 补丁
  - loop.go: Agent Loop 多轮对话核心逻辑
  - context.go: 代码库上下文加载与 Prompt 构建
- Label 任务类型支持 (`ai:solve` → solve_issue, `ai:fix` → fix_bug)
- testify 集成测试框架
- 集成测试套件 (tests/integration/)
  - helpers_test.go: 测试辅助函数 (TestEnv, MockGitea, MockLLM)
  - webhook_test.go: Webhook 端到端测试
  - agent_test.go: Agent 生命周期测试

### Changed
- DevRunner / BugfixRunner 改用 Agent Loop (Tool-Use 模式)
- RunnerFactory 增加 db 参数支持
- Executor.SetGiteaClientFactory 传递 db 给 RunnerFactory

### Fixed
- DevRunner/BugfixRunner 的 DB 注入问题 (AuditLogger nil panic)
- 推理模型支持 (reasoning_content 字段)

## [0.3.0] - 2026-06-02

### Added
- 轻量级沙箱 (`internal/sandbox/`)
  - sandbox.go: 工作目录隔离 + 命令白名单 + 超时控制 + 输出限制
  - git.go: Git 操作封装 (clone/branch/commit/push + 分支限制)
  - audit.go: 命令审计日志
- DevRunner / BugfixRunner 基础版
- 命令白名单: git, sh, bash, go, python, node, npm, make 等
- 分支名验证: ValidateBranchName + GenerateBranchName

## [0.2.0] - 2026-06-01

### Added
- Gitea API 扩展
  - PRDiff: 获取 PR diff 内容
  - PRFiles: 获取 PR 变更文件列表
  - IssueComments: 获取评论历史
- Runner 接口和实现
  - AnalyzeRunner: Issue 分析
  - ReviewRunner: PR 审查
  - InteractionRunner: @Mention 回复
  - RunnerFactory: 根据 task_type 选择 Runner
- 队列可靠性增强
  - pending task 后台扫描 (每 60 秒)
  - stale running task 恢复 (超过 10 分钟)
- API 认证 (Bearer Token 中间件)
- 配置化模板 (agents.templates)
  - Go template 渲染引擎
  - 预置 analyze/review/reply 三种模板
- API 响应隐藏 gitea_token (AgentDTO)

### Changed
- Dispatcher 使用 RunnerFactory 选择 Runner
- webhook.EventCallback 返回 bool (支持失败重试)

### Fixed
- Webhook 去重时机 (任务成功入队后才标记)

## [0.1.0] - 2026-06-01

### Added
- 项目骨架 (Go 1.26.3)
- Webhook 接收
  - HTTP Handler (签名验证 + 去重 + 异步回调)
  - HMAC-SHA256 签名验证
  - X-Gitea-Delivery 幂等去重
  - 事件解析 (issues/PR/comment)
- Gitea API 客户端
  - Admin API (创建用户 + 生成 Token)
  - Issue 操作 (评论 + 标签)
  - PR 操作 (创建 + 评论)
  - 仓库操作 (信息 + 文件内容)
- Agent 管理
  - Agent CRUD
  - 路由规则 CRUD
  - Agent 创建 (含 Gitea 账号自动注册)
- LLM 调用层
  - Provider 接口
  - OpenAI 兼容 Provider
  - Anthropic Provider
  - Provider 注册表
- Dispatcher
  - Router (Label+Assignee 双条件路由)
  - TaskQueue (SQLite + 内存队列)
  - Executor (并发控制 + 超时 + 重试)
- 管理 API
  - Agent CRUD 接口
  - 任务查询接口
  - 路由规则接口
  - 统计数据
  - 操作日志
- SQLite 存储 (WAL 模式)
- YAML 配置 (环境变量展开)

[Unreleased]: https://github.com/your-org/gitea-agent-gateway/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/your-org/gitea-agent-gateway/compare/v0.3.1...v0.7.0
[0.3.1]: https://github.com/your-org/gitea-agent-gateway/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/your-org/gitea-agent-gateway/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/your-org/gitea-agent-gateway/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/your-org/gitea-agent-gateway/releases/tag/v0.1.0
