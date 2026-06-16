# 任务执行文档

> 当前待办：**Phase 14 沙箱增强** + 下方可选项。  
> v2 Assign 工作流（Phase 16–19）已于 2026-06-16 完成，见 [20260616-TASKS.md](20260616-TASKS.md)。

---

## 已完成（归档）

| 阶段 | 内容 | 归档 |
|------|------|------|
| Phase 1–13 | 项目骨架 → 集成收尾 | [20260604-TASKS.md](20260604-TASKS.md) |
| Phase 14.5 | Agent 迭代控制配置化 | [sandbox-roadmap.md](sandbox-roadmap.md) |
| Phase 15 | Web UI 优化 | [20260605-TASKS.md](20260605-TASKS.md) |
| Phase 16–19 | Assign 工作流 v2 | [20260616-TASKS.md](20260616-TASKS.md) |

参考：[assign-workflow-progress.md](assign-workflow-progress.md) · [trigger-rules-and-workflow-improvement.md](trigger-rules-and-workflow-improvement.md) · [ARCHITECTURE.md](ARCHITECTURE.md)

端到端测试报告：[20260605-e2e-test-report.md](20260605-e2e-test-report.md)（v2 主路径见 `tests/integration/workflow_test.go` 13 项）

---

## 待开发

### Phase 14：沙箱增强

详见 [沙箱迭代计划](sandbox-roadmap.md)

- [ ] 14.1 临时目录模式（与 Session 级 workspace 路径约定需对齐）
- [ ] 14.2 更丰富的上下文工具
- [ ] 14.3 配置化的超时和限制
- [ ] 14.4 安全增强

---

## 可选 / 后续

非 v2 阻塞；按需排期。

### UI / API

- [ ] `GET /api/workflow-context?repo=&issue=` — Issue/任务详情展示 stage、active agent、session 状态
- [ ] Web UI：工作流策略 preset（free/standard/strict）配置页（当前仅 `config.yaml`）
- [ ] 删除未挂载的 `TriggerRules.vue` 源文件
- [ ] AgentDetail 移除「触发规则」弃用 Tab（或合并为 Playbook 链接）

### 工作流 / 多仓库

- [ ] **WorkflowPolicy 按 repo DB 覆盖** — 同一 Gateway 多仓库时，各 repo 独立 L2 门禁（全局 preset + per-repo 覆盖）；见设计文档 §5.7.2
- [ ] 阶段切换时 Gitea unassign 上一 Agent（设计 §5.6 / 原 18.5，已跳过）
- [ ] 组织级 Webhook 注册指引补充（DEPLOYMENT 扩展）

### 运维 / 文档

- [ ] 独立 v2 E2E 测试报告（Feature + Bug Assign 全流程）
- [ ] `scripts/setup-test.go` 移除对已删除 `routes` 表的 INSERT
- [ ] `webhook/parser.go` 清理仅 legacy 使用的 `HasLabel`（若测试不再依赖）

### Agent 创建

- [ ] Agent 创建「向导」：按 role 一键填充 analyze/coder/review 默认 Prompt + 命名建议（当前为模板下拉 + 手动创建）
