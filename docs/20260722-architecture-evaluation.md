# 架构深度评估报告 — Gitea Agent Gateway

> 评估日期：2026-07-22
> 评估基线：v0.10.0（分支 `feature/llm-provider-model-catalog`）
> 评估范围：Go 后端（internal/ 131 个文件）、Vue3 前端（web/src）、配置与文档体系
> 评估方法：源码走查（入口、关键包、并发路径）+ 依赖关系分析 + 文档对照

---

## 0. 总评

| 维度 | 评分（10 制） | 一句话结论 |
|---|---|---|
| 模块划分 | 8.5 | 14 个 internal 包职责清晰，workflow / agent / agents 三层拆分是亮点 |
| 依赖关系 | 8.0 | 无环，4 处刻意的依赖注入解耦；扣分项是两阶段 setter 与包级全局态 |
| 数据流向 | 8.0 | 单向异步管线清晰；webhook 入口存在崩溃丢事件窗口 |
| 技术选型 | 8.0 | 与「单节点开源网关」定位高度匹配，依赖极度克制（3 个直接依赖） |
| 可扩展性 | 7.0 | Runner / CodingBackend / Provider 三个策略扩展点良好；单节点是硬边界 |
| 可维护性 | 7.5 | 测试与文档优于同类项目；4 个超大文件与前端工程化是主要债务 |
| 性能 | 7.0 | 与当前规模匹配；SQLite 单写、LLM 非流式、HTTP 客户端短连接是已知天花板 |
| 演进可行性 | 8.0 | 项目既定路线（OpenCode / MCP / Harness）与架构兼容，无需推翻 |

**总体 7.8 / 10。** 设计纪律高于同规模个人/小团队项目的常见水平：依赖无环、文档体系完整（34 篇设计/签核/E2E 文档）、策略模式运用得当、依赖面极小。当前架构**不需要重构**，需要的是定向硬化（并发安全）与工程化补齐（大文件拆分、前端工程化）。

---

## 1. 架构设计合理性

### 1.1 模块划分 —— 合理，职责边界清晰

整体是「三层管线 + 横切基础设施」结构：

| 层 | 包 | 职责 |
|---|---|---|
| 接入层 | `webhook` | HMAC 验签、delivery 去重、事件解析 |
| 编排层 | `dispatcher` + `workflow` | 事件解析（Resolver）、L1/L2 门禁、WorkflowContext 状态机、Session 生命周期、任务队列与执行器 |
| 执行层 | `agents` → `agent` → `llm` / `sandbox` / `gitea` | 5 种 Runner 策略、多轮工具调用循环、LLM 适配、沙箱执行、Gitea 写回 |
| 横切 | `store` / `config` / `api` / `auth` / `logging` / `mcp` | 持久化、配置、管理 API、认证、日志、MCP 工具消费 |

**亮点：**

1. **`agent`（通用循环引擎）与 `agents`（业务 Runner 策略）分离** —— 循环引擎不关心业务语义，Runner 不关心工具调用细节，可独立测试、独立演进。
2. **`workflow` 独立成包** —— Resolver / 状态机 / 门禁 / Session / 生命周期各为一个文件，v2 Assign 模型的核心概念都有明确落点。
3. **CodingBackend 接口（internal / opencode_http 双实现）** —— 为 OpenCode sidecar 预留的扩展点，属于「提前一期做对接口」的典范。
4. **Prompt 四级加载链**（DB 版本历史 → Agent 字段 → 配置文件 → 内置模板）—— 优先级设计务实，支持 UI 版本管理。

**瑕疵：**

- `dispatcher` 仍偏「上帝包」：事件管线 + 队列 + 执行器 + 模板渲染挤在一个包，`dispatcher.go` 单文件 26KB。
- `api/router.go` 27KB 单文件承担全部���理端点（30+ 路由），按资源拆分（agents.go / tasks.go / users.go …）会更清晰。

### 1.2 依赖关系 —— 健康无环，解耦手法得当

`go list` 确认依赖无环。依赖方向始终指向基础设施层：

