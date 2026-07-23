# 沙箱增强迭代计划

> 目标版本：v0.4  
> 状态：核心已落地；剩余按需（`cat` 行号、`find` glob、审计摘要）  
> 更新：2026-07-23（`rg` + temp/Session 对齐）

## 14.1 临时目录模式 ✅（核心）

- [x] 14.1.1 sandbox/sandbox.go — 支持临时目录模式（`os.MkdirTemp`）
- [x] 14.1.2 sandbox/sandbox.go — 配置化选择固定目录或临时目录（`mode: fixed|temp`）
- [x] 14.1.3 / 14.1.4 — Cleanup；Session 级 workspace 不走 temp 自动删（见 SessionLifecycle）
- [x] temp 与 Session 生命周期对齐：`NewWithPath` → `Persistent=true`；`Setup` 不覆盖预置 `WorkDir`；`Cleanup`/`CleanupWithDelay` 对 Persistent 为 no-op

### 运维要点（temp / Session）

| 场景 | 路径谁创建 | 任务结束 | 谁回收 |
|------|----------|----------|--------|
| 无 Session + `mode=fixed` | `sandbox.base_dir/task_{id}` | `Cleanup` 删除 | 任务 defer / 失败延迟清理 |
| 无 Session + `mode=temp` | `os.MkdirTemp` | `Cleanup` 删除 | 同上 |
| 有 Session | `{workspace.base_dir}/sessions/...` | **不删**（Persistent） | `SessionLifecycle`：Issue closed / PR merged / idle TTL / 磁盘上限 |

配置 `sandbox.mode=temp` **不会**把已绑定 Session 的工作区改成临时目录；误调 `Cleanup` 也不会删 Session 目录。

## 14.2 更丰富的上下文工具

- [ ] 14.2.1 agent/tools.go — `cat` 工具增强（支持行号范围）
- [x] 14.2.2 agent/tools.go — `tree` 工具
- [x] 14.2.3 agent/tools.go — `git_log` 工具
- [x] 14.2.4 agent/tools.go — `git_blame` 工具
- [x] 14.2.5 agent/tools.go — `rg` 工具（ripgrep；未安装时回退 `search_code`；已入 `analyze-readonly` / `coder-default`）
- [ ] 14.2.6 agent/tools.go — `find` 工具增强（glob）

## 14.3 配置化的超时和限制 ✅

- [x] 14.3.1 config/schema.go — SandboxConfig
- [x] 14.3.2 config.example.yaml — sandbox 段
- [x] 14.3.3 sandbox 从配置加载（main `parseSandboxConfig`）
- [x] 14.3.4 命令超时来自 SandboxConfig

**路径约定（S2）**：`sandbox.base_dir` 留空或为历史默认 `./workspace` 时，启动对齐为 `workspace.base_dir`；示例配置两者同为 `./data/work`。

## 14.4 安全增强

- [x] 14.4.1 文件路径验证（防路径穿越）
- [x] 14.4.2 文件大小限制
- [x] 14.4.3 命令白名单（参数注入面由白名单约束）
- [ ] 14.4.4 审计日志内容摘要（可选）

## 14.5 Agent 迭代控制配置化 ✅

- [x] 14.5.1–14.5.7 AgentLoopConfig / DB / API
- [x] 14.5.8 配置加载校验：`max_iterations` 1–100，`total_timeout` 1m–1h（`ValidateAgentLoopConfig`）
