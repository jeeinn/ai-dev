# 任务执行文档

---

## 已完成

| 阶段 | 内容 | 归档 |
|------|------|------|
| Phase 1-13 | 项目骨架 → 集成收尾 | [20260604-TASKS.md](20260604-TASKS.md) |
| Phase 14.5 | Agent 迭代控制配置化 | [sandbox-roadmap.md](sandbox-roadmap.md) |
| Phase 15 | Web UI 优化 | [20260605-TASKS.md](20260605-TASKS.md) |
| Phase 16 | Assign 工作流 v2 — P0 主路径 | 见下方 Phase 16 归档说明 |

端到端测试报告：[20260605-e2e-test-report.md](20260605-e2e-test-report.md)

**Phase 16 归档说明**（2026-06-16）：`internal/workflow/`、`workflow_contexts` / `agent_sessions` 表、Event Resolver 主路径、L1 门禁、Label 触发移除、51+ 新测试已落地。遗留项见 Phase 16 各节 `[~]` 标记，不阻塞 Phase 17。

---

## 待开发

### Phase 14：沙箱增强

详见 [沙箱迭代计划](sandbox-roadmap.md)

- 14.1 临时目录模式
- 14.2 更丰富的上下文工具
- 14.3 配置化的超时和限制
- 14.4 安全增强

---

## Phase 16–19：Assign 工作流 v2

> **设计依据**：[trigger-rules-and-workflow-improvement.md](trigger-rules-and-workflow-improvement.md)
>
> **目标**：全面拥抱 Assign 触发；Gateway 维护 WorkflowContext + AgentSession；L1/L2/L3 门禁；Session 级 Workspace 续作。
>
> **原则**：Assign 触发 Who，WorkflowContext 定义 When，WorkflowPolicy 定义能不能转，AgentSession 支撑 Continue。**v2 不兼容 Label 阶段触发。**

| 阶段 | 代号 | 交付物概要 | 依赖 |
|------|------|------------|------|
| Phase 16 | P0 | ~~Assign 主路径 + 数据模型 + L1 门禁~~ ✅ | — |
| Phase 17 | P1 | Session 续作 + WorkflowPolicy L2/L3 | Phase 16 |
| Phase 18 | P2 | 生命周期回收 + Web UI + **routes/Label 触发移除** | Phase 17 |
| Phase 19 | P3 | 运维增强 + Agent 向导 + 多仓库 | Phase 18 |

---

### Phase 16（P0）：状态机与 Assign 主路径 ✅

**交付标准**：Issue Assign 功能性 Agent → 按 role 入队执行；PR Request Reviewer → review；L1 违规有评论拒绝；无 Agent 自触发、无同 issue 并发双 Task；**Label / labeled 事件不触发 Task**。**不含 @mention 续作**（Phase 17）。

#### 16.1 数据模型与迁移

- [x] `agents` 表增加 `role` 字段：`analyze` | `coder` | `review`（NOT NULL，默认 `analyze` 或迁移时按名称推断）
- [x] 新建 `workflow_contexts` 表：`repo`, `issue_id`, `pr_id`, `stage`, `active_agent_id`, `active_role`, `session_id`, `updated_at`；唯一索引 `(repo, issue_id)`（纯 PR 场景见设计 §5.2.1）
- [x] 新建 `agent_sessions` 表：见设计文档 §6.2；唯一索引 `(repo, issue_id, agent_id, role)` 或 `(session_id)` PK
- [x] `tasks` 表增加可选字段：`session_id`, `role`（便于追溯）
- [x] `internal/store`：CRUD + 单元测试
- [~] API：`GET/PUT` Agent 含 `role` ✅；`GET /api/workflow-context?repo=&issue=` **未实现**（Phase 18 UI 前补）

#### 16.2 Agent 注册与 API / UI