```
webhook → config
agent → {config, llm, logging, mcp, sandbox}
agents → {agent, config, gitea, llm, mcp, sandbox, store, webhook}
workflow → {agents, config, gitea, store, webhook}
dispatcher → {agents, workflow, ...}
api → {agents, auth, config, gitea, llm, store}
```

**4 处刻意解耦（设计成熟的标志）：**

| 手法 | 位置 | 解决的问题 | 评价 |
|---|---|---|---|
| 函数注入 `SetModelDiscoveryFunc` | config/manager.go:40 | 破 llm ↔ config 循环依赖 | 有效但引入包级全局变量且无锁（P2） |
| `config.Store` 接口 | config/manager.go:14 | config 不反向依赖 store 具体类型 | 教科书式做法 |
| `GiteaClientFactory` 接口注入 | dispatcher → executor/agents | 测试可 mock Gitea | 好 |
| `ModelMetaProvider` 接口 | agents/runners.go:47 | Runner 不直接依赖 ConfigManager | 好 |

**扣分项：**

1. **两阶段 setter 注入**（`SetWorkflowComponents` / `SetDebugConfigGetter` / `SetModelMetaProvider`，main.go:114-135）—— Dispatcher 构造后处于「半成品」状态，注入遗漏只能在运行时暴露。建议改为构造参数或 functional options，让「不可用的对象」无法被构造出来。
2. **`store` 无 Repository 接口** —— 全部包直接依赖 `*store.DB` 具体类型，单元测试必须起真实 SQLite（好在 modernc 纯 Go 内存库很快，部分缓解）。

### 1.3 数据流向 —— 单向清晰，入口有丢失窗口

主链路（对应架构图编号）：

```
Gitea → ①webhook(验签/去重/解析) → ②异步回调 Dispatcher
  → Resolver → L1 门禁 → WorkflowContext.Transition → L2 门禁
  → Session.GetOrCreate → ③TaskQueue(先写 DB 再入内存 channel)
  → ④Executor Worker → ⑤Runner → ⑥LLM / ⑦Sandbox 工具循环
  → ⑧写回 Gitea 评论/PR → OnComplete 推进状态机
```

**做得好的：** 同步验签快速 200 + 异步处理；任务先落库再入队，崩溃可恢复；写回失败标记 `partial` 并用 admin token 补发提示（有补偿意识）。

**三个一致性风险：**

1. **【P1】webhook 事件丢失窗口** —— `handler.go:86` 裸 goroutine 异步回调，`MarkProcessed` 在回调成功后才执行。进程若在「已返回 200 → 事件入队」之间崩溃，事件**永久丢失且 Gitea 不会重投**。建议收件箱模式：先同步落库（pending）再异步处理，或至少复用现有 `processed_deliveries` 表做「已接收/已处理」两阶段标记。
2. **三处状态靠约定保持一致** —— `inFlight sync.Map`（内存锁）、`pending` 任务检查（DB）、内存 channel 三份状态；启动时的 `loadPending` 恢复路径与运行态路径是两份逻辑，易随演进脱节。
3. **补偿链路无重试** —— 写回失败 → partial → admin token 补发，但补发失败仅 `log.Printf("[WARN]")`，无重试与告警通道（dispatcher.go:547 等处同类问题）。

---

## 2. 技术选型匹配度

