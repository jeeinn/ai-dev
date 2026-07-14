# 任务清单（核心演进）

> 更新：2026-07-14  
> 产品边界：**Gitea 优先** · 内置 Agent 默认可用 · OpenCode 可选 · 不做多托管平台抽象  
> 决策：[archived/20260714-coding-gateway-multi-vcs.md](archived/20260714-coding-gateway-multi-vcs.md)  
> 旧版分散 backlog 已归档：[archived/20260714-TASKS.md](archived/20260714-TASKS.md)

---

## 演进主线

```text
可靠性 + 写路径抽取
        │
        ├─► OpenCode Path A（可选加强 coder）
        │
        └─► Internal 能力：ToolPack → Analyze 短 Loop → MCP → Skills
```

两条线正交：OpenCode **不替代** Analyze 读仓；Analyze **永远** `backend=internal`。

---

## P0 — 立即

### 1. 写回可靠性

- [x] Executor：Gitea 评论/写回失败时任务不得标为纯粹 `success`（失败或「部分完成」+ 可读错误）
- [x] 关键失败可观测（日志 + task.error / 评论）

### 2. 写路径抽取（OpenCode 与 Analyze 共用前置）

设计：[server-runtime-design-v4.md](server-runtime-design-v4.md) §4.5 / A2

- [x] `prepareWriteWorkspace` / `finalizeWriteChanges`（**零行为变更**，独立可合并）
- [x] 预留只读 `prepareAnalyzeWorkspace`（浅 clone、不建分支）形状，避免两套 clone

### 3. OpenCode Path A

设计：[todo-20260714-opencode-path-a.md](todo-20260714-opencode-path-a.md) · [server-runtime-design-v4.md](server-runtime-design-v4.md)

- [ ] A0 本机 `opencode serve` PoC + API 字段记录
- [x] A1 `agents.backends` + Agent `backend` / `backend_options` + migration
- [ ] A3 `CodingBackend` + `OpenCodeHTTPBackend`；非写任务强制 `internal`
- [ ] A3 health 失败 → 可读评论（默认不静默降级）
- [ ] A4 mock 测试 + 运维说明（WebUI backend 可后置）

**约束**：默认 `internal`；Analyze / Review 永不走 OpenCode。

---

## P1 — 核心能力（Internal Loop）

设计：[20260714-internal-capabilities-toolpack-mcp-skills.md](20260714-internal-capabilities-toolpack-mcp-skills.md)

### 4. ToolPack

- [ ] `config.yaml` 命名包（`coder-default` / `analyze-readonly`）
- [ ] Agent `tool_pack` 字段；`AssembleToolRegistry`
- [ ] coder 行为与现行 `DefaultTools` **零回归**

### 5. Analyze 短只读 Loop（读仓）

- [ ] 浅 clone `default_branch` + `analyze-readonly` ToolPack
- [ ] 短 `AgentLoop`（低 `max_iterations`）；禁止写工具 / 随意 `run_command`
- [ ] 评论引用真实路径；不建分支、不提 PR；workspace 可清理
- [ ] clone 失败策略（失败注释或可选弱降级 single-shot）

### 6. 沙箱可运维化（支撑 4/5）

详见 [sandbox-roadmap.md](sandbox-roadmap.md)；本清单只收核心：

- [ ] `SandboxConfig` 接入 YAML（替换 runner 硬编码默认）
- [ ] 路径穿越 / 输出与文件大小限制（安全底线）
- [ ] （按需）`rg` 工具；temp 清理与 Session workspace 对齐后再上

### 7. 工作流可观测

- [ ] Web UI / 任务详情消费已有 `GET /api/workflow-context`
- [ ] （可选）preset `free|standard|strict` 配置入口（现仅 yaml）

---

## P2 — 增强（不阻塞主线）

### 8. 远程 MCP → ToolRegistry

- [ ] `mcp_servers` 定义 + Agent 启用列表
- [ ] 合并进同一 `ToolRegistry`；Analyze 仅只读类

### 9. 文件型 Skills

- [ ] 扫描 Gateway 目录 + 仓库内 `SKILL.md`（agentskills.io 形态）
- [ ] 渐进披露 + `load_skill`；Analyze 默认禁任意脚本

### 10. 产品打磨（旧 backlog 摘录）

完整列表见 [archived/20260714-TASKS.md](archived/20260714-TASKS.md)。按需选自：

- UI：移除废弃 TriggerRules Tab；Agent 创建向导  
- 多仓：WorkflowPolicy per-repo；阶段切换 unassign  
- LLM 可选：tiktoken / 摘要 / 成本预算 — [todo-20260714-LLMProvider-可选增强.md](todo-20260714-LLMProvider-可选增强.md)

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

| 周次 | 焦点 |
|------|------|
| 1 | P0.1 写回可靠性 + P0.2 写路径抽取 |
| 1–2 | P0.3 OpenCode A0–A1 |
| 2–3 | P0.3 A3–A4；并行 P1.4 ToolPack |
| 3–4 | P1.5 Analyze 短 Loop + P1.6 沙箱 YAML |
| 之后 | P1.7 可观测；P2 MCP → Skills |

---

## 关键文档

| 文档 | 用途 |
|------|------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | 现行架构 |
| [DEPLOYMENT.md](DEPLOYMENT.md) | 部署 |
| [server-runtime-design-v4.md](server-runtime-design-v4.md) | OpenCode / CodingBackend |
| [20260714-internal-capabilities-toolpack-mcp-skills.md](20260714-internal-capabilities-toolpack-mcp-skills.md) | ToolPack / MCP / Skills / Analyze |
| [todo-20260714-opencode-path-a.md](todo-20260714-opencode-path-a.md) | Path A checklist |
| [sandbox-roadmap.md](sandbox-roadmap.md) | 沙箱细项 |
| [archived/](archived/) | 历史设计与旧 TASKS |

已交付：Assign 工作流 v2（[archived/20260616-TASKS.md](archived/20260616-TASKS.md)）、LLM Provider 主方案（[archived/20260710-LLMProvider…](archived/20260710-LLMProvider模型选择与Token配置扩展方案.md)）。
