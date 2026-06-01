# Agent 后续开发决策记录

> 记录时间：2026-06-01  
> 目的：沉淀当前关于 Agent 执行模型、安全边界、队列、Webhook 去重、Prompt 模板等关键设计决策，作为后续开发依据。

## 一、背景

当前项目已经具备 Webhook 接收、Gitea API、Agent 管理、LLM Provider、Dispatcher、SQLite 存储等基础模块，但真正的端到端 Agent 执行链路尚未完全打通。

后续开发重点应围绕以下主链路推进：

```text
Gitea Webhook
  -> Webhook Handler
  -> Dispatcher Router
  -> TaskQueue
  -> Agent Executor
  -> Agent Runtime
  -> LLM / Git / Shell / Gitea API
  -> Gitea 评论 / 分支 / PR / 标签回写
```

本文档记录几个已确认或需要重点推进的关键设计点。

---

## 二、目标 Agent 执行模型

### 2.1 设计结论

Agent 类型不同，执行模型不同：

| Agent 类型 | 执行方式 | 是否修改仓库 | 是否创建 PR | 典型输出 |
|---|---|---:|---:|---|
| CodeReview Agent | 读取 PR/Diff，调用 LLM 生成审查建议 | 否 | 否 | PR 评论 / Review 报告 |
| Analyze Agent | 读取 Issue，调用 LLM 生成需求分析 | 否 | 否 | Issue 评论 / 标签 |
| Interaction Agent | 读取 Issue/PR 评论上下文，调用 LLM 回复 | 通常否，特殊情况可触发修改型任务 | 通常否 | 评论回复 |
| Dev Agent / Issue Solver | clone 仓库，创建分支，修改代码，运行命令，提交并 push，创建 PR | 是 | 是 | 分支、Commit、PR、评论 |
| Bugfix Agent | clone 仓库，定位问题，修改代码，运行测试，提交并创建 PR | 是 | 是 | 分支、Commit、PR、评论 |

### 2.2 设计建议

后续应把 Agent 执行抽象成多种 `Runner` 或 `Workflow`，避免所有逻辑堆进当前 `dispatcher.Executor`。

建议抽象：

```text
AgentExecutor
  -> 根据 agent.type / task.task_type 选择 Runner

Runner 接口：
  - ReviewRunner：只读 PR diff，LLM 审查，评论回写
  - AnalyzeRunner：只读 Issue，LLM 分析，评论/标签回写
  - InteractionRunner：只读评论上下文，LLM 回复
  - DevRunner：clone、分支、修改、测试、push、创建 PR
  - BugfixRunner：clone、定位、修复、测试、push、创建 PR
```

### 2.3 TODO

- [ ] 为 `agents` 表增加 `type` 字段，例如：`review` / `analyze` / `interaction` / `dev` / `bugfix` / `custom`。
- [ ] 为 `tasks` 表增加更明确的 `task_type` 语义，例如：`review_pr` / `analyze_issue` / `solve_issue` / `fix_bug` / `reply_comment`。
- [ ] 将当前 `dispatcher.Executor` 从“直接调用 LLM”改造为“选择具体 Runner 并执行”。
- [ ] 先实现只读型 Agent：`AnalyzeRunner`、`ReviewRunner`、`InteractionRunner`。
- [ ] 再实现写入型 Agent：`DevRunner`、`BugfixRunner`。
- [ ] 写入型 Agent 必须通过 PR 交付，不允许直接 push 到默认分支。

---

## 三、宿主机命令执行与沙箱设计

### 3.1 设计结论

允许智能体 Agent 在宿主机执行命令，但必须具备以下安全限制：

1. 沙箱隔离。
2. 超时控制。
3. 资源限制。
4. 工作目录限制。
5. 命令白名单/黑名单。
6. 日志审计。
7. 失败任务保留与清理。

尤其是 `Dev Agent` 和 `Bugfix Agent` 需要执行：

```text
git clone
git checkout -b <branch>
代码修改命令
测试 / 构建 / lint 命令
git add / commit / push
创建 PR
```

这些操作必须受到严格边界控制。

### 3.2 沙箱策略

建议分阶段实现。

