# Agent 后续开发决策记录

> 记录时间：2026-06-01  
> 最后更新：2026-06-02  
> 目的：沉淀当前关于 Agent 执行模型、安全边界、队列、Webhook 去重、Prompt 模板等关键设计决策，作为后续开发依据。

## 一、背景

当前项目已经具备完整的端到端 Agent 执行链路：

```text
Gitea Webhook
  -> Webhook Handler (签名验证 + 去重)
  -> Dispatcher Router (事件路由)
  -> TaskQueue (SQLite 持久化 + 内存队列)
  -> Agent Executor (并发控制)
  -> Runner (AnalyzeRunner / ReviewRunner / DevRunner / BugfixRunner)
  -> Agent Loop (多轮对话 + Tool Calling)
  -> Sandbox (目录隔离 + 命令白名单)
  -> Gitea 评论 / PR 回写
```

**当前版本：v0.3.1**，已完成 Tool-Use Agent 实现，端到端测试验证通过。

---

## 二、目标 Agent 执行模型

### 2.1 设计结论

Agent 类型不同，执行模型不同：

| Agent 类型 | 执行方式 | 是否修改仓库 | 是否创建 PR | 典型输出 | 实现状态 |
|---|---|---|---|---|---|
| Analyze Agent | 读取 Issue，调用 LLM 生成需求分析 | 否 | 否 | Issue 评论 | ✅ 已实现 |
| CodeReview Agent | 读取 PR/Diff，调用 LLM 生成审查建议 | 否 | 否 | PR 评论 | ✅ 已实现 |
| Interaction Agent | 读取 Issue/PR 评论上下文，调用 LLM 回复 | 否 | 否 | 评论回复 | ✅ 已实现 |
| Dev Agent | Tool-Use Agent，理解代码库，修改代码 | 是 | 是 | 分支、Commit、PR | ✅ 已实现 |
| Bugfix Agent | Tool-Use Agent，定位问题，修复代码 | 是 | 是 | 分支、Commit、PR | ✅ 已实现 |

### 2.2 Runner 架构

```text
RunnerFactory
  ├── AnalyzeRunner   (只读: Issue → LLM → 评论)
  ├── ReviewRunner    (只读: PR Diff → LLM → 评论)
  ├── InteractionRunner (只读: 评论历史 → LLM → 回复)
  ├── DevRunner       (写入: Agent Loop → Git → PR)
  └── BugfixRunner    (写入: Agent Loop → Git → PR)
```

### 2.3 任务类型映射

| 事件 | 任务类型 | Runner | 触发条件 |
|------|----------|--------|----------|
| issues/assigned | analyze_issue | AnalyzeRunner | Issue 分配给 Agent |
| issues/labeled (ai:solve) | solve_issue | DevRunner | Issue 添加 ai:solve 标签 |
| issues/labeled (ai:fix) | fix_bug | BugfixRunner | Issue 添加 ai:fix 标签 |
| pull_request/opened | review_pr | ReviewRunner | PR 创建 |
| issue_comment | reply_comment | InteractionRunner | @Mention Agent |

---

## 三、宿主机命令执行与沙箱设计

### 3.1 设计结论

**不使用 Docker**，采用轻量级"软隔离"方案。

### 3.2 理由

1. **国内部署不便**：Docker 在国内环境部署受限，镜像拉取困难。
2. **项目负担**：Docker 增加了部署复杂度和维护成本。
3. **轻量级需求**：当前场景不需要完整的容器隔离。
4. **可控性**：自实现方案更可控，易于调试和维护。

### 3.3 当前实现

```go
// sandbox/sandbox.go
type Sandbox struct {
    BaseDir     string            // 基础工作目录
    TaskID      int64             // 任务 ID
    WorkDir     string            // 工作目录 (BaseDir/task_{TaskID})
    Timeout     time.Duration     // 命令超时
    MaxOutput   int               // 最大输出字节数
    AllowedCmds map[string]bool   // 允许的命令白名单
}
```

### 3.4 安全边界

