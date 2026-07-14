# Gitea + AI Agent 研发流程提效方案（v3 动态 Agent 版）

## 一、核心理念

以 **Gitea 为研发协作中枢**，通过 Webhook 事件驱动，将 AI Agent 无缝嵌入现有 Git 工作流。
每个 Agent 是 Gitea 中的一个**虚拟团队成员**，有自己的账号、职责和专属模型。
不替代人的决策权，而是让 Agent 承担重复性脑力劳动，人只做审核和最终决策。

## 二、系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                        Gitea (中枢)                          │
│  Issues / PR / Labels / Assignees / @Mention / Webhooks     │
│                                                             │
│  虚拟成员:  ai-frontend-dev / ai-backend-dev / ai-reviewer  │
└──────────┬──────────────────────────────────┬───────────────┘
           │ Webhook Events                   │ API 调用
           ▼                                  ▲
┌─────────────────────┐              ┌─────────────────────────┐
│   Webhook Gateway   │──────────────│     Agent 回写层         │
│  (事件路由 & 过滤)    │              │  (评论/提PR/打Label/     │
└─────────┬───────────┘              │   @回复/通知)            │
          │                          └─────────────────────────┘
          ▼                                    ▲
┌─────────────────────┐                        │
│  Agent Dispatcher   │────────────────────────│
│  (Label+Assignee    │                        │
│   双条件路由)        │                        │
└─────────┬───────────┘                        │
          │                                    │
    ┌─────┼──────┬──────────┬──────────┐       │
    ▼     ▼      ▼          ▼          ▼       │
┌──────┐┌──────┐┌──────┐┌────────┐┌────────┐  │
│PM    ││前端  ││后端  ││Code    ││测试    │  │
│Agent ││Agent ││Agent ││Review  ││Agent   │  │
│      ││      ││      ││Agent   ││        │  │
└──────┘└──────┘└──────┘└────────┘└────────┘──┘
  ↑ 可动态创建、配置、启用/禁用
```

## 三、动态 Agent 管理（核心特性）

### 3.1 Agent = Gitea 虚拟成员

每个 Agent 在系统中注册时自动完成：
1. **Gitea 账号自动注册**（Admin API 创建用户）
2. **生成 API Token**（该账号专属，权限最小化）
3. **分配模型和 Prompt**
4. **创建路由规则**

Agent 在 Gitea 中的表现和真人团队成员一样：
- 有独立用户名、头像、邮箱
- 可以被分配 Issue（Assign）
- 可以被 @Mention 引用
- 可以评论 Issue、创建 PR、Review 代码
- 操作记录可追溯到具体 Agent

### 3.2 Agent 管理 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/agents` | 列出所有 Agent |
| POST | `/api/agents` | 创建 Agent（自动注册 Gitea 账号） |
| GET | `/api/agents/:id` | 获取 Agent 详情 |
| PUT | `/api/agents/:id` | 更新 Agent 配置（模型/Prompt/状态） |
| DELETE | `/api/agents/:id` | 删除 Agent（可选禁用 Gitea 账号） |
| POST | `/api/agents/:id/enable` | 启用 Agent |
| POST | `/api/agents/:id/disable` | 禁用 Agent |
| POST | `/api/agents/:id/test` | 测试 Agent（发送测试请求） |

### 3.3 Agent 创建流程

```
Web UI → 创建 Agent
  ├── 名称: "前端研发 Agent"
  ├── Gitea 用户名: ai-frontend-dev  （自动注册）
  ├── 模型配置:
  │   ├── Provider: deepseek
  │   ├── Model: deepseek-chat
  │   ├── Max Tokens: 8192
  │   └── Temperature: 0.2
  ├── Prompt:
  │   ├── System Prompt: 你是高级前端研发工程师...
  │   └── User Template: 请根据以下需求实现代码...
  └── 路由规则:
      ├── Event: issues
      ├── Action: assigned
      ├── Label: agent:dev:frontend
      └── Assignee: ai-frontend-dev
```