#### Phase A：本地受限执行，适合早期 MVP

- 每个任务使用独立工作目录：`workspace/{task_id}/repo`。
- 所有命令必须以该目录为工作目录或子目录。
- 使用 `context.WithTimeout` 控制命令超时。
- 限制单任务最大执行时间。
- 限制输出日志大小，避免日志爆炸。
- 禁止明显危险命令，例如：
  - 删除系统目录。
  - 修改系统配置。
  - 后台常驻进程。
  - 网络扫描。
  - 访问工作区外路径。

#### Phase B：Docker 沙箱，适合正式版本

- 每个任务启动临时容器。
- 将 clone 后的仓库挂载到容器内。
- 限制 CPU、内存、磁盘、网络。
- 任务结束后销毁容器。
- 失败任务保留 workspace 和日志一段时间用于排查。

建议最终采用 Docker 沙箱作为正式方案。

### 3.3 TODO

- [ ] 增加 `runtime` 或 `sandbox` 模块，统一封装命令执行。
- [ ] 实现 `CommandRunner`：支持超时、工作目录校验、输出截断、错误码记录。
- [ ] 增加命令执行审计日志，写入 `operation_logs`。
- [ ] 增加 workspace 清理逻辑：成功立即清理，失败保留一段时间。
- [ ] 增加 Docker 沙箱配置项，例如：CPU、内存、网络开关、镜像名、最大执行时长。
- [ ] 所有写入型 Agent 必须走沙箱执行，不应直接在 Gateway 根目录执行命令。

---

## 四、Agent Token 权限收敛

### 4.1 设计结论

Agent Token 权限由用户在 Web UI 创建或配置 Agent 时选择。

不同类型 Agent 推荐默认权限不同：

| Agent 类型 | 推荐权限 | 说明 |
|---|---|---|
| CodeReview Agent | `read:repository`、`write:issue` | 读取 PR/Diff，评论审查结果 |
| Analyze Agent | `write:issue` | 读取 Issue 并评论、打标签 |
| Interaction Agent | `write:issue`，必要时 `read:repository` | 回复评论，必要时读取上下文 |
| Dev Agent | `read:repository`、`write:repository`、`write:issue` | clone、push 分支、创建 PR、评论 |
| Bugfix Agent | `read:repository`、`write:repository`、`write:issue` | 修复代码、创建 PR、评论 |

### 4.2 Gitea scopes 支持问题

当前实现中 `AdminCreateToken` 没有传 scopes，需要确认目标 Gitea 版本是否支持 token scopes。

如果 Gitea 版本支持 scopes：

- Web UI 提供权限多选框。
- 后端创建 Token 时传入 scopes。
- DB 记录 Agent 申请的权限集合。

如果 Gitea 版本不支持 scopes：

- 仍然在 Web UI 中记录“逻辑权限”。
- 后端执行前按 Agent 权限做应用层校验。
- 不允许低权限 Agent 调用写仓库、push、创建 PR 等操作。

### 4.3 安全注意事项

当前 `agents.gitea_token` 不应直接通过 API 返回给前端。

后续应：

- API response 使用 DTO，隐藏 `gitea_token`。
- 管理 API 增加认证。
- DB 中 Token 尽量加密存储。
- Token 只在真正调用 Gitea API 或 Git 操作时读取。

### 4.4 TODO

- [ ] 调研当前目标 Gitea 版本是否支持 token scopes。
- [ ] 扩展 `CreateAgentRequest`，增加 `permissions/scopes` 字段。
- [ ] 扩展 `agents` 表，记录 Agent 权限配置。
- [ ] 修改 `AdminCreateToken`，如果版本支持则提交 scopes。
- [ ] 所有执行器调用 Gitea 写操作前检查 Agent 权限。
- [ ] API 返回 Agent 信息时隐藏 `gitea_token`。
- [ ] 管理 API 增加认证。

---

## 五、任务队列设计

### 5.1 设计结论

短期坚持 SQLite + 内存队列，适合小规模单实例部署。

如果未来需要多实例、高可靠或更复杂调度，则需要重新设计队列和分布式锁。

### 5.2 当前适用场景

