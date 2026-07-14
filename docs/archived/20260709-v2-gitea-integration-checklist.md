# v2 Gitea 联调 Checklist

> **分支**：`feature/assign-workflow-v2`  
> **适用提交**：`9030a90`（P0）→ `71ec850`（P1-2）  
> **设计依据**：[20260615-trigger-rules-and-workflow-improvement.md](./20260615-trigger-rules-and-workflow-improvement.md) · [ARCHITECTURE.md](../ARCHITECTURE.md)

本文档用于在**真实 Gitea 实例**上验证 v2 Assign 工作流。集成测试（15 项 workflow + policy/resolver 单测）已通过；本清单覆盖自动化测试无法替代的人工观测项。

---

## 0. 实现复核摘要（2026-06）

| 能力 | 状态 | 关键实现 |
|------|------|----------|
| PRID 全链路 | ✅ | `tasks.pr_id` · ReviewRunner 用 `task.PRID` · writeback 评论到 PR |
| PR merge 检测 | ✅ | `parser.PullRequest.Merged` · `resolvePRClosed` 读 `evt.PR.Merged` |
| PRID 回写 context | ✅ | DevRunner → executor → `onComplete(task.PRID)` → `OnTaskComplete` |
| L2 gate 接线 | ✅ | `coder_switch_agent` · `review_warn_if_draft`（含 Draft 字段） |
| L3 通知 | ✅ | analyze 完成 · PR 创建后 Issue 引导评论 |
| 可观测 API | ✅ | `GET /api/workflow-context?repo=&issue=` |
| setup 脚本 | ✅ | `scripts/setup-test.go` 已移除 routes，改为 v2 三 Agent |
| Web UI workflow 面板 | ❌ | 仅 API，Dashboard 仍看 tasks 间接推断 |

**Commit 链**：`9030a90` → `8ef1ac6` → `4e7530f` → `71ec850`

---

## 1. 前置环境

### 1.1 服务启动

- [ ] Gitea 可访问（示例：`http://localhost:3000`）
- [ ] Gateway 已构建并运行：`go build -o gateway . && ./gateway -config config.yaml`
- [ ] 健康检查：`GET http://localhost:8080/health` → 200
- [ ] Web UI 可登录：`admin` / `admin123`（或 `config.yaml` 中 `auth.default_admin_password`）
- [ ] `config.yaml` 已配置：
  - [ ] `gitea.url` / `admin_token` / `webhook_secret`
  - [ ] `llm.providers.*.api_key`（DevRunner 需要真实 LLM）
  - [ ] `workflow.preset: standard`（或 `strict` 测门禁）
  - [ ] `session.*`（可选，联调可保持默认）

### 1.2 测试仓库

- [ ] 在 Gitea 创建测试仓库，如 `owner/test-repo`
- [ ] 仓库有可 clone 的默认分支（`main`）及基础代码
- [ ] 当前账号对该仓库有 Assign / Request Review 权限

### 1.3 Webhook 配置

在仓库 **Settings → Webhooks** 添加：

| 项 | 值 |
|----|-----|
| URL | `http://<gateway-host>:8080/webhook/gitea` |
| Secret | 与 `config.yaml` → `gitea.webhook_secret` 一致 |
| Content Type | `application/json` |

**必须勾选的事件**：

- [ ] Issues
- [ ] Issue Comment
- [ ] Pull Request
- [ ] Pull Request Comment（PR 上 @mention 续作需要）

> 本地 Gitea → 本机 Gateway 时，确保 Gitea 能访问 Gateway 地址（Docker 内用 `host.docker.internal` 等）。

### 1.4 创建三个功能性 Agent

**推荐**：Web UI → Agents → 新建（会自动在 Gitea 创建用户 + Token）

| 字段 | analyze | coder | review |
|------|---------|-------|--------|
| name | analyze-agent | coder-agent | review-agent |
| gitea_username | analyze-agent | coder-agent | review-agent |
| role | analyze | coder | review |
| repos | 勾选测试仓库 | 同上 | 同上 |

- [ ] 三个 Agent 状态为 **active**
- [ ] 在 Gitea 仓库 **Collaborators** 中将三个 Agent 用户加为协作者（否则无法 Assign / Request Review）
- [ ] 记录各 Agent 的 `gitea_username`（Assign 时选的是 Gitea 用户，不是 Gateway 显示名）

**备选**：`go run scripts/setup-test.go` 直接向 DB 插入 Agent（需自行确保 Gitea 侧存在对应用户且 Token 有效）。

---

## 2. 主路径：Analyze → Coder → Review → Merge

使用**新 Issue** 跑通全链路。建议 Issue 标题含 `[v2-test]` 便于检索。

### 2.1 Assign Analyze

**操作**：在 Issue 上将 Assignee 设为 `analyze-agent`

**预期**：

