# 任务清单

> 更新：2026-07-24  
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
        ├─► P0 写路径交付正确性（本分支）：clean-tree 先 push / 失败不标成功
        ├─► P1 Agent 摩擦 + Bootstrap 文件日志（本分支）
        ├─► P2 可观测性轻量项（本分支：评论区分 / 分析落地提示 / local-only 日志）
        └─► 按需 backlog：OpenCode A+ / LLM / 沙箱细项 · UI 高亮未推送分支
```

---

## 实跑复盘：Issue #4（2026-07-24）

来源：[jeeinn/ai-dev#4](http://182.92.129.124:3000/jeeinn/ai-dev/issues/4) · Task #6 `solve_comment` · session `619286bd` · 分支 `ai/dev/issue-4`

### 结论（核实）

| 项 | 事实 |
|----|------|
| 本地 commit | **有** — agent 经 `run_command` 完成 `git commit`（先因缺 `user.name/email` 失败一次，配置后成功） |
| Push | **无** — 日志无 `git push`；启动时即 `Branch ai/dev/issue-4 is local-only` |
| PR | **未开成** — `PR finalize on clean tree failed: create PR: API error 404` |
| Issue 观感 | agent 评论「完成」+ `Task 6 completed successfully` / `action=comment`，**静默降级** |

根因：`BuildSolveToolPrompt` 声明网关负责 commit/push/开 PR，但 agent 自行 commit 后工作区干净，`finalizeWriteChanges` 走 clean-tree 路径时**直接 CreatePR、不 Push**；远端无 head 分支 → Gitea 404 → 仍按成功评论写回。

### 过程中的其它摩擦

| 现象 | 说明 |
|------|------|
| `[WARN] Tool execution failed: unknown tool: ls` | 模型把 `ls` 当成 **tool 名**调用；注册表只有 `list_files` / `run_command` 等。Sandbox 白名单里的 `ls` 仅能经 `run_command` 使用 |
| `cd /workspace` / 错误绝对路径 | agent 多次猜 cwd，浪费轮次 |
| `git commit` exit 128 | session 工作区未预置 git identity |
| `git apply` / `apply_diff` 失败 | 改文件路径不稳，最终靠 shell + `git add` |
| 分析报告未完全落地 | 分析建议更新 `README.md` 链接；本次可能只改了 `docs/archived/README.md` |

---

## P0 — 写路径交付正确性（优先）

> 目标：本地有 commit 时，必须 push 成功并开出 PR（或明确失败），禁止「评论成功、远端无物」。

- [x] **clean-tree 先 Push 再 CreatePR**  
  `finalizeWriteChanges` 在 `!git.HasChanges()` 且分支 ≠ base 时：若本地领先 origin / 远端无该分支，先 `git.Push`，再 `finalizeWriteTaskPR`；push 失败不得假装成功。
- [x] **PR / push 失败不标 success**  
  finalize 失败返回 error（fail-closed）；不再静默降级为「成功评论」。`admin` Gitea client 不可用时同样失败。
- [x] **约束 agent 自行 commit/push**  
  Prompt 强化：不要自己 `git commit` / `git push`，交给网关 finalize。  
  （可选未做：sandbox 拦截 agent 侧 commit。）

---

## P1 — Agent 摩擦

- [x] **`unknown tool` 错误附带可用工具名**  
  例：`unknown tool "ls"; use list_files or run_command`
- [x] **（可选）常见 shell 别名自动改写**  
  `ls`/`dir` → `list_files`；`pwd` → `run_command`；`cat`/`type`（带 path）→ `read_file`。无法安全改写时错误信息附 hint。
- [x] **预置 git identity**  
  `prepareWriteWorkspace` 写入 `user.name` / `user.email`，避免第一次 commit 失败。  
  **约定**：`user.name` = agent 的 Gitea 用户名（如 `ai-coder`）；`user.email` = `{gitea_username}@matea.local`（如 `ai-coder@matea.local`）。
- [x] **注入真实 workspace 路径**  
  写任务 / analyze loop 注入实际 `WorkDir`；analyze single-shot（无仓库）明确告知「无本地工作区，勿臆造路径」。

---

## P1 — Bootstrap / 运维体验

### Bug：最小 bootstrap 未开启文件日志（已核实）

来源：README / DEPLOYMENT 描述「首次无 `config.yaml` 时自动写入最小 bootstrap」。

核实（`internal/config/bootstrap.go` + `main.go` + `internal/logging/logging.go`）：

| 项 | 现状 |
|----|------|
| bootstrap 写入 | 有 `server` / `database` / `workspace` / `logging.level: info` / 随机 `jwt_secret` |
| `logging.path` | **缺失** |
| `logging.SetupOutput(cfg.Logging.Path)` | `path` 为空时**仅 stdout**，不创建文件 |
| 生效文件名 | `path` 为**目录**；实际文件为 `{path}/matea.log`（例：`path: "./data"` → `./data/matea.log`；示例配置用 `./data/logs`） |

结论：**属实** — 首次 bootstrap 启动后不便落盘排查，与「最小可运维」预期不符。

- [x] **bootstrap 增加默认 `logging.path`**  
  建议写入 `path: "./data"`（得到 `./data/matea.log`，便于与 `matea.db` / `work` 同级排查）；或与 `config.example.yaml` 对齐用 `./data/logs`。同步更新 README / DEPLOYMENT 对 bootstrap 内容的说明。
- [ ] （可选）bootstrap 启动日志打印「Logging to file: …」，确认文件落盘成功

---

## P2 — 可观测性 / 完整性

- [x] **Issue 评论区分**「PR 已创建」vs「更新已有 PR」  
  成功路径用 ✅ / 🔄 前缀区分；「仅本地未推送」成功评论已由 P0 fail-closed 消除（失败任务由执行器写失败态）。
- [x] **分析建议落地提示（soft gate）**  
  analyze 结果末尾追加落地说明：分析不会自动改代码，需 coder 任务执行。  
  （未做：文件级 checklist / README 链接自动核对。）
- [x] **session 分支 local-only 日志警告**  
  `prepareWriteWorkspace` 发现远端无对应分支时打 `[WARN] … local-only`。  
  - [ ] （可选）WebUI 高亮未推送分支

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