**自动执行**：
1. 调用 Gitea Admin API 创建 `ai-frontend-dev` 账号
2. 为该账号生成 API Token
3. 存储到 SQLite `agents` 表
4. 创建路由规则到 `routes` 表

### 3.4 Agent @Mention 互动

当用户在 Issue 或 PR 评论中 @某个 Agent 时，Agent 可以自动回复。

**触发条件**：`issue_comment` 事件，评论内容包含 `@ai-frontend-dev`

**交互场景**：
- 用户在 Issue 中 @Agent 询问进度 → Agent 回复当前任务状态
- 用户在 PR 中 @Agent 要求修改 → Agent 根据评论修改代码
- 用户 @Agent 提问技术问题 → Agent 基于代码上下文回答

**实现方式**：
```yaml
routes:
  # @Mention 触发
  - event: "issue_comment"
    action: "created"
    match:
      mention: "ai-frontend-dev"   # 评论中 @了这个 Agent
    agent: "frontend-dev"
    priority: 5
```

### 3.5 预置 Agent 模板

系统内置几个常用 Agent 模板，用户创建时可选择：

| 模板名 | Gitea 用户名 | 默认模型 | 用途 |
|--------|-------------|---------|------|
| 需求分析 Agent | ai-analyze | deepseek-chat | 需求完整性分析 |
| 前端研发 Agent | ai-frontend-dev | deepseek-chat | 前端代码实现 |
| 后端研发 Agent | ai-backend-dev | deepseek-chat | 后端代码实现 |
| Bug 修复 Agent | ai-bugfix | deepseek-chat | Bug 定位与修复 |
| Code Review Agent | ai-reviewer | deepseek-chat | PR 代码审查 |
| 测试 Agent | ai-tester | deepseek-chat | 测试用例生成 |

用户可以在模板基础上修改，也可以从零创建自定义 Agent。

## 四、Gitea 账号与权限

### 4.1 自动注册

通过 Gitea Admin API 创建用户：
```
POST /api/v1/admin/users
Authorization: Basic {admin_token}

{
    "login_name": "ai-frontend-dev",
    "username": "ai-frontend-dev",
    "email": "ai-frontend-dev@gateway.local",
    "password": "{auto_generated_random_password}",
    "must_change_password": false,
    "send_notify": false
}
```

然后为该用户生成 API Token：
```
POST /api/v1/users/ai-frontend-dev/tokens
Authorization: Basic {admin_token}

{
    "name": "gateway-agent-token",
    "scopes": ["write:issue", "write:repository", "read:repository"]
}
```

### 4.2 Token 权限最小化

每个 Agent 账号只授予：
- `write:issue` — 评论 Issue、打标签
- `write:repository` — 创建分支、推送代码、创建 PR
- `read:repository` — 读取仓库内容

**不授予**：`admin`、`write:user`、`write:organization` 等无关权限。

### 4.3 账号管理

- 创建 Agent → 自动注册 Gitea 账号
- 禁用 Agent → 禁用 Gitea 账号（API Token 失效）
- 删除 Agent → 可选保留或禁用 Gitea 账号
- Agent 头像 → 可在 Web UI 自定义上传

## 五、Agent 执行环境

### 5.1 工作目录管理

每次任务在 Gateway 服务器本地临时目录 clone 仓库：

```
/tmp/gateway-work/{task-id}/
├── repo/          # clone 的仓库
└── artifacts/     # Agent 产出物（日志、中间文件）
```

**流程**：
1. 创建临时目录 `/tmp/gateway-work/{task-id}/`
2. `git clone https://{agent_token}@gitea.example.com/{owner}/{repo}.git`
3. Agent 执行操作（写代码、运行测试等）
4. 推送分支 + 创建 PR
5. 任务完成后删除临时目录

**认证方式**：HTTPS + Agent 自己的 Gitea Token（URL 中嵌入 token）