SQLite + 内存队列适合：

- 单进程 Gateway。
- 小团队使用。
- Agent 并发数较低。
- 任务量不大。
- 部署简单优先。

### 5.3 当前风险

当前队列模型存在几个风险：

1. 内存队列满时任务只落库，不会被当前进程继续捞取。
2. 多实例下可能重复消费 pending task。
3. 缺少任务租约/锁，无法可靠判断某任务是否被某 worker 占用。
4. 任务状态恢复能力不足，例如进程崩溃时 `running` 任务如何处理不明确。

### 5.4 后续增强方向

单实例增强：

- 后台定期扫描 pending task。
- 启动时恢复 pending / stale running 任务。
- `running` 超时后重置为 pending 或 failed。
- 队列满时可阻塞或降级为 DB 轮询。

多实例增强：

- 增加任务锁字段：`locked_by`、`locked_until`。
- 使用 SQLite 事务抢占任务。
- 或将队列替换为 Redis / NATS / RabbitMQ。

### 5.5 TODO

- [ ] 当前阶段保留 SQLite + 内存队列。
- [ ] 增加 pending task 后台扫描机制。
- [ ] 增加 stale running task 恢复机制。
- [ ] 明确队列满时策略：阻塞等待或 DB 后台扫描。
- [ ] 如果后续支持多实例，再引入任务锁或外部队列。

---

## 六、Webhook 去重语义

### 6.1 设计结论

Webhook 去重语义采用：**成功持久化任务后去重**。

也就是说，不能在事件刚解析完成时就立即标记 delivery 已处理。只有当事件成功转换为任务，并且任务成功写入 DB 后，才应标记该 delivery 已处理。

### 6.2 原因

如果在 dispatch 前就标记已处理，可能出现事件丢失：

```text
收到 Webhook
  -> 解析成功
  -> 标记 delivery 已处理
  -> 异步 dispatch
  -> 进程崩溃 / 入队失败
  -> Gitea 重试时被判定为重复
  -> 事件永久丢失
```

应改为：

```text
收到 Webhook
  -> 解析成功
  -> 路由匹配
  -> 创建 task
  -> task 写入 DB 成功
  -> 标记 delivery 已处理
  -> 返回 accepted
```

### 6.3 未匹配事件的处理

对于未匹配任何路由的事件，有两种选择：

1. 直接返回 `ignored`，并记录 delivery，避免重复重试。
2. 不记录 delivery，允许后续路由配置变更后重新处理。

建议 MVP 采用方案 1：

- 未匹配事件也记录为已处理。
- 额外写入操作日志：`webhook_ignored`。
- 避免 Gitea 重复投递造成噪音。

如果未来需要“补处理历史事件”，应设计单独的 replay 机制，而不是依赖 Gitea 重试。

### 6.4 TODO

- [ ] 调整 `webhook.Handler`，不要在 callback 前立即 `MarkProcessed`。
- [ ] 将去重标记移动到任务成功持久化之后。
- [ ] 对 ignored 事件记录审计日志。
- [ ] 评估是否扩展 `processed_deliveries` 表，增加 `status`、`event`、`repo`、`task_id`、`error` 字段。
- [ ] 增加 Webhook 去重与失败重试测试。

---

## 七、Prompt 模板来源

### 7.1 设计结论

需要在 `config.example.yaml` 和实际配置中增加 `agents.templates`。

原因：

- 用户通过 Web UI 创建自定义 Agent 时，需要可选择的默认模板。
- 不同 Agent 类型需要不同的 system prompt 和 user template。
- 当前 `llm-prompt-design.md` 已经设计了模板，但 `config.example.yaml` 尚未落地。

### 7.2 建议配置结构

建议增加类似配置：