| 选型 | 匹配度 | 关键理由 | 风险/代价 |
|---|---|---|---|
| Go 1.26 + 标准库 ServeMux | 高 | 单二进制分发、go:embed 内嵌前端、webhook/后台轮询场景并发模型简单 | 无框架 → 中间件需手写（见 §3.1-7） |
| 仅 3 个直接依赖（yaml、modernc/sqlite、testify） | 高 | 供应链攻击面极小，审计成本低 | 部分功能需自造轮子 |
| modernc.org/sqlite（纯 Go） | 高 | 免 CGO → 5 平台交叉编译发布（v0.10.0 已实践） | 极端写入压力下性能略逊于 C 版 SQLite |
| SQLite WAL + 单写连接 | 高 | 与单节点部署模型匹配，零运维 | 多副本/多实例时的硬天花板 |
| 手动 DI（无 fx/wire） | 高 | 规模匹配，启动顺序一目了然 | 两阶段 setter 是手动 DI 的典型坏味道 |
| Vue3 + Element Plus + Pinia | 高 | 管理后台效率优先，中文生态好 | 全量引入 bundle 偏大（后台场景可接受） |
| go:embed 前后端一体 | 高 | 单二进制交付，���署体验极佳 | 前端 dev 需代理（已配 vite proxy） |
| JWT + bcrypt | 高 | 管理后台标准方案 | 与静态 token 双制式并存增加心智负担 |
| 非 Docker 沙箱（目录隔离+白名单） | 中 | 取舍已被 ARCHITECTURE.md 明确记录：毫秒级启动、零容器依赖，匹配「受信仓库」场景 | 若未来面向公开仓库/不可信代码则不成立，需容器级隔离 |
| Anthropic 适配器 | 中 | 覆盖 Claude 用户 | 不支持 tool calls → coder 类 Agent 实际无法使用 Claude，建议在 Agent 配置校验中显式约束，避免用户误配 |

**值得强调的��断：** 项目没有引入任何 Agent/LLM 框架（LangChainGo、Eino 等），AgentLoop 仅 ~2 个文件实现多轮工具循环 + 截断 + 预算控制 —— 对于这个场景（工具集固定、循环逻辑直白）是正确取舍，框架的抽象成本远���于收益。

**前端选型附注：** vite 8 + 纯 JS、无 TS/ESLint/测试。对 ~4.4k 行、8 个页面的管理后台可接受，但与后端的工程纪律明显不对称（详见 §3.3）。另外发现 `@element-plus/icons-vue` 被大量使用却未声明在 `web/package.json`（幽灵依赖，靠 node_modules 偶然存在而工作），应补声明。

---

## 3. 可扩展性 / 可维护性 / 性能

### 3.1 隐患清单（按优先级）

**P1 —— 建议尽快修复：**

| # | 问题 | 位置 | 说明与修复方向 |
|---|---|---|---|
| 1 | `llm.Registry` 数据竞争 | registry.go:56 `Get` 读 map vs :73 `Reload` 整体替换，无锁 | 配置热更新（PUT /api/config）与在途 Agent 任务并发时存在 data race，极端情况下 panic。修复：RWMutex，或 `atomic.Value` 持有不可变快照 |
| 2 | `ConfigManager.Get()` 返回共享 `*Config` | config/manager.go | 读锁释放后调用方持有可变对象，热更新时同类竞争。修复：返回深拷贝（或按配置段拷贝） |
| 3 | webhook 事件丢失窗口 | webhook/handler.go:86 | 见 §1.3-1，收件箱模式 |
| 4 | Provider 类型判定不一致 | registry.go:33 仍按名称 `EqualFold("claude"/"anthropic")` 选择适配器；而配置体系已引入 `type` 字段（`openai_compatible`/`anthropic`，见 config.example.yaml:54/68） | 名为 `my-claude-proxy` 的 OpenAI 兼容 provider 会被错误地走 Anthropic 适配器。修复：统一以 `type` 字段为准，名称匹配仅作历史兜底 |

**P2 —— 择机处理：**

| # | 问题 | 位置 |
|---|---|---|
| 5 | 四个超大文件：`config/manager.go` 29KB、`agents/runners.go` 28KB、`api/router.go` 27KB、`dispatcher/dispatcher.go` 26KB | 按职责拆分 |
| 6 | 包级全局态：`modelDiscoveryFn`（无锁）、`logging.currentLevel` | 测试隔离性差 |
| 7 | 认证双制式（JWT + 静态 Bearer token）并存；无 CORS / 限流 / 请求日志中间件 | api/router.go —— 建议统一中间件链 |
| 8 | 错误吞没模式 `log.Printf("[WARN]")` 后继续 | dispatcher.go:547 等；关键路径错误应反映到任务状态 |
| 9 | `gitea.Client` 逐次新建、无连接复用 | dispatcher/agents —— 共享 Transport |
| 10 | Agent Loop 的 ChatRequest 未透传 TopP / Frequency / Presence Penalty | 当前分支已知残留项 |
| 11 | `data/work/sessions/` 运行时克隆产物与代码库同树 | 确认 .gitignore 覆盖即可 |

