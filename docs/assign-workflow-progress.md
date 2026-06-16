# Assign 工作流 v2 — 完成总览

> 设计依据：[trigger-rules-and-workflow-improvement.md](./trigger-rules-and-workflow-improvement.md)  
> 任务清单：[TASKS.md](./TASKS.md)（待办）· 归档：[20260616-TASKS.md](./20260616-TASKS.md)

**状态**：✅ **v2 全部完成**（2026-06-16）  
**验收清单**：**11/11** 通过

---

## 阶段完成状态

| 阶段 | 代号 | 状态 | 提交数 |
|------|------|------|--------|
| Phase 16 | P0 | ✅ 完成 | 10 |
| Phase 17 | P1 | ✅ 完成 | 6 |
| Phase 18 | P2 | ✅ 完成 | 6 |
| Phase 19 | P3 | ✅ 核心完成 | 3 |

---

## 验收清单（11/11）

| # | 项 | 阶段 |
|---|-----|------|
| 1 | Assign analyze-* → 分析报告，stage=analyzed | 16 |
| 2 | Assign coder-* → PR，Session 保留 workspace | 17 |
| 3 | bug 标签 → fix_bug | 16 |
| 4 | PR Request reviewer-* → 审查评论 | 16 |
| 5 | @coder PR 评论 → push 续作 | 17 |
| 6 | standard 允许 / strict 拒绝跳过 analyze | 17 |
| 7 | 无 PR review → L1 拒绝 | 16 |
| 8 | Agent 评论不循环 | 16 |
| 9 | PR merge 后 workspace 回收 | 18 |
| 10 | labeled + ai:* 不产生 Task | 16 |
| 11 | routes API / 触发规则 UI 已移除 | 18 |

---

## 新增代码（Phase 16–19）

### `internal/store`

| 文件 | 用途 |
|------|------|
| `workflow.go` | WorkflowContext CRUD + 阶段转换 |
| `session.go` | AgentSession CRUD + 生命周期 |
| `store_test.go` | 18 个单元测试 |

### `internal/workflow`

| 文件 | 用途 |
|------|------|
| `resolver.go` | Event Resolver（Assign / PR / @mention / lifecycle / reset） |
| `context.go` | WorkflowManager 状态机 |
| `gate_l1.go` | L1 结构性门禁 |
| `policy.go` | WorkflowPolicy L2 + L3（free/standard/strict） |
| `session.go` | SessionService |
| `lifecycle.go` | Session 生命周期 + TTL + 磁盘 LRU |
| `*_test.go` | resolver / context / gate / session / policy / mention / lifecycle |

### 集成测试

| 文件 | 数量 |
|------|------|
| `tests/integration/workflow_test.go` | 13 项 |

---

## 测试覆盖

| 包 | 测试数 |
|----|--------|
| `internal/store` | 18 |
| `internal/workflow` | 70+ |
| `tests/integration` (workflow) | 13 |
| **全量** `go test ./...` | ✅ 通过 |

---

## 各阶段要点摘要

### Phase 16 — Assign 主路径
Agent `role`、WorkflowContext / AgentSession 表、Event Resolver、L1 门禁、Label 触发移除（Breaking）

### Phase 17 — Session 续作
Session 级 Workspace、@mention + `/dev` `/reply` `/force`、WorkflowPolicy L2/L3

### Phase 18 — 生命周期与 UI
closed/merged → archive、TTL + LRU、Web UI role、routes 表 DROP

### Phase 19 — 运维与文档（核心）
- `/gateway reset` 评论 + `POST /api/sessions/reset`
- Agent 创建：role + 内置 Prompt 模板 + 关联 repos
- `ARCHITECTURE.md` v2 流程图与 Assign 模型说明
- 迁移对照见设计文档 §11.2

---

## 可选后续（非 v2 阻塞）

| 项 | 说明 |
|----|------|
| Phase 14 | 沙箱增强（独立路线图） |
| 18.5 | Gitea unassign 上一 Agent（已跳过） |
| 19.3 | WorkflowPolicy 按 repo DB 覆盖 |
| UI | Issue 详情 WorkflowContext 面板、`GET /api/workflow-context` |
| 清理 | 删除未挂载的 `TriggerRules.vue` 源文件 |

---

## 核心原则（已实现）

**Assign 触发 Who → WorkflowContext 定义 When → WorkflowPolicy 定义能不能转 → AgentSession 支撑 Continue**

v2 **不兼容** Label 阶段触发；流程顺序由 Repo 级门禁配置，Agent 评论引导下一步。
