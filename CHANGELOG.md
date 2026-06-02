# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Phase 14: 沙箱增强任务规划 (TASKS.md)

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

[Unreleased]: https://github.com/your-org/gitea-agent-gateway/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/your-org/gitea-agent-gateway/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/your-org/gitea-agent-gateway/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/your-org/gitea-agent-gateway/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/your-org/gitea-agent-gateway/releases/tag/v0.1.0