### 3.2 性能评估

- **SQLite 单写连接** —— API 写入、任务状态流转、审计日志全部串行。默认 `max_concurrent: 2` 下无感；并发调高或审计风暴时会在 `busy_timeout 10s` 上排队。这是单 writer 模型的固有上限，WAL 只解决读写并发。对 webhook 驱动的低频人工触发场景（每天几十~几百任务）**完全够用**。
- **任务队列** —— 内存 channel（容量 100）+ 60s scanner 兜底重置超时 running 任务，吞吐对该场景富余。
- **LLM 调用非流式** —— 长任务（loop 总超时 30m）期间用户只能看到「🔄 已开始」和最终结果，无中间进度。OpenCode A+ 的 SSE 计划正好补这个短板，方向正确。
- **Token 估算启发式**（CJK 2 字符/token、其他 4 字符/token）—— 务实的第一版，tiktoken 精确计数已在 roadmap（todo-20260714）。
- **模型元数据缓存**（成功 1h / 失败 5min TTL）—— 合理。
- **前端全量 Element Plus** —— 首屏 bundle 偏大，后台系统可接受；若要优化可改按需引入。

### 3.3 可维护性评估

**加分项：**
- ~60 个测试文件，核心包覆盖良好（agent 9 / agents 10 / workflow 7 / gitea 6）；`tests/integration` 的 `TestEnv`（内存 SQLite + Mock Gitea + Mock LLM + 完整 HTTP server）是高质量的测试基础设施。
- 文档体系罕见地完整：ARCHITECTURE.md 与代码同步、TASKS.md 有明确「明确不做」清单、v1→v2 迁移有归档对照、E2E 有签核报告。**这是长期可维护性最大的资产。**

**减分项：**
- 测试薄弱包：`webhook` / `mcp` / `auth` / `logging` 各 1 个测试文件；无 LLM provider 的 HTTP mock 测试（openai.go / anthropic.go 的序列化逻辑无覆盖）。
- 前端工程化缺口：无 TS / ESLint / 测试；`SystemConfig.vue` 1252 行、`Agents.vue` 683 行；40+ 处 API 调用散落在 view 中无模块化封装（`/config`、`/agents` 多页重复请求）；`@element-plus/icons-vue` 幽灵依赖。

### 3.4 可扩展性评估

**三个策略扩展点设计良好：** Runner（新任务类型 = 新 Runner + 工厂注册）、CodingBackend（internal / opencode_http）、LLM Provider（单方法接口）。新增能力都不需要改动编排层。

**硬边界（是「边界」而非「缺陷」，但应在文档中明示）：**

| 边界 | 根源 | 触发重构的信号 |
|---|---|---|
| 单节点部署 | `inFlight sync.Map`、内存 channel、Session 清理循环均在进程内 | 需要 HA / 多副本 |
| 单 Gitea 实例 | `config.gitea` 为单数结构 | 服务多个 Gitea 站点 |
| 单 SQLite 文件 | store 层 | 多副本共享状态 |

这些与「中小团队 Gitea-first」的产品定位（TASKS.md 明确不做多平台 SPI）一致。

---

## 4. 未来演进方向评估

### 4.1 项目既定路线 —— 均可行，与现有架构兼容

| 路线 | 架构支撑现状 | 结论 |
|---|---|---|
| OpenCode A+（SSE 进度 / session 持久化 / 集成测试） | CodingBackend 接口 + opencode_http 实现已就位 | 纯增量演进，无阻碍 |
| MCP 工具消费 | `mcp` 包已接入 ToolRegistry | 已落地，继续扩展 server 即可 |
| Harness 验证（Reviewer 独立 Checker 防自评） | no_progress_limit / verify_commands 已落地；Checker 只需在 Runner 层新增 | 无架构阻碍 |
| 沙箱补强（rg / temp 模式对齐） | 局部改动 | 无阻碍 |
| LLM 增强（tiktoken / 语义摘要 / 成本预算） | truncate.go 与 task_usage 表已有挂点 | 无阻碍 |

