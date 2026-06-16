# Assign 工作流 v2 进度总览

> 设计依据：[trigger-rules-and-workflow-improvement.md](./trigger-rules-and-workflow-improvement.md)  
> 任务清单：[TASKS.md](./TASKS.md)

**最后更新**：2026-06-16 · **当前阶段**：Phase 19（P3）

---

## 总览

| 阶段 | 代号 | 状态 | 说明 |
|------|------|------|------|
| Phase 16 | P0 | ✅ 完成 | Assign 主路径 + 数据模型 + L1 门禁 |
| Phase 17 | P1 | ✅ 完成 | Session 续作 + WorkflowPolicy L2/L3 |
| Phase 18 | P2 | ✅ 完成 | 生命周期 + Web UI + routes 移除 |
| Phase 19 | P3 | ⏳ **当前** | 运维增强 + Agent 向导 + 文档/E2E |

**v2 验收清单：11/11 ✅**

---

## Phase 18（P2）✅

| 子阶段 | 内容 | 关键文件 |
|--------|------|----------|
| 18.1 | Session 生命周期：closed/merged → archive + context=done | `internal/workflow/lifecycle.go` |
| 18.2 | 磁盘 LRU：`EnforceDiskLimit` + `parseSize` + cleanup loop | `lifecycle.go` |
| 18.3 | Web UI：Agent role 字段；删触发规则页；routes Tab 弃用提示 | `Agents.vue`, `AgentDetail.vue`, `router/index.js` |
| 18.4 | routes 完全移除：`DROP TABLE routes`，删 route.go/router.go、API | `sqlite.go`, `api/router.go` |
| 18.5 | Gitea unassign | ⏭️ 已跳过 |
| 18.6 | 集成测试：`TestIssueClosedArchivesSession` | `workflow_test.go`, `lifecycle_test.go` |

### Phase 18 关键变更

- **Breaking**：`routes` 表已 `DROP`；`Router.Match` 与 legacy 流水线已移除
- **生命周期**：`issues.closed` / PR merged → archive sessions + `WorkflowContext.stage=done`
- **TTL**：后台 goroutine 每 10 分钟扫描 idle sessions + archived workspaces
- **磁盘 LRU**：超过 `max_disk_per_repo` 删除最旧 archived workspace
- **Web UI**：Agent 表单/列表新增 role；独立触发规则页已从路由移除

---

## Phase 19（P3）⏳ 下一步

| 子阶段 | 内容 | 优先级 |
|--------|------|--------|
| **19.1** | `/gateway reset` + `POST /api/sessions/reset` | P0 |
| **19.2** | Agent 创建向导（analyze/coder/review 模板） | P1 |
| 19.3 | 多仓库 WorkflowPolicy 覆盖 + 组织级 Webhook 文档 | P1 |
| **19.4** | README/ARCHITECTURE 迁移指南 + Assign 版 E2E 报告 | P0 |

**建议顺序**：19.4（文档收尾）与 19.1 并行 → 19.2 → 19.3

---

## 验收清单（全部通过）

| 项 | 阶段 |
|----|------|
| Assign analyze-* → analyzed | 16 |
| Assign coder-* → PR + Session workspace | 17 |
| bug 标签 → fix_bug | 16 |
| PR Request reviewer-* | 16 |
| @coder PR 评论续作 | 17 |
| standard/strict 门禁 | 17 |
| 无 PR review → L1 拒绝 | 16 |
| Agent 评论不循环 | 16 |
| PR merge workspace 回收 | **18** |
| labeled 不产生 Task | 16 |
| routes API/UI 移除 | **18** |

---

## 代码文件清单（Phase 16–18）

| 文件 | 用途 |
|------|------|
| `internal/store/workflow.go` | WorkflowContext CRUD |
| `internal/store/session.go` | AgentSession CRUD |
| `internal/workflow/resolver.go` | Event Resolver |
| `internal/workflow/context.go` | WorkflowManager 状态机 |
| `internal/workflow/gate_l1.go` | L1 门禁 |
| `internal/workflow/session.go` | SessionService |
| `internal/workflow/policy.go` | L2/L3 WorkflowPolicy |
| `internal/workflow/lifecycle.go` | Session 生命周期 + TTL + LRU |
| `tests/integration/workflow_test.go` | 13 项集成测试 |

---

## 遗留 polish（Phase 19 消化）

| 项 | 说明 |
|----|------|
| Dashboard 引导文案 | 仍写「配置触发规则」，改 Assign Playbook |
| `TriggerRules.vue` 文件 | 路由已删，源文件可清理 |
| `GET /api/workflow-context` | 可选 API，供 UI 状态面板 |
| README / ARCHITECTURE | 仍引用 Label 触发 → 19.4 |
| `scripts/setup-test.go` | 仍 INSERT routes，需更新 |

---

## Phase 19 入口条件 ✅

- [x] v2 核心流程 Assign → Session → 续作 → 生命周期 全链路可用
- [x] routes Breaking 变更已落地
- [x] 验收清单 11/11 通过
- [ ] 对外文档与 E2E 报告（**19.4 首要**）