- [x] `CreateAgentRequest` / `Agent` 增加 `role`（API + `internal/agents/manager.go`）
- [~] Web UI：创建/编辑 Agent 时选择 role；列表展示 role 徽章 **未实现**（Phase 18.3）
- [~] 文档/README：功能性 Agent 命名示例 **未更新**（Phase 16.9 / 19.4）

#### 16.3 Event Resolver（新包 `internal/workflow` 或扩展 `dispatcher`）

- [x] 新建 `EventResolver`，`SetWorkflowComponents` 启用 v2 流水线
- [x] 解析 `issues.assigned`：**仅**使用 payload 中本次 `assignee.login`
- [x] assignee → `GetAgentByGiteaUsername`；非 Registry 用户 → 忽略
- [x] 由 `agent.role` 映射 `task_type`；Issue 含 `bug` → `fix_bug`
- [x] 解析 `pull_request` + `review_requested` / opened；PR 关联 Issue（`Fixes #N`）
- [x] `issues.unassigned` → 忽略
- [~] 流水线含 L1 + in-flight + 进度评论；**L2 / Session GetOrCreate → Phase 17**
- [x] Label 触发路径已移除（Resolver 忽略 `labeled`；Router 无 label 匹配）
- [x] `determineTaskType()` label 分支已删除

#### 16.4 WorkflowContext 状态机

- [x] `Transition(ctx, assignee, role)` + `ApplyTransition`
- [x] stage 枚举：`idle | analyzing | analyzed | developing | reviewing | done`
- [x] Assign analyze / coder；PR review_requested → reviewing
- [x] Task 成功回调：analyzing → analyzed；coder 写 `pr_id`
- [~] PR merged / Issue closed → done（**Phase 18.1**）
- [x] 单元测试（`context_test.go`）

#### 16.5 L1 结构性门禁

- [x] `internal/workflow/gate_l1.go`：`CheckL1`
- [x] `l1.review_requires_pr` / `l1.review_on_closed_pr`
- [~] `l1.dev_push_requires_branch`：预留（**Phase 17.3**）
- [x] `l1.assign_unknown_agent`：Resolver 忽略
- [x] `postGateComment` + `FormatAgentComment`（`<!-- gateway-agent -->`）

#### 16.6 并发与自触发防护

- [x] sender == 任意 active Agent → 跳过
- [x] in-flight 锁：`sync.Map` `(repo, issue_id)`
- [x] pending/running Task 去重

#### 16.7 集成测试

- [x] `TestWebhookIssueAssignedAnalyze`
- [x] `TestWebhookIssueAssignedCoder`
- [x] `TestWebhookAssignUnknownUser`
- [x] `TestWebhookAssignSelfTrigger`
- [x] `TestWebhookReviewRequested`
- [x] `TestL1ReviewNoPR`
- [x] `TestLabeledNoTask`
- [x] `TestBugLabelFixBug`

#### 16.8 弃用 Label 触发（v2 Breaking，与 16.3 同步）

- [x] `router.go` 移除 label 匹配（Phase 18 删 routes 表）
- [~] `HasLabel` 仍保留于 parser（legacy 测试）
- [x] 集成测试：`TestLabeledNoTask`
- [x] CHANGELOG Breaking

#### 16.9 文档

- [~] [ARCHITECTURE.md](ARCHITECTURE.md) **未更新**
- [~] `config.example.yaml` **未更新**
- [~] README Quick Start 仍引用 `ai:analyze` **未更新**

> 详见 [phase16-completion.md](phase16-completion.md)

---

### Phase 17（P1）：Session 续作与 WorkflowPolicy

**交付标准**：coder Task 不删 workspace；@mention 可续作；L2 可配置；Task 开始/结束有评论；analyze 完成后 L3 建议。

> **范围说明**：Phase 16 **不包含** @mention 续作（仍不触发或返回提示）；@mention 在 Phase 17.3 接入 Event Resolver 后生效。

#### 17.1 AgentSession 服务

