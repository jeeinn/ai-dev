# TODO: OpenCode Path A 接入

> 状态：进行中（P0.1/P0.2/A1/A3/A4-mock 已落地；A0 本机验收与 WebUI backend 未完）  
> 创建日期：2026-07-14  
> 更新：2026-07-15  
> 优先级：高  
> 设计依据：[server-runtime-design-v4.md](server-runtime-design-v4.md)  
> 字段笔记：[opencode-a0-notes.md](opencode-a0-notes.md)  
> 早期 CLI 方案（已废弃作主路径）：[archived/20260710-opencode-integration.md](archived/20260710-opencode-integration.md)

---

## 产品约束（勿偏离）

1. 默认 `backend=internal`，Analyze / Review 强制 internal  
2. OpenCode = **本机 sidecar**（`opencode serve` HTTP/OpenAPI）；CLI `run` 仅降级  
3. Gateway 管编排与 Git 写回；OpenCode 管改码  
4. **不做**远程 OpenCode、完整 Path B worktree、多 Git 托管抽象（见平台策略归档）

---

## 实施清单（摘自 v4 §7 / §12）

### A0 — PoC

- [x] 记录实际请求字段（directory query/header / system / model）→ [opencode-a0-notes.md](opencode-a0-notes.md)
- [ ] 本机启动 `opencode serve --port 4096` 并端到端确认改码落在 Gateway workspace

### A1 — 配置与存储

- [x] `AgentBackendsConfig` / `BackendConfig`（`opencode_http` / `builtin`）
- [x] 默认加载为 `internal`；`config.example.yaml` 示例 `opencode-local`
- [x] Agent：`Backend`、`BackendOptions` + SQLite migration
- [x] API CRUD 暴露字段

### A2 — 抽取 write helpers（零行为变更）

- [x] `prepareWriteWorkspace` / `finalizeWriteChanges`
- [x] `runWriteTask` 改为 helpers + `InternalCodingBackend`
- [x] 独立 PR，全量测试绿

### A3 — OpenCode HTTP Backend

- [x] `CodingBackend` 接口 + `ResolveCodingBackend`
- [x] `opencode_http.go`：session / message / abort / health
- [x] 写任务按 backend 选择；非写任务强制 internal
- [x] health 失败 → 任务 **failed** + 可读错误（勿静默降级，除非显式 `allow_fallback_internal`）
- [x] createSession 绑定 `?directory=` + `X-Opencode-Directory`

### A4 — 测试与运维

- [x] httptest mock OpenCode
- [ ] 集成：mock server + 假仓库
- [x] ARCHITECTURE + sidecar 运维说明（DEPLOYMENT / ARCHITECTURE）
- [ ] WebUI：Agent backend 下拉（可 API-only 首发）

### A+ — 可选后续

- [ ] SSE 进度 → Issue 评论或 task progress
- [ ] 持久化 `opencode_session_id`
- [x] `allow_fallback_internal`（已接线：health 失败时可切 internal）
- [ ] Claude PrintBackend（契约型 CLI，非盲扫）

---

## 验收（Path A Done）

1. 默认配置下 Analyze / Review / Dev（internal）与接入前一致  
2. coder `backend=opencode-local` + 本机 sidecar → Issue→改码→PR  
3. sidecar 宕机时失败原因可读（任务 status=`failed`，非 success）  
4. 误配 backend 的 Analyze/Review 不走 OpenCode  
5. Session Continue 复用 `WorkspacePath`；尽量续 OpenCode session  
6. 文档说明如何启动 sidecar

---

## 明确不做（本 TODO 范围外）

- 远程 `opencode serve` / 跨机 workspace  
- Path B bare mirror + worktree 基础设施  
- 早期稿中的 ExternalCLIRunner 作为默认交付主路径