**清理策略**：
- 任务成功 → 立即删除
- 任务失败 → 保留 24 小时（便于排查），然后定时清理
- 磁盘空间不足 → 强制清理最早的临时目录

### 5.2 沙箱隔离

> **TODO (Phase 2)**：Agent 代码执行改为 Docker 容器隔离，防止恶意代码影响 Gateway 服务器。
>
> 方案：每个任务启动一个临时容器，挂载 clone 的仓库，Agent 在容器内执行。任务结束后销毁容器。

### 5.3 大 Diff 处理

当 PR Diff 超过 LLM Token 限制时的降级策略：

- **阈值**：Diff 超过 8000 Token（约 32KB）
- **降级方案**：
  1. 只传变更文件列表 + 每个文件的前 50 行
  2. 或分文件审查，每次只审查一个文件，最后汇总
- **优先级**：新增/修改文件 > 删除文件；`.go`/`.py`/`.js` 等代码文件 > 配置文件 > 文档

> **TODO (Phase 2)**：支持语义化 Diff（基于 AST 解析，只传变更的函数/类，而非整行 Diff）。

## 六、事件流转设计

### 6.1 完整流转（需求→开发→审查→合并）

```
1. PM/研发 创建 Issue
   └─ Issue: "实现用户登录功能"

2. PM 打标签 `agent:analyze`，分配给 ai-analyze
   └─ Webhook: action=assigned, assignee=ai-analyze, label=agent:analyze
   └─ 触发 → 需求分析 Agent (ai-analyze)
   └─ Agent 输出需求分析报告，打标签 `analyzed`
   └─ @PM 回复分析结果

3. PM Review 需求分析报告
   └─ 确认无误后，打标签 `agent:dev:frontend`，分配给 ai-frontend-dev
   └─ Webhook: action=assigned, assignee=ai-frontend-dev, label=agent:dev:frontend
   └─ 触发 → 前端研发 Agent (ai-frontend-dev)

4. 前端研发 Agent 开始工作
   └─ clone 仓库 → 创建分支 → 实现 → 推送 → 创建 PR
   └─ PR 关联 Issue，打标签 `agent:dev:done`
   └─ @PM 通知已提交 PR
   └─ 清理临时目录

5. PR 审查 Agent (ai-reviewer) 自动触发
   └─ 审查代码 → 输出报告 → 打标签 `reviewed-by-agent`
   └─ @研发人员 审核

6. 研发人员审核 PR
   └─ 通过 → 合并
   └─ 不通过 → 评论修改意见，@ai-frontend-dev 要求修改
   └─ Agent 收到 @Mention → 根据评论修改代码 → 更新 PR
```

### 6.2 Bug 修复流转

```
1. 测试/用户 创建 Issue（Label: `bug`）
   └─ 描述 Bug 症状和复现步骤

2. 研发 打标签 `agent:bugfix`，分配给 ai-bugfix
   └─ 触发 → Bug 修复 Agent (ai-bugfix)
   └─ Agent 分析 → 定位 → 修复 → 提交 PR
   └─ @研发 通知修复完成

3. PR 审查 Agent (ai-reviewer) 自动触发
   └─ 审查修复代码

4. 研发人员审核合并
```

### 6.3 @Mention 互动流转

```
1. 研发人员在 Issue 评论: "@ai-frontend-dev 这个需求的技术方案能用 React 吗？"
   └─ Webhook: action=created, mention=ai-frontend-dev
   └─ 触发 → 前端研发 Agent (ai-frontend-dev)
   └─ Agent 基于 Issue 上下文回复技术建议

2. 研发人员在 PR 评论: "@ai-frontend-dev 这个函数的命名不太规范，改成 camelCase"
   └─ 触发 → 前端研发 Agent
   └─ Agent 修改代码，推送更新
   └─ @研发人员 通知已修改
```

## 七、Label 规范