- [ ] `SessionService.GetOrCreate(repo, issue, agent, role)`
- [ ] `SessionService.GetByIssue` / `GetByMentionTarget` / `GetActiveForIssue`
- [ ] Task 完成后：`session.status=idle`，更新 `branch`, `pr_id`, `last_task_id`, `last_active_at`
- [ ] analyze Session：无 workspace_path；coder Session：绑定 workspace

#### 17.2 Session 级 Workspace

- [ ] 目录布局：`{workspace.base_dir}/sessions/{session_id}/repo/`
- [ ] DevRunner：若 session.WorkspacePath 存在 → fetch + checkout branch，否则 clone 并写入 session
- [ ] **移除** Task 结束 `defer sb.Cleanup()`（coder）；改为 Session 归档时清理
- [ ] `task.BaseBranch` 改为优先读 `session.Branch`

#### 17.3 @Mention 续作

- [ ] `issue_comment` / `pull_request_comment`：解析 `@username`
- [ ] 路由优先级：显式 @ → WorkflowContext.active_agent → 最近 idle Session
- [ ] 行为：analyze role 或仅讨论 → `reply_comment`；coder + 有 branch/PR → `solve_comment`
- [ ] 评论 `/dev`、`/reply` 强制切换行为
- [ ] 接入 `InteractionHandler` 或合并进 EventResolver

#### 17.4 WorkflowPolicy L2 + L3

- [ ] `config.WorkflowPolicy` + `config.example.yaml` 默认值（preset: standard）
- [ ] `EvaluateGate(from, to, role, ctx, policy)` → `GateResult`
- [ ] 实现 gates：`coder_requires_analyzed`, `allow_skip_analyze`, `reanalyze_while_developing`, `rerun_same_stage`, `coder_switch_agent`, `review_warn_if_draft`
- [ ] 预设：`free` / `standard` / `strict` 一键展开
- [ ] L3 模板：`on_analyze_done`, `on_coder_pr_opened`, `on_gate_soft`, `on_gate_hard`
- [ ] 进度评论：Assign 后 `🔄 已开始`；完成 `✅` + 建议下一步

#### 17.5 /force 与 soft 绕过

- [ ] 评论 `@agent /force` 或 body 含 `/force`：soft 门禁跳过警告入队
- [ ] hard / L1 不可 force

#### 17.6 测试

- [ ] Session 复用：连续两次 coder Task 不重复 clone
- [ ] @mention 续作 PR 评论 → solve_comment
- [ ] L2 hard：`coder_requires_analyzed` 拒绝 + 评论
- [ ] L2 soft：警告 + 仍入队

---

### Phase 18（P2）：生命周期、Web UI、routes 移除

**交付标准**：PR 合并 / Issue 关闭归档 Session；可配置 TTL；UI 可见工作流状态；**routes 表/API/UI 与 Label 触发代码完全移除**。

#### 18.1 Session 生命周期

- [ ] 监听 `issues.closed` → WorkflowContext=done，archive Sessions
- [ ] 监听 `pull_request.closed`：merged → archive + 延迟删 workspace；未合并 → 保留 `pr_closed_retention`
- [ ] 后台 goroutine：扫描 `last_active_at` 超 `idle_ttl` → archive
- [ ] archived + 超过 `workspace_retention` → 删除目录
- [ ] `config.session`：`idle_ttl`, `workspace_retention`, `pr_closed_retention`

#### 18.2 磁盘与 LRU（基础）

- [ ] 统计 repo 下 session 目录总大小
- [ ] 超 `max_disk_per_repo` → 删除最旧 archived workspace

#### 18.3 Web UI

- [ ] Agent 表单：role 必填
- [ ] **删除**「触发规则」独立页（`TriggerRules.vue`）、Layout 菜单项、Agent 详情「触发规则」Tab 与快捷 `ai:*` 按钮
- [ ] 新页或 Dashboard 卡片：工作流策略 preset（free/standard/strict）
- [ ] Issue/任务详情：展示 stage、active agent、session_id、session 状态
- [ ] Dashboard 引导步骤：「Assign Agent」替代「配置触发规则」
- [ ] Playbook 链接到设计文档 §14；迁移说明链到 §11.2