```yaml
agents:
  defaults:
    provider: "deepseek"
    model: "deepseek-chat"
    max_tokens: 4096
    temperature: 0.3

  templates:
    analyze:
      name: "需求分析 Agent"
      type: "analyze"
      gitea_username: "ai-analyze"
      permissions:
        - "write:issue"
      system_prompt: |
        你是一个需求分析专家。你的任务是分析用户提交的 Issue，评估需求完整性和可行性。
      user_template: |
        请分析以下 Issue：

        ## Issue #{{.Issue.Number}}: {{.Issue.Title}}
        {{.Issue.Body}}

        请输出结构化需求分析报告。

    review:
      name: "Code Review Agent"
      type: "review"
      gitea_username: "ai-reviewer"
      permissions:
        - "read:repository"
        - "write:issue"
      system_prompt: |
        你是一个资深代码审查专家。请审查 PR 变更，输出结构化审查报告。
      user_template: |
        请审查以下 PR：

        ## PR #{{.PR.Number}}: {{.PR.Title}}
        {{.PR.Body}}

        ## Diff
        {{.PR.Diff}}

    dev:
      name: "研发 Agent"
      type: "dev"
      gitea_username: "ai-dev"
      permissions:
        - "read:repository"
        - "write:repository"
        - "write:issue"
      system_prompt: |
        你是一个高级研发工程师。请根据 Issue 需求修改代码，并通过 PR 交付。
      user_template: |
        请解决以下 Issue：

        ## Issue #{{.Issue.Number}}: {{.Issue.Title}}
        {{.Issue.Body}}

        仓库：{{.Repo.FullName}}
        默认分支：{{.Repo.DefaultBranch}}
```

### 7.3 Prompt 优先级

Prompt 来源优先级保持：

```text
Agent DB 自定义配置
  > config.yaml agents.templates
  > 系统内置兜底模板
```

### 7.4 TODO

- [ ] 扩展 `config/schema.go`，增加 `AgentsConfig`、`AgentTemplateConfig`。
- [ ] 修改 `config.example.yaml`，增加 `agents.defaults` 和 `agents.templates`。
- [ ] Agent 创建 API 支持从模板创建。
- [ ] 创建 Agent 时，如果请求未传 prompt/model/permissions，则从模板或默认值填充。
- [ ] 实现 Go template 渲染，将 Issue/PR/Repo/Comment/Task 上下文注入 `user_template`。
- [ ] 实现 Prompt 历史版本 CRUD。

---

## 八、建议开发优先级

### P0：先打通最小闭环

- [ ] 初始化 LLM Registry、Router、Queue、Executor。
- [ ] Webhook callback 接入 Router 和 Queue。
- [ ] 成功入队后再标记 delivery 已处理。
- [ ] 实现 `AnalyzeRunner` 或 `ReviewRunner`，先完成只读型 Agent。
- [ ] LLM 结果回写到 Gitea Issue/PR 评论。

### P1：安全与可靠性

- [ ] 管理 API 认证。
- [ ] API 隐藏 Agent Token。
- [ ] 修复 Task 状态更新时间。
- [ ] 队列 pending task 扫描。
- [ ] Gitea API 参数 URL escape。
- [ ] Agent 创建失败回滚。

### P2：写入型 Agent

- [ ] Workspace 管理。
- [ ] Git clone / branch / commit / push 封装。
- [ ] Shell 命令沙箱执行。
- [ ] PR 创建与评论回写。
- [ ] Dev Agent / Bugfix Agent 实现。

### P3：Prompt 与 Web UI

- [ ] 配置模板落地。
- [ ] Prompt API 和历史版本。
- [ ] Web UI 创建 Agent 时选择模板、模型、权限。
- [ ] Web UI 展示任务状态、日志、PR 链接。

### P4：正式化部署与测试

- [ ] README。
- [ ] Dockerfile。
- [ ] 单元测试。
- [ ] Webhook 到评论回写的端到端测试。
- [ ] Docker 沙箱正式启用。

---

## 九、当前结论

后续开发应优先避免继续堆 UI 或零散接口，而是先完成一个最小可用 Agent 闭环：

```text
Issue / PR Event
  -> 路由到 Agent
  -> 创建任务
  -> 执行只读型 Runner
  -> 调用 LLM
  -> 评论回写 Gitea
```

在该闭环稳定后，再推进写入型 Dev Agent / Bugfix Agent。写入型 Agent 是项目重头戏，但必须建立在沙箱、权限、任务状态、审计日志和 Gitea 回写能力都可靠的基础上。
