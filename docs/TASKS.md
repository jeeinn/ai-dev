# 任务清单

> 更新：2026-07-17  
> 产品边界：**Gitea 优先** · 内置 Agent 默认可用 · OpenCode 可选 · 不做多托管平台抽象  
> 决策：[archived/20260714-coding-gateway-multi-vcs.md](archived/20260714-coding-gateway-multi-vcs.md)  
> P0–P2 核心演进已归档：[archived/20260716-TASKS.md](archived/20260716-TASKS.md)

---

## 演进主线

```text
P0–P2 核心能力（已交付）
        │
        └─► P3 开源准备 ──► 首发后：沙箱补强 / 产品打磨 / Harness 验证
```

---

## P3 — 开源准备

详单：[OPEN-SOURCE-CHECKLIST.md](OPEN-SOURCE-CHECKLIST.md)

### 11. 开源阻塞（首发前必须）

- [x] LICENSE 文件（MIT，与 README 一致）
- [x] CI：`.github/workflows` 跑 `go test ./... -count=1` + `go vet ./...`
- [x] 清理 `DEPLOYMENT.md` 等占位 URL（`your-org`）
- [x] README / DEPLOYMENT 安全默认值警示（改默认密码、`jwt_secret`）
- [x] 敏感配置：`config.example.yaml` 完整；可选 `.env.example`；勿提交 token
- [x] README「5 分钟 Mock 测试」+ Gitea 完整联调入口

### 12. 强烈建议（首发前质量加固）

- [x] v2 联调 Sign-off：补 **Merge → `done`**（[20260717-v2-merge-signoff.md](20260717-v2-merge-signoff.md)；Mock + E13）
- [x] Session / Sandbox **双轨 `base_dir` 对齐**
- [x] `loop_config` 启动参数校验（`max_iterations` 1–100、`timeout` 1m–1h）
- [x] Dockerfile：**文档澄清：示例暂未提供**（短期不做 compose/K8s；见 [DEPLOYMENT.md](DEPLOYMENT.md)）
- [x] Linux E2E：`scripts/linux/e2e-smoke.sh`；TESTING 说明完整 Assign E2E 以 Windows/pwsh 为主
- [x] 文档同步：`sandbox-roadmap.md`、`todo-20260714-opencode-path-a.md`、内部能力验收状态
- [x] `CONTRIBUTING.md` + `SECURITY.md`
- [x] 首版 Release **准备**：CHANGELOG `[0.10.0]` + [RELEASE-v0.10.0.md](RELEASE-v0.10.0.md)（tag/二进制待维护者推送；跳过已有 v0.2–v0.7.0）

---

## P1 — 遗留（不阻塞开源）

详见 [sandbox-roadmap.md](sandbox-roadmap.md)

### 6. 沙箱补强

- [ ] `rg` 工具（Analyze 读仓效率）
- [ ] temp 模式清理与 Session workspace 生命周期对齐
- [ ] （可选）`cat` 行号范围、`find` glob、审计日志内容摘要

---

## P2 — 增强（开源后 / 按需）

### 13. OpenCode A+

设计：[server-runtime-design-v4.md](server-runtime-design-v4.md) §A+ · [todo-20260714-opencode-path-a.md](todo-20260714-opencode-path-a.md)

- [ ] SSE 进度 → Issue 评论或 task progress
- [ ] 持久化 `opencode_session_id`（Session 续作）
- [ ] OpenCode 集成测试：mock server + 假仓库
- [ ] Claude PrintBackend（契约型 CLI，非 Path B）

### 14. LLM 可选增强

[todo-20260714-LLMProvider-可选增强.md](todo-20260714-LLMProvider-可选增强.md)

- [ ] tiktoken 精确计数（可选开启）
- [ ] 超长 Session 语义摘要
- [ ] per-task 成本预算上限

### 15. 产品打磨

完整列表见 [archived/20260714-TASKS.md](archived/20260714-TASKS.md)、[archived/20260716-TASKS.md](archived/20260716-TASKS.md) §10。

- [ ] `AgentDetail.vue` 移除「触发规则」弃用 Tab
- [ ] Agent 创建向导（按 role 一键填 Prompt）
- [ ] WorkflowPolicy per-repo DB 覆盖
- [ ] 阶段切换 Gitea unassign（设计曾标可选跳过）
- [ ] 组织级 Webhook 注册指引（DEPLOYMENT 扩展）

### 16. Harness 验证（建议 backlog）

- [ ] Dev/Bugfix：测试/构建通过作为程序化完成条件
- [ ] Review 与 Coder 独立 Checker 上下文（防自评）
- [ ] Loop 无进展退出（连续 N 轮无 diff / 测试不过则停）

---

## 明确不做

| 项 | 说明 |
|----|------|
| GitHub / GitLab / Gitee 多平台 Host SPI | 中小团队 Gitea-first |
| Issue 级任意 PR base（label/body） | 边缘场景 |
| 远程 OpenCode / Path B worktree 基础设施 | v4 非目标；另议 |
| Gateway 反向做成 MCP Server | 后置，非当前 ToolPack/MCP 消费路径 |

---

## 建议节奏

| 阶段 | 焦点 |
|------|------|
| 第 1 周 | P3.11 开源阻塞 + CONTRIBUTING/SECURITY |
| 第 2 周 | P3.12 质量加固 + v0.10.0 Release |
| 之后 | P1.6 沙箱 / P2 OpenCode A+ / 产品打磨 / Harness |

---

## 关键文档

| 文档 | 用途 |
|------|------|
| [OPEN-SOURCE-CHECKLIST.md](OPEN-SOURCE-CHECKLIST.md) | 开源阻塞 + 强烈建议详单 |
| [20260717-v2-merge-signoff.md](20260717-v2-merge-signoff.md) | S1 Merge→done Sign-off |
| [RELEASE-v0.10.0.md](RELEASE-v0.10.0.md) | v0.10.0 发布步骤 |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 现行架构 |
| [DEPLOYMENT.md](DEPLOYMENT.md) | 部署 |
| [20260716-e2e-test-report.md](20260716-e2e-test-report.md) | 本机 E2E 报告 |
| [server-runtime-design-v4.md](server-runtime-design-v4.md) | OpenCode / CodingBackend |
| [sandbox-roadmap.md](sandbox-roadmap.md) | 沙箱细项 |
| [archived/20260716-TASKS.md](archived/20260716-TASKS.md) | P0–P2 交付记录 |
| [archived/](archived/) | 历史设计与旧 TASKS |