| Label | 含义 | 触发动作 |
|-------|------|----------|
| `agent:analyze` | 需求分析 | 分配给 ai-analyze 时触发 |
| `agent:dev:frontend` | 前端研发 | 分配给 ai-frontend-dev 时触发 |
| `agent:dev:backend` | 后端研发 | 分配给 ai-backend-dev 时触发 |
| `agent:bugfix` | Bug修复 | 分配给 ai-bugfix 时触发 |
| `agent:test` | 测试 | 分配给 ai-tester 时触发 |
| `analyzed` | 已分析 | 需求分析 Agent 完成后自动添加 |
| `agent:dev:done` | 研发完成 | 研发 Agent 完成后自动添加 |
| `reviewed-by-agent` | 已审查 | PR 审查 Agent 完成后自动添加 |
| `human:review-needed` | 需人工审核 | Agent 完成后通知人工 |

**注意**：Label 不再是唯一触发条件，需要配合 Assignee 双条件匹配。

## 八、Webhook 配置方式

### 8.1 Phase 1：仓库级 Webhook（手动配置）

每个仓库单独在 Gitea 设置中添加 Webhook：
1. 进入仓库 → 设置 → Webhooks → 添加 Webhook → Gitea
2. 目标 URL：`http://your-server:8080/webhook/gitea`
3. 密钥：配置中的 Webhook 密钥
4. 选择事件：Issues、Pull Request、Issue Comment
5. 激活

### 8.2 Phase 2：API 自动注册

Gateway 启动时或新仓库创建时，通过 Gitea API 自动注册 Webhook：

```go
// POST /api/v1/repos/{owner}/{repo}/hooks
{
    "type": "gitea",
    "active": true,
    "config": {
        "url": "http://gateway:8080/webhook/gitea",
        "content_type": "json",
        "secret": "your-webhook-secret"
    },
    "events": ["issues", "issue_comment", "pull_request", "push"]
}
```

> **TODO (Phase 2)**：Gateway 监听 Gitea 系统事件，新仓库创建时自动注册 Webhook。

### 8.3 Phase 2：系统级 Webhook

> **TODO**：支持 Gitea 系统级 Webhook（管理员 → 系统设置 → Webhooks），一个 Gateway 服务所有仓库，无需逐个配置。

## 九、通知机制

### 9.1 Phase 1：Gitea @Mention 通知

Agent 完成任务后，在 Issue/PR 评论中 @相关人员，触发 Gitea 邮件/站内通知。

### 9.2 Phase 2：IM 通知

> **TODO (Phase 2)**：支持额外的 IM 通知渠道（可配置）。
>
> ```yaml
> notify:
>   channels:
>     - type: "dingtalk"
>       webhook: "https://oapi.dingtalk.com/robot/send?access_token=..."
>     - type: "feishu"
>       webhook: "https://open.feishu.cn/open-apis/bot/v2/hook/..."
>     - type: "wechat_work"
>       webhook: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..."
>     - type: "email"
>       smtp: "smtp.example.com:587"
>       from: "agent@example.com"
> ```
>
> 触发条件：Agent 任务成功/失败时，向配置的渠道推送通知。

## 十、技术选型

### 10.1 Webhook Gateway

- **语言**：Go 1.22+
- **前端**：Vue 3 + Element Plus（`go:embed` 打包）
- **存储**：SQLite（配置 + Agent 注册表 + 路由规则 + 任务队列 + 操作日志 + 幂等去重）
- **部署**：单二进制，零依赖

### 10.2 Agent 实现

- **LLM**：通过可配置的 Provider 调用（OpenAI 兼容 / Anthropic）
- **代码操作**：Git CLI（HTTPS + Agent 自己的 Gitea Token 认证）
- **Gitea API**：Go SDK（`gitea.com/gitea/go-sdk`）

### 10.3 配置示例

