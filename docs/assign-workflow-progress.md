# Assign 工作流 v2 进度总览

> 设计依据：[trigger-rules-and-workflow-improvement.md](./trigger-rules-and-workflow-improvement.md)  
> 任务清单：[TASKS.md](./TASKS.md)

**最后更新**：2026-06-16 · **当前阶段**：Phase 18（P2）

---

## 总览

| 阶段 | 代号 | 状态 | 说明 |
|------|------|------|------|
| Phase 16 | P0 | ✅ 完成 | Assign 主路径 + 数据模型 + L1 门禁 |
| Phase 17 | P1 | ✅ 完成 | Session 续作 + WorkflowPolicy L2/L3 |
| Phase 18 | P2 | ⏳ 待开始 | 生命周期 + Web UI + routes 移除 |
| Phase 19 | P3 | ⏳ 待开始 | 运维增强 + Agent 向导 + 多仓库 |

---

## Phase 16（P0）✅

| 子阶段 | 内容 | 关键文件 |
|--------|------|----------|
| 16.1 | 数据模型：`agents.role`、`workflow_contexts`、`agent_sessions`、`tasks` 扩展 | `internal/store/workflow.go`, `session.go` |
| 16.2 | Agent API：`role` 字段 | `internal/api/router.go`, `internal/agents/manager.go` |
| 16.3 | Webhook 解析：`assignee` + `requested_reviewers` | `internal/webhook/parser.go` |
| 16.4 | Event Resolver（替代 Router.Match + determineTaskType） | `internal/workflow/resolver.go` |
| 16.5 | WorkflowContext 状态机 | `internal/workflow/context.go` |
| 16.6 | L1 结构性门禁 | `internal/workflow/gate_l1.go` |
| 16.7 | Dispatcher v2 流水线 + sender 过滤 + in-flight 锁 | `internal/dispatcher/dispatcher.go` |
| 16.8 | 8 个集成测试（Assign / review / L1 / labeled / bug） | `tests/integration/workflow_test.go` |
| 16.9 | 移除 Label 触发代码（Breaking） | `router.go`, `CHANGELOG.md` |
| 16.10 | 文档：`config.example.yaml` workflow/session 注释 | README/ARCHITECTURE → Phase 19.4 |

---

## Phase 17（P1）✅

| 子阶段 | 内容 | 关键文件 |
|--------|------|----------|
| 17.1 | SessionService：GetOrCreate、CompleteTask、Archive | `internal/workflow/session.go` |
| 17.2 | Session 级 Workspace：DevRunner 复用、coder 不 Cleanup | `internal/agents/runners.go` |
| 17.3 | @Mention 续作：comment 解析 + `/dev`、`/reply` | `internal/workflow/resolver.go` |
| 17.4 | WorkflowPolicy L2 + L3：3 预设 + EvaluateGate + 评论模板 | `internal/workflow/policy.go` |
| 17.5 | `/force` 绕过软门禁 | `dispatcher.go` + `resolver.go` |
| 17.6 | 测试 + bugfix（sessionID 作用域） | 见下方测试矩阵 |

---

## Phase 18（P2）⏳ 下一步

| 子阶段 | 内容 | 优先级 |
|--------|------|--------|
| **18.1** | Session 生命周期：`issues.closed` / `pull_request.merged` → archive + TTL | P0 |
| 18.2 | 磁盘 LRU：`max_disk_per_repo` | P1 |
| **18.3** | Web UI：Agent role、删除触发规则页、工作流状态面板 | P0 |
| **18.4** | routes 表/API/UI 完全移除（Breaking） | P0 |
| 18.5 | 可选：阶段切换时 Gitea unassign 上一 Agent | P2 |
| 18.6 | 生命周期集成测试 | P0 |

**建议实施顺序**：18.1 → 18.3（role UI）→ 18.4 → 18.6 → 18.2 → 18.5

---

## 新增文件清单（Phase 16–17）

| 文件 | 用途 | 测试 |
|------|------|------|
| `internal/store/workflow.go` | WorkflowContext CRUD + 阶段转换 | store_test |
| `internal/store/session.go` | AgentSession CRUD + 生命周期 | store_test |
| `internal/workflow/resolver.go` | Event Resolver（Assign / PR / @mention） | resolver_test (18) |
| `internal/workflow/context.go` | WorkflowManager 状态机 | context_test (9) |
| `internal/workflow/gate_l1.go` | L1 结构性门禁 | gate_l1_test (8) |
| `internal/workflow/session.go` | SessionService | session_test (7) |
| `internal/workflow/policy.go` | WorkflowPolicy L2 + L3 | policy_test (18) |
| `internal/workflow/mention_test.go` | @mention 解析 | mention_test (10) |
| `tests/integration/workflow_test.go` | 端到端 Assign 工作流 | 12 项 |

---

## 测试矩阵

```
internal/store       18 tests
internal/workflow    ~70 tests（resolver + context + gate + session + policy + mention）
tests/integration    12 workflow tests
```

---

## 遗留项（Phase 18/19 消化）

| 项 | 目标阶段 |
|----|----------|
| `GET /api/workflow-context` | 18.3 |
| Web UI Agent role 选择 | 18.3 |
| README Assign Quick Start（移除 `ai:analyze`） | 19.4 |
| ARCHITECTURE Event Resolver 流程图 | 19.4 |
| `handleEventLegacy` + routes 表删除 | 18.4 |
| PR merge 后 workspace 回收 | 18.1 |

---

## Phase 18 入口条件 ✅

- [x] Session GetOrCreate 已接入 Dispatcher 入队
- [x] DevRunner Session 级 Workspace，coder 不 Cleanup
- [x] @mention → solve_comment / reply_comment
- [x] L2 门禁 + /force + L3 评论
- [ ] Issue closed / PR merged 事件处理（**18.1 首要任务**）
