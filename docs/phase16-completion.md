# Phase 16 完成核对（2026-06-16）

> 对照 [trigger-rules-and-workflow-improvement.md](./trigger-rules-and-workflow-improvement.md) 与代码实现。

## 已实现

| 模块 | 文件 | 状态 |
|------|------|------|
| Event Resolver | `internal/workflow/resolver.go` | ✅ |
| WorkflowContext 状态机 | `internal/workflow/context.go` | ✅ |
| L1 门禁 | `internal/workflow/gate_l1.go` | ✅ |
| Store CRUD | `internal/store/workflow.go`, `session.go` | ✅ |
| Dispatcher v2 流水线 | `internal/dispatcher/dispatcher.go` | ✅ |
| 集成测试 | `tests/integration/workflow_test.go` | ✅ 8 项 |
| CHANGELOG Breaking | `CHANGELOG.md` | ✅ |

## 遗留（不阻塞 Phase 17）

| 项 | 计划阶段 |
|----|----------|
| Web UI Agent role 选择 | Phase 18.3 |
| `GET /api/workflow-context` | Phase 18.3 前 |
| ARCHITECTURE / README / config.example | Phase 16.9 / 19.4 |
| Session GetOrCreate 接入流水线 | Phase 17.1 |
| L2 WorkflowPolicy | Phase 17.4 |
| @mention 续作 | Phase 17.3 |
| PR merged / Issue closed → done | Phase 18.1 |
| routes 表/API 完全删除 | Phase 18.4 |

## Phase 17 入口条件

- [x] Assign → role → task_type 主路径可用
- [x] WorkflowContext stage 转换 + Task 完成回调
- [x] `agent_sessions` 表 CRUD 就绪（尚未 wired 到 Runner）
- [x] Label 触发已移除
- [ ] DevRunner 仍 `defer Cleanup()` — **17.2 首要改造**