```yaml
server:
  host: "0.0.0.0"
  port: 8080

gitea:
  url: "https://git.example.com"
  admin_token: "${GITEA_ADMIN_TOKEN}"  # 用于自动注册账号
  webhook_secret: "${WEBHOOK_SECRET}"

workspace:
  base_dir: "/tmp/gateway-work"
  cleanup_after: "24h"
  max_disk_usage: "10GB"

dispatcher:
  max_concurrent: 3
  retry_count: 1
  timeout: 300
  queue_size: 100

database:
  path: "./data/gateway.db"

logging:
  level: "info"
  path: "./data/logs"
```

## 十一、安全考虑

1. **Webhook 签名验证**：HMAC-SHA256 验证 `X-Gitea-Signature`
2. **幂等去重**：基于 `X-Gitea-Delivery` 去重，防止重试导致重复执行
3. **API Token 权限最小化**：每个 Agent 只授予 `write:issue` + `write:repository`
4. **分支保护**：Agent 不能直接推送到 main/master，必须通过 PR
5. **人工审核必须**：Agent 的所有代码变更必须经过人工审核才能合并
6. **操作审计**：所有 Agent 操作记录到 SQLite 审计日志，可追溯到具体 Agent 账号
7. **回滚机制**：Agent 操作可一键回滚（Revert PR）
8. **临时目录清理**：任务完成后删除，失败任务 24h 后自动清理
9. **Agent 账号隔离**：每个 Agent 独立账号，权限互不影响

> **TODO (Phase 2)**：Docker 沙箱隔离 Agent 执行环境。

## 十二、实施路径

### Phase 1：基础框架（2周）
- [ ] 搭建 Webhook Gateway（Go 单二进制）
- [ ] 实现 Agent Dispatcher + Label+Assignee 双条件路由
- [ ] 实现 Gitea API 回写层（Go SDK）
- [ ] 基础配置管理（config.yaml + SQLite）
- [ ] 幂等去重（`X-Gitea-Delivery`）
- [ ] 临时目录管理 + 自动清理

### Phase 2：动态 Agent 管理（2周）
- [ ] Agent CRUD API（创建/读取/更新/删除）
- [ ] Gitea 账号自动注册（Admin API）
- [ ] Agent Token 自动生成
- [ ] Web UI Agent 管理页面
- [ ] 预置 Agent 模板
- [ ] @Mention 触发 Agent 互动

### Phase 3：核心 Agent 实现（3周）
- [ ] 需求分析 Agent
- [ ] PR 审查 Agent（支持大 Diff 降级）
- [ ] 前端/后端研发 Agent
- [ ] Bug 修复 Agent
- [ ] 基础测试覆盖

### Phase 4：优化迭代（持续）
- [ ] Agent 效果评估
- [ ] Prompt 优化
- [ ] 测试 Agent
- [ ] 多语言/框架支持
- [ ] API 自动注册 Webhook
- [ ] 系统级 Webhook 支持
- [ ] IM 通知（钉钉/飞书/企微/邮件）
- [ ] Docker 沙箱隔离
- [ ] 语义化 Diff（AST 解析）
- [ ] Agent 头像自定义上传

## 十三、预期收益

| 指标 | 当前 | 预期 | 提效 |
|------|------|------|------|
| 需求分析耗时 | 2-4h | 10-15min | 75%+ |
| Code Review 等待 | 4-8h | 即时 | 90%+ |
| Bug 定位时间 | 1-2h | 5-10min | 80%+ |
| 重复代码编写 | 人工 | Agent | 60%+ |

## 十四、与现有工具对比

| 方案 | 优势 | 劣势 |
|------|------|------|
| **本方案（Gitea+Webhook）** | 轻量、可控、与Git工作流无缝集成 | 需要自建维护 |
| MetaGPT | 开箱即用、角色齐全 | 黑盒、难定制、与现有Git流程割裂 |
| GitHub Copilot Workspace | 深度集成GitHub | 仅限GitHub、不开源 |
| Cursor/Windsurf | IDE内集成 | 单人工具、不支持团队协作流 |

本方案的核心优势：**以Git为唯一真相源，Agent是Git工作流的增强层，而非独立系统。每个Agent是团队中的虚拟成员，通过@Mention可随时互动。**
