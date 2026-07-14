# TODO: OpenCode Path A 接入

> 状态：未完成  
> 创建日期：2026-07-14  
> 优先级：高  
> 设计依据：[server-runtime-design-v4.md](server-runtime-design-v4.md)  
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

- [ ] 本机启动 `opencode serve --port 4096`
- [ ] 对临时 git 目录：create session → message → 确认文件变更
- [ ] 记录实际请求字段（directory / system / model）

### A1 — 配置与存储

- [ ] `AgentBackendsConfig` / `BackendConfig`（`opencode_http` / `builtin`）
- [ ] 默认加载为 `internal`；`config.example.yaml` 示例 `opencode-local`
- [ ] Agent：`Backend`、`BackendOptions` + SQLite migration
- [ ] API CRUD 暴露字段

### A2 — 抽取 write helpers（零行为变更）

- [ ] `prepareWriteWorkspace` / `finalizeWriteChanges`
- [ ] `runWriteTask` 改为 helpers + `InternalCodingBackend`
- [ ] 独立 PR，全量测试绿

### A3 — OpenCode HTTP Backend

- [ ] `CodingBackend` 接口 + `ResolveCodingBackend`
- [ ] `opencode_http.go`：session / message / abort / health
- [ ] 写任务按 backend 选择；非写任务强制 internal
- [ ] health 失败 → 可读评论（勿静默降级，除非显式 `allow_fallback_internal`）

### A4 — 测试与运维

- [ ] httptest mock OpenCode
- [ ] 集成：mock server + 假仓库
- [ ] ARCHITECTURE + sidecar 运维说明
- [ ] WebUI：Agent backend 下拉（可 API-only 首发）

### A+ — 可选后续

- [ ] SSE 进度 → Issue 评论或 task progress
- [ ] 持久化 `opencode_session_id`
- [ ] `allow_fallback_internal`
- [ ] Claude PrintBackend（契约型 CLI，非盲扫）

---

## 验收（Path A Done）

1. 默认配置下 Analyze / Review / Dev（internal）与接入前一致  
2. coder `backend=opencode-local` + 本机 sidecar → Issue→改码→PR  
3. sidecar 宕机时失败原因可读  
4. 误配 backend 的 Analyze/Review 不走 OpenCode  
5. Session Continue 复用 `WorkspacePath`；尽量续 OpenCode session  
6. 文档说明如何启动 sidecar

---

## 明确不做（本 TODO 范围外）

- 远程 `opencode serve` / 跨机 workspace  
- Path B bare mirror + worktree 基础设施  
- 早期稿中的 ExternalCLIRunner 作为默认交付主路径