#### 18.4 routes 表与 API 移除（v2 Breaking）

- [ ] 删除 API：`GET/POST/DELETE /api/routes`、`GET /api/agents/{id}/routes`
- [ ] DB migration：`DROP TABLE routes`（或迁移前 `scripts/export-routes.go` 导出 JSON 备份）
- [ ] 删除 `internal/store/route.go` 及 Router 对 DB routes 的依赖
- [ ] 删除 Web `TriggerRules.vue`、相关 API 调用、`agentRouteCounts` 等
- [ ] 更新 [gitea-ai-agent-design.md](gitea-ai-agent-design.md) Label 路由段落为 superseded

#### 18.5 可选：Gitea unassign 上一 Agent

- [ ] stage 切换时 API unassign 上一 Agent 账号（可配置开关）

#### 18.6 测试

- [ ] Issue closed → session archived
- [ ] PR merged → workspace 延迟删除
- [ ] idle_ttl 过期 archive

---

### Phase 19（P3）：增强与运维

**交付标准**：运维可 reset；多仓库策略；创建 Agent 有 role 模板向导。

#### 19.1 运维命令

- [ ] 评论 `/gateway reset` 或 API：`POST /api/sessions/reset?repo=&issue=` → archive + 删 workspace + context=idle
- [ ] `rerun_same_stage` policy 与 reset 联动文档

#### 19.2 Agent 创建向导

- [ ] 模板：analyze / coder / review 一键创建 Agent + Gitea 用户 + 默认 Prompt
- [ ] 可选：创建时选关联 repos + 协作者

#### 19.3 多仓库与组织

- [ ] WorkflowPolicy 按 repo 覆盖（DB 或 config `[[repos]]`）
- [ ] Event Resolver repo 过滤与 Agent.Repos 一致
- [ ] 文档：组织级 Webhook 注册指引（Phase 2）

#### 19.4 文档、迁移与 E2E

- [ ] 发布 **v2 迁移指南**：Label/route 用户对照表（见设计文档 §11.2）
- [ ] 更新 README：Assign 工作流 Quick Start；Breaking Changes 专节
- [ ] 端到端测试报告：Feature + Bug 全流程（Assign 版，无 Label 触发）
- [ ] 同步 ARCHITECTURE；确认无文档再引用 `ai:analyze` 作触发

---

## 执行顺序建议

```
Phase 16.1 → 16.2 → 16.3+16.8 → 16.4 → 16.5 → 16.6 → 16.7 → 16.9
    ↓
Phase 17.1 → 17.2 → 17.3 → 17.4 → 17.5 → 17.6
    ↓
Phase 18.1 → 18.2 → 18.3 → 18.4 → 18.5 → 18.6
    ↓
Phase 19.x（按需并行）
```

Phase 14（沙箱增强）与 Phase 16+ **可并行**，但 DevRunner Session 改造（17.2）与沙箱临时目录（14.1）存在交集，合并时需统一 workspace 路径约定。

---

## 验收清单（v2 整体）

- [ ] Assign `analyze-*` → 分析报告评论，WorkflowContext.stage=analyzed
- [ ] Assign `coder-*` → PR，Session 保留 workspace
- [ ] Assign `coder-*` on Issue 带 `bug` 标签 → `fix_bug` Task
- [ ] PR Request `reviewer-*` → 审查评论，WorkflowContext.stage=reviewing
- [ ] @coder 在 PR 评论 → push 更新，复用 Session
- [ ] 未 analyze 直接 coder：standard 允许，strict 拒绝
- [ ] 无 PR 时 review：L1 拒绝并评论
- [ ] Agent 评论不触发循环
- [ ] PR merge 后 workspace 回收
- [ ] `labeled` + `ai:*` 事件 **不产生 Task**
- [ ] routes API / 触发规则 UI **已移除**