| 安全措施 | 实现 | 说明 |
|----------|------|------|
| 目录隔离 | ✅ | 每个任务独立目录 `workspace/{task_id}/` |
| 命令白名单 | ✅ | git, sh, bash, go, python, node, npm, make 等 |
| 命令黑名单 | ✅ | rm, dd, mkfs, shutdown, reboot 等 |
| 超时控制 | ✅ | 单命令 5 分钟 |
| 输出限制 | ✅ | stdout/stderr 各 1MB |
| 分支限制 | ✅ | 只能 push 到 `ai/*` 分支 |
| 分支名验证 | ✅ | ValidateBranchName + GenerateBranchName |
| 审计日志 | ✅ | 记录所有命令执行到 operation_logs |
| PR 人工 review | ✅ | 不允许自动合并 |

### 3.5 后续增强（v0.4）

借鉴 ai-git-bot 项目：

1. **临时目录模式**：支持 `os.MkdirTemp` 创建临时工作目录
2. **更丰富的上下文工具**：cat 增强、tree、git_log、git_blame、rg
3. **配置化的超时和限制**：SandboxConfig 结构，支持 Web UI 配置
4. **安全增强**：文件路径验证、文件大小限制、命令参数验证

---

## 四、Agent Token 权限收敛

### 4.1 设计结论

Agent Token 权限由用户在 Web UI 创建或配置 Agent 时选择。

### 4.2 当前实现

- API 响应使用 AgentDTO，隐藏 `gitea_token` ✅
- 管理 API 增加 Bearer Token 认证 ✅

### 4.3 后续增强

- [ ] DB 中 Token 加密存储
- [ ] 扩展 `agents` 表，记录 Agent 权限配置
- [ ] 所有执行器调用 Gitea 写操作前检查 Agent 权限

---

## 五、任务队列设计

### 5.1 设计结论

短期坚持 SQLite + 内存队列，适合小规模单实例部署。

### 5.2 当前实现

| 功能 | 状态 | 说明 |
|------|------|------|
| 内存队列 | ✅ | channel 实现 |
| SQLite 持久化 | ✅ | 任务创建时持久化 |
| 启动加载 pending | ✅ | LoadPending() |
| 后台扫描 | ✅ | 每 60 秒扫描一次 |
| stale 任务恢复 | ✅ | 超过 10 分钟重置为 pending |
| 队列满兜底 | ✅ | DB 后台扫描 |

### 5.3 后续增强

- [ ] 多实例支持：任务锁字段 `locked_by`、`locked_until`
- [ ] 可选：Redis / NATS / RabbitMQ 外部队列

---

## 六、Webhook 去重语义

### 6.1 设计结论

Webhook 去重语义采用：**成功持久化任务后去重**。

### 6.2 当前实现

```text
收到 Webhook
  -> 解析成功
  -> 路由匹配
  -> 创建 task
  -> task 写入 DB 成功
  -> 标记 delivery 已处理
  -> 返回 accepted
```

### 6.3 已完成

- [x] `webhook.Handler` 在 callback 后标记 delivery
- [x] 任务创建时检查 delivery_id 唯一性
- [x] 未匹配事件也标记为已处理（避免重复重试）

---

## 七、Prompt 模板来源

### 7.1 设计结论

需要在 `config.example.yaml` 和实际配置中增加 `agents.templates`。

### 7.2 当前实现

```yaml
agents:
  defaults:
    provider: "deepseek"
    model: "deepseek-v4-flash"
    max_tokens: 4096
    temperature: 0.3

  templates:
    analyze:
      name: "需求分析 Agent"
      system_prompt: "..."
      user_template: "请分析以下 Issue：..."

    review:
      name: "代码审查 Agent"
      system_prompt: "..."
      user_template: "请审查以下 PR：..."

    reply:
      name: "评论回复 Agent"
      system_prompt: "..."
      user_template: "请回复以下评论：..."
```

### 7.3 Prompt 优先级

```text
Agent DB 自定义配置
  > config.yaml agents.templates
  > 系统内置兜底模板
```

### 7.4 已完成

- [x] config/schema.go — AgentsConfig, AgentTemplateConfig
- [x] config.example.yaml — 预置 analyze/review/reply 三种模板
- [x] dispatcher/template.go — Go template 渲染引擎
- [x] 模板变量支持：Issue, PR, Comment, Repo, Sender

---

## 八、Tool-Use Agent 实现方案（v0.3.1）

### 8.1 决策结论

**采用 Go 原生 Tool-Use 方案**，通过 Function Calling 实现 LLM 与代码库交互。

