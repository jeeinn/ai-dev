# 任务清单

> 更新：2026-07-23  
> **状态：暂缓推进** — 已完成项与点时报告已归档；本文件仅保留未完成的按需 backlog。  
> 产品边界：**Gitea 优先** · 内置 Agent 默认可用 · OpenCode 可选 · 不做多托管平台抽象  
> 决策：[archived/20260714-coding-gateway-multi-vcs.md](archived/20260714-coding-gateway-multi-vcs.md)  
> 已归档交付记录：  
> - P0–P2 核心演进 → [archived/20260716-TASKS.md](archived/20260716-TASKS.md)  
> - P3 开源 + 开源后加固 → [archived/20260723-TASKS.md](archived/20260723-TASKS.md)  
> 架构评估（点时）→ [archived/20260722-architecture-evaluation.md](archived/20260722-architecture-evaluation.md)

---

## 演进主线

```text
P0–P2（已归档）→ P3 开源 + 加固（已归档）
        │
        └─► 按需 backlog（暂缓）：OpenCode A+ / LLM 可选 / 沙箱细项
```

---

## 按需 backlog（未完成）

### 沙箱可选增强

详见 [archived/20260604-sandbox-roadmap.md](archived/20260604-sandbox-roadmap.md)（核心已交付）

- [ ] （可选）`cat` 行号范围、`find` glob、审计日志内容摘要

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
| [archived/20260723-TASKS.md](archived/20260723-TASKS.md) | P3 + 开源后加固交付记录 |
| [archived/](archived/) | 历史设计、签核、E2E、清单 |
