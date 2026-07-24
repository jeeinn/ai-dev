# 任务清单

> 更新：2026-07-24  
> 产品边界：**Gitea 优先** · 内置 Agent 默认可用 · OpenCode 可选 · 不做多托管平台抽象  
> 决策：[archived/20260714-coding-gateway-multi-vcs.md](archived/20260714-coding-gateway-multi-vcs.md)  
> 已归档交付记录：  
> - P0–P2 核心演进 → [archived/20260716-TASKS.md](archived/20260716-TASKS.md)  
> - P3 开源 + 开源后加固 → [archived/20260723-TASKS.md](archived/20260723-TASKS.md)  
> - 写路径 / Agent 摩擦 / Bootstrap（Issue #4）→ [archived/20260724-TASKS.md](archived/20260724-TASKS.md)  
> 架构评估（点时）→ [archived/20260722-architecture-evaluation.md](archived/20260722-architecture-evaluation.md)

---

## 演进主线

```text
P0–P2 → P3 → 写路径/摩擦/Bootstrap（均已归档）
        │
        ├─► 【下一步】PR 续作注入 review / 评论历史
        ├─► 【随后】可观测性 WebUI（对话日志 → 审计日志 → local-only 高亮）
        └─► 更后：沙箱细项 · OpenCode A+ · LLM 可选 · 运维可选
```

---

## 1. PR 续作注入评论 / review 历史（优先）

来源：2026-07-24 实跑 [rust-study#2](http://182.92.129.124:3000/jeeinn/rust-study/pulls/2) — Request Reviewer 后在 PR 上 `@ai-coder 根据上面的 review 结果进行修改`，coder 只拿到触发评论正文，**未注入** ai-reviewer 的审查内容，续作 commit 与 review 建议无关。

- [ ] **`solve_comment`（及同类写路径续作）注入 PR 评论 / review 历史**  
  组装 `task.Context` / prompt 时，除触发评论外，拉取该 PR（`IssueComments`）近期评论，优先包含 review agent 的审查报告；截断策略可对齐 `InteractionRunner`（约最近 10 条）或按 token 预算裁剪。  
  目标：用户只需 `@coder 按 review 修改`，模型仍能看到具体意见，避免跑偏改无关文件。

---

## 2. 可观测性 / WebUI（其次）

> **系统日志说明（现状，非待办）**：运行日志走 stdout + 可选文件（bootstrap 已默认 `logging.path: "./data"` → `./data/matea.log`），**不入库**。入库的是 `operation_logs`（操作审计）与开启调试后的 `task_conversation_logs`（Agent 对话）。查系统日志请看终端或 `matea.log`。

- [ ] **Agent 对话日志查看（WebUI）**  
  现状：`debug.conversation_log.enabled` 可将多轮 LLM 消息写入 `task_conversation_logs`（store 已有 `ListConversationLogs`），但 **无 REST 读接口、无前端页面**。  
  需求：按 `task_id` 查询 API；WebUI 展示多轮对话（role / content / tool_calls）；**任务列表可跳转**至该 task 对话（详情入口或 `/tasks/:id/conversation`）。
- [ ] **（可选）操作审计日志 WebUI**  
  `operation_logs` 已入库且有 `GET /api/logs`，前端无页面。
- [ ] **（可选）WebUI 高亮未推送分支**  
  日志侧已有 local-only `[WARN]`；UI 高亮仍待做。

---

## 3. 更后 / 按需

### 运维可选

- [ ] （可选）bootstrap 启动日志打印「Logging to file: …」，确认文件落盘成功

### 沙箱可选增强

详见 [archived/20260604-sandbox-roadmap.md](archived/20260604-sandbox-roadmap.md)（核心已交付）

- [ ] （可选）`cat` 行号范围、`find` glob、审计日志内容摘要
- [ ] （可选）sandbox 拦截 agent 侧 `git commit` / `git push`，统一走 finalize

### OpenCode A+

设计：[server-runtime-design-v4.md](server-runtime-design-v4.md) §A+ · 清单归档：[archived/20260714-todo-opencode-path-a.md](archived/20260714-todo-opencode-path-a.md)

- [ ] SSE 进度 → Issue 评论或 task progress
- [ ] 持久化 `opencode_session_id`（Session 续作）
- [ ] OpenCode 集成测试：mock server + 假仓库
- [ ] Claude PrintBackend（契约型 CLI，非 Path B）

### LLM 可选增强

[todo-20260714-LLMProvider-可选增强.md](todo-20260714-LLMProvider-可选增强.md)

- [ ] tiktoken 精确计数（可选开启）
- [ ] 超长 Session 语义摘要
- [ ] per-task 成本预算上限

### 继续延后

| 项 | 说明 |
|----|------|
| 文件级 analyze 落地 checklist / README 链接自动核对 | soft gate 文案已归档交付；自动核对未做 |
| API 中间件链 | CORS / 限流 / 访问日志；有运维痛点再立项 |
| `gitea.Client` Transport 显式复用 | DefaultTransport 已够用 |

---

## 明确不做

| 项 | 说明 |
|----|------|
| GitHub / GitLab / Gitee 多平台 Host SPI | 中小团队 Gitea-first |
| Issue 级任意 PR base（label/body） | 边缘场景 |
| 远程 OpenCode / Path B worktree 基础设施 | v4 非目标；另议 |
| Gateway 反向做成 MCP Server | 后置，非当前 ToolPack/MCP 消费路径 |

---

## 现行文档（非归档）

| 文档 | 用途 |
|------|------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | 现行架构 |
| [DEPLOYMENT.md](DEPLOYMENT.md) | 部署 |
| [server-runtime-design-v4.md](server-runtime-design-v4.md) | OpenCode / CodingBackend 设计权威 |
| [todo-20260714-LLMProvider-可选增强.md](todo-20260714-LLMProvider-可选增强.md) | LLM 剩余可选增强 |
| [archived/20260724-TASKS.md](archived/20260724-TASKS.md) | 写路径 / 摩擦 / Bootstrap 交付记录 |
| [archived/20260723-TASKS.md](archived/20260723-TASKS.md) | P3 + 开源后加固交付记录 |
| [archived/](archived/) | 历史设计、签核、E2E、清单 |