### 8.2 理由

1. **纯 Go，无外部依赖**：保持项目轻量
2. **渐进式实现**：先支持简单场景，逐步增强
3. **完全可控**：不依赖第三方项目的状态
4. **端到端验证通过**：Issue → Agent Loop → PR 测试成功

### 8.3 架构

```text
Gateway
  └── Agent Loop
       ├── 1. 加载代码库上下文 (目录结构 + 关键文件)
       ├── 2. 发送任务 + 工具定义给 LLM
       ├── 3. LLM 返回 tool_calls
       ├── 4. 执行工具 (读文件/写文件/搜索/运行命令)
       ├── 5. 将结果返回 LLM
       └── 6. 重复直到 LLM 返回 stop 或达到最大轮次
```

### 8.4 工具定义

| 工具 | 功能 | 参数 |
|------|------|------|
| `read_file` | 读取文件内容 | `path` |
| `write_file` | 写入/创建文件 | `path`, `content` |
| `list_files` | 列出目录结构 | `dir` |
| `search_code` | 搜索代码内容 | `pattern`, `dir` |
| `run_command` | 执行命令 (受限) | `command` |
| `apply_diff` | 应用 Diff 补丁 | `diff` |

### 8.5 关键参数

| 参数 | 当前值 | 说明 |
|------|--------|------|
| 最大轮次 | 20 | 防止无限循环 |
| 上下文窗口 | 8K tokens | 代码上下文限制 |
| LLM 支持 | DeepSeek / OpenAI | Function Calling 支持 |
| 验证策略 | go build / go test | 每次修改后验证 |
| 错误处理 | 返回给 LLM | 工具执行错误由 LLM 自行修复 |

---

## 九、竞品分析结论

### 9.1 调研项目

| 项目 | 语言 | 核心功能 | License |
|------|------|----------|---------|
| **wshm** | Rust | AI 代码审查、Merge Queue、Issue Triage | SSPL-1.0 |
| **ai-git-bot** | Java | PR Review、Issue→Code、Writer Agent、E2E 测试 | MIT |

### 9.2 代码修改实现方式对比

| 项目 | 代码修改方式 | 是否 Tool-Use | 说明 |
|------|-------------|---------------|------|
| **wshm OSS** | ❌ 不支持 | - | 只做分析，不做修改 |
| **wshm Pro** | 结构化输出 | ❌ | JSON + Unified Diff，一次性生成 |
| **ai-git-bot** | Tool-Use Agent | ✅ | 多轮对话，迭代式开发 |
| **我们** | Tool-Use Agent | ✅ | 多轮对话，迭代式开发 |

### 9.3 沙箱隔离对比

| 维度 | ai-git-bot | 我们 | 评价 |
|------|------------|------|------|
| **目录隔离** | ✅ 临时目录 | ✅ 固定目录 | 各有优势 |
| **命令白名单** | ❌ 无 | ✅ 有 | 我们更安全 |
| **分支限制** | ❌ 无 | ✅ 有 | 我们更安全 |
| **审计日志** | ❌ 无 | ✅ 有 | 我们更好 |
| **输出限制** | ✅ 有 | ✅ 有 | 差不多 |
| **超时控制** | ✅ 有 | ✅ 有 | 差不多 |

### 9.4 可借鉴改进

从 **ai-git-bot** 借鉴：
1. 临时目录模式（更灵活）
2. 更丰富的上下文工具（cat 增强、tree、git_log、git_blame、rg）
3. 配置化的超时和限制

从 **wshm** 借鉴：
1. Issue Triage（自动分类标签）
2. PR Risk Analysis（风险评估）
3. Merge Queue（自动合并队列）

---

## 十、关键原则

1. **架构简单化**：避免过度设计，switch 够用就不上接口。
2. **可配置化**：Prompt、模型、权限、超时都通过配置管理。
3. **轻量安全可控**：不依赖 Docker，自实现软隔离。
4. **渐进式演进**：先只读型，再写入型，逐步增强。
5. **安全优先**：命令白名单、分支限制、PR 人工 review。
6. **工具化思维**：LLM 通过工具与代码库交互，而非一次性生成。
7. **借鉴但不依赖**：参考开源项目优点，但保持自主实现。