- [ ] Gateway 日志：`Resolve` → `analyze_issue` 入队
- [ ] Web UI → Tasks：新任务 `task_type=analyze_issue`，`status=success`
- [ ] Gitea Issue 出现 Agent 分析报告评论（含 `<!-- gateway-agent -->` 标记）
- [ ] **L3**：Issue 出现引导评论「分析完成…建议 Assign coder Agent」
- [ ] API 验证：
  ```bash
  curl -H "Authorization: Bearer <JWT或dev-api-token>" \
    "http://localhost:8080/api/workflow-context?repo=owner/test-repo&issue=<N>"
  ```
  - [ ] `stage` = `analyzed`
  - [ ] `active_role` = `analyze`
  - [ ] `pr_id` = 0

### 2.2 Assign Coder（开 PR）

**操作**：同一 Issue 上将 Assignee 改为 `coder-agent`

**预期**：

- [ ] 任务 `task_type=solve_issue`（无 `bug` 标签）或 `fix_bug`（有 `bug` 标签）
- [ ] 任务完成后 `pr_id` > 0（Tasks API 或 DB）
- [ ] Gitea 出现新 **Pull Request**（PR body 含 `Fixes #<N>`）
- [ ] **L3**：Issue 出现「PR 已创建…建议 Request reviewer Agent」
- [ ] workflow-context：
  - [ ] `stage` = `developing`（或保持 developing）
  - [ ] `pr_id` = 实际 PR 编号

> **注意**：L3 PR 通知依赖任务结果含 `PR created: <url>` 格式；DevRunner 已按此输出。

### 2.3 Request Review

**操作**：在 **PR 页面** → Reviewers → 选择 `review-agent`（触发 `review_requested`）

**预期**：

- [ ] 任务 `task_type=review_pr`，且 **`pr_id` = PR 编号**（非 linked issue 号）
- [ ] 审查评论出现在 **PR** 上（不是 Issue）
- [ ] ReviewRunner 能正确拉取 PR diff（无 404）
- [ ] workflow-context（linked issue）：
  - [ ] `stage` = `reviewing`
  - [ ] `pr_id` 与 PR 编号一致

### 2.4 Merge PR

**操作**：在 Gitea 合并 PR

**预期**：

- [ ] Webhook：`pull_request` / `action=closed` / **`merged: true`**
- [ ] workflow-context：`stage` = `done`
- [ ] 相关 `agent_sessions` 状态 → `archived`
- [ ] Gateway 日志含 lifecycle archive 记录

---

## 3. 扩展场景

### 3.1 PR 关闭但未合并

**操作**：创建 PR 后 **Close** 但不 Merge

**预期**：

- [ ] `merged: false` → context **保持** `reviewing`（不转 `done`）
- [ ] Session **保留**（`pr_closed_retention` 策略）

### 3.2 Bug 修复路径

**操作**：新建 Issue，添加 `bug` 标签，Assign `coder-agent`（跳过 analyze）

**预期**（`preset: standard`）：

- [ ] `task_type=fix_bug`
- [ ] BugfixRunner 执行（非 solve_issue）

**预期**（`preset: strict`）：

- [ ] 无 analyze 时 Assign coder → **L2 硬拒绝** + Issue 门禁评论

### 3.3 @mention 续作

| 场景 | 操作 | 预期 task_type |
|------|------|----------------|
| Coder 改代码 | PR/Issue 评论 `@coder-agent 请修复…` | `solve_comment` |
| 分析讨论 | `@analyze-agent 补充…` | `reply_comment` |
| 强制 dev | `@coder-agent /dev …` | `solve_comment` |
| 强制回复 | `@analyze-agent /reply …` | `reply_comment` |

- [ ] Session 复用：同一 `(repo, issue, agent, role)` 不重复建 workspace
- [ ] Agent 自己的评论 **不触发** 新任务（防循环）

### 3.4 L2 软门禁 + /force

**前置**：`workflow.preset: standard`，在 `developing` 阶段再次 Assign 同一 coder

- [ ] 出现 **soft 警告**评论（`rerun_same_stage`）
- [ ] 任务仍入队（standard 下 soft 不阻断）

**操作**：评论 `@coder-agent /force 继续`

- [ ] 日志：`/force bypassing soft gate`
- [ ] 出现「已跳过软门禁」评论

### 3.5 Draft PR 审查（可选）

**操作**：创建 Draft PR，Request `review-agent`

- [ ] `preset: strict` 时 `review_warn_if_draft=soft` → 警告评论（仍允许 review）
- [ ] `preset: standard` 时 `review_warn_if_draft=off` → 无警告

### 3.6 Coder Agent 切换（可选）

**前置**：已有 active coder session，Assign **另一个** coder Agent

- [ ] standard：`coder_switch_agent=soft` → 警告后允许
- [ ] strict：`coder_switch_agent=hard` → 拒绝 + 门禁评论

### 3.7 L1：无 PR 时 Review

**操作**：在无 PR 的 Issue 上直接 Assign `review-agent`