### 4.2 何时需要更大的架构变化（信号 → 方案对照）

| 信号 | 建议方案 | 提前引入的代价 |
|---|---|---|
| 需要多副本 / HA | 队列外置（NATS / Redis Streams）+ Postgres + 分布式锁；dispatcher 拆 ingest/worker | 高 —— 现在做是过度设计 |
| 多 Gitea 实例 / 多租户 | gitea 配置实例化 + client 池 + 数据模型加 instance 维度 | 中 |
| 工作流复杂度爆炸（人工审批、跨仓库长事务） | 评估 Temporal 等工作流引擎；当前状态机（6 个 stage）远未到需要它的复杂度 | 高 |
| 需要复杂 Agent 编排 / RAG | 届时再评估 Eino / LangChainGo；当前自研 Loop 更可控 | 中 |
| 执行不可信代码（公开仓库） | 容器级沙箱后端（Docker / gVisor）；sandbox 的 Mode 抽象已留雏形 | 视威胁模型而定 |

### 4.3 替代路线对比

| 路线 | 成本 | 收益 | 建议 |
|---|---|---|---|
| 保持架构 + 定向硬化（P1 四项） | 低（约 1-2 天） | 消除全部已知正确性风险 | **推荐，当前阶段最优解** |
| 大文件拆分 + store 接口化 | 中（约 1-2 周） | 测试与演进成本持续下降 | 推荐，随下个 feature 分支同步做 |
| 前端 TS 化 + 组件/API 模块化 | 中 | 管理 UI 长期可维护性 | 择机，不阻塞主线 |
| 平台化重写（多租户 / HA / 消息队列） | 高 | 打开新部署形态 | 不做 —— 与 Gitea-first 定位冲突，且无需求信号 |

---

## 5. 改进路线图建议

**立即（可随当前 LLM catalog 分支或紧随其后的补丁）：**
1. `llm.Registry` 加锁或改 `atomic.Value` 快照；`ConfigManager.Get()` 返回拷贝
2. Provider 类型统一走 `type` 字段（registry.go:33），名称匹配降级为兜底
3. webhook 收件箱落库（最低成本版：回调内先持久化「已接收」标记再处理）

**短期（1–2 周）：**
4. 拆分 4 个超大文件；API 统一中间件链（CORS / 限流 / 请求日志）
5. 关键路径错误处理升级：写回失败重试 + `operation_logs` 告警标记
6. `gitea.Client` 复用共享 Transport；补 LLM provider 的 HTTP mock 测试；补 `@element-plus/icons-vue` 依赖声明

**中期（1–2 月，与 OpenCode A+ / Harness 主线合并做）：**
7. `store` Repository 接口化（先 tasks / sessions 两个高频面）
8. 前端：SystemConfig 按 Tab 拆组件、API 层模块化、引入 ESLint；可选渐进 TS
9. SSE / 流式进度回传（与 OpenCode A+ 合并实施，同步解决「长任务无反馈」）

---

## 附：关键证据位置

| 结论 | 文件 |
|---|---|
| 启动组装与两阶段注入 | main.go:86-135, 206-213 |
| Registry 竞争 / 名称匹配问题 | internal/llm/registry.go:26-40, 55-74 |
| webhook 丢失窗��� | internal/webhook/handler.go:86 |
| 事件管线 | internal/dispatcher/dispatcher.go（handleEventV2） |
| 错误吞没示例 | internal/dispatcher/dispatcher.go:547 |
| Runner 策略与 token 自适应 | internal/agents/runners.go:47, 133 |
| 超大文件 | config/manager.go、agents/runners.go、api/router.go、dispatcher/dispatcher.go |
| 前端巨型组件 | web/src/views/SystemConfig.vue（1252 行） |
| 架构基线文档 | docs/ARCHITECTURE.md、docs/TASKS.md |