- [ ] **拒绝**，Issue 评论说明需 open PR
- [ ] **无** 新 task 入队

### 3.8 重置工作流

**方式 A — 评论**：Issue 评论 `/gateway reset`

**方式 B — API**：
```bash
curl -X POST -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/sessions/reset?repo=owner/test-repo&issue=<N>"
```

**预期**：

- [ ] 该 issue 所有 session → `archived`
- [ ] workflow-context → `stage=idle`，`active_agent_id=0`
- [ ] 可重新从 Assign analyze 开始

### 3.9 Label 触发（应无效）

**操作**：给 Issue 打 `ai:analyze` 等旧 Label

- [ ] **不产生** 新 Task（v2 Breaking）

---

## 4. API 与 UI 验证清单

### 4.1 认证

- [ ] Web 登录后 JWT 可访问 `/api/stats`、`/api/agents`（authorizeWrap）
- [ ] 静态 Token `api.auth_token` 也可访问上述接口

### 4.2 工作流可观测

```bash
# 单个 Issue
GET /api/workflow-context?repo=owner/test-repo&issue=5

# 仓库下全部
GET /api/workflow-context?repo=owner/test-repo
```

- [ ] 返回 `stage`、`pr_id`、`active_agent_id`、`active_role`、`session_id`

### 4.3 Web UI

- [ ] Dashboard 任务列表刷新正常
- [ ] Agents 页 role 徽章显示正确
- [ ] Tasks 页可按 `task_type` / `status` 筛选
- [ ] ⚠️ workflow-context **尚无 UI 页面**，联调时用 API 或 `curl`

---

## 5. 日志与排错

### 5.1 正常日志关键词

```
[INFO] Resolve ... task_type=analyze_issue
[INFO] Task N created PR #M
[INFO] Lifecycle event: archive ... merged=true
[INFO] /force bypassing soft gate
```

### 5.2 常见问题

| 现象 | 排查 |
|------|------|
| Webhook 无反应 | Gitea Webhook 投递记录；Secret 是否一致；Gateway 是否可达 |
| Assign 无任务 | assignee 是否为 Agent 的 **gitea_username**；Agent 是否 active |
| 401 后跳登录页 | 重启 gateway（需 authorizeWrap）；清除浏览器 token 重登 |
| Review 404 / 空 diff | 检查 task.`pr_id` 是否为 PR 号；Fixes #N 场景 IssueID≠PRID |
| L3 PR 通知未出现 | task 是否 success；`pr_id>0`；result 含 `PR created: http://...` |
| Merge 后 stage 不变 | Webhook payload 是否含 `"merged": true`（非 `state: merged`） |
| DevRunner 超时 | 调大 `dispatcher.timeout`；检查 LLM API；看 sandbox 日志 |

### 5.3 建议观测点

- Gateway stdout / `./data/logs`
- Gitea → Webhook → Recent Deliveries
- SQLite：`workflow_contexts`、`agent_sessions`、`tasks`（`pr_id`、`session_id`）
- Web UI → Tasks 列表

---

## 6. 通过标准（Sign-off）

主路径 **必须全部打勾** 方可认为 v2 联调通过：

- [ ] 2.1 Analyze + L3
- [ ] 2.2 Coder + PR + L3 + `pr_id` 回写
- [ ] 2.3 Review 评论在 PR 上
- [ ] 2.4 Merge → `done` + session archived
- [ ] 3.9 Label 不触发
- [ ] 4.2 workflow-context API 与 stage 一致

扩展场景（3.x）按测试目标选做；**strict preset** 建议在第二次联调单独验证。

---

## 7. 已知限制（联调时不必阻塞）

| 项 | 说明 |
|----|------|
| workflow-context 无 Web UI | 用 API；后续可加 Dashboard 面板 |
| workspace 物理删除 | merge 后 archive；目录清理依赖 TTL/LRU，非即时 rm |
| `EnforceDiskLimit` | 当前不按 repo 分桶计量 |
| 无 mention 无 @ | 评论无 @Agent 时不 fallback 触发 |
| 集成测试 mock Gitea | 真实环境才能验证 PR diff / clone / push |
| Windows 沙箱单测 | `sh`/`echo` 相关单测可能失败，不影响 Gitea 联调 |

---

## 8. 快速命令参考

```bash
# 构建 & 运行
go build -o gateway . && ./gateway -config config.yaml

# 全量测试（Linux/CI 推荐）
go test ./... -count=1

# workflow 集成测试
go test ./tests/integration/ -v -run TestWebhook -count=1

# 插入测试 Agent（DB 直写，需 Gitea 用户已存在）
go run scripts/setup-test.go

# 查询 workflow context
curl -s -H "Authorization: Bearer dev-api-token" \
  "http://localhost:8080/api/workflow-context?repo=owner/test-repo&issue=1" | jq
```

---

**维护**：P0/P1 变更后更新 §0 与相关预期；联调报告可另存为 `docs/YYYYMMDD-v2-e2e-report.md`。
