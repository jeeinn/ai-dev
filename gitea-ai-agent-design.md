# Gitea + AI Agent 研发流程提效方案（v2 细化版）

## 一、核心理念

以 **Gitea 为研发协作中枢**，通过 Webhook 事件驱动，将 AI Agent 无缝嵌入现有 Git 工作流。
不替代人的决策权，而是让 Agent 承担重复性脑力劳动，人只做审核和最终决策。

## 二、系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                        Gitea (中枢)                          │
│  Issues / PR / Labels / Assignees / Webhooks / API          │
└──────────┬──────────────────────────────────┬───────────────┘
           │ Webhook Events                   │ API 调用
           ▼                                  ▲
┌─────────────────────┐              ┌─────────────────────────┐
│   Webhook Gateway   │──────────────│     Agent 回写层         │
│  (事件路由 & 过滤)    │              │  (评论Issue/提交PR/      │
└─────────┬───────────┘              │   打Label/通知)          │
          │                          └─────────────────────────┘
          ▼                                    ▲
┌─────────────────────┐                        │
│  Agent Dispatcher   │────────────────────────│
│  (标签→Agent 路由)   │                        │
└─────────┬───────────┘                        │
          │                                    │
    ┌─────┼──────┬──────────┬──────────┐       │
    ▼     ▼      ▼          ▼          ▼       │
┌──────┐┌──────┐┌──────┐┌────────┐┌────────┐  │
│需求分析││研发  ││Bug修复││PR审查  ││测试    │  │
│Agent  ││Agent ││Agent ││Agent   ││Agent   │──┘
└──────┘└──────┘└──────┘└────────┘└────────┘
```

## 三、核心组件

### 3.1 Webhook Gateway

- 监听 Gitea Webhook 事件（`issues`, `pull_request`, `issue_comment`, `push`）
- 事件过滤：只处理与 AI Agent 相关的事件（通过 Label 或 Assignee 区分）
- 事件队列：异步处理，避免阻塞 Gitea 响应
- 签名验证：HMAC-SHA256 验证（`X-Gitea-Signature` 头部）
- 幂等去重：基于 `X-Gitea-Delivery` 唯一 ID 去重，防止 Webhook 重试导致重复执行

### 3.2 Agent Dispatcher

- 根据 Issue/PR 的 Label 或 Assignee 决定调用哪个 Agent
- 路由规则可配置（YAML/JSON 配置文件）
- 支持优先级和并发控制

### 3.3 Agent 回写层

- 统一封装 Gitea API 调用（评论、打标签、创建 PR、分配 Assignee）
- 处理 API 限流和重试
- 记录 Agent 操作日志

## 四、Gitea 账号与权限

### 4.1 专用 AI Agent 账号

创建专用的 `ai-agent` Gitea 账号，不复用管理员账号。

**Token 权限最小化**：
- `write:issue` — 评论 Issue、打标签
- `write:repository` — 创建分支、推送代码、创建 PR
- `read:repository` — 读取仓库内容

**不授予**：`admin`、`write:user`、`write:organization` 等无关权限。

### 4.2 Phase 2 扩展

> **TODO (Phase 2)**：支持多 Agent 账号（不同 Agent 用不同 Gitea 账号，便于区分操作来源）。

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
2. `git clone https://{token}@gitea.example.com/{owner}/{repo}.git`
3. Agent 执行操作（写代码、运行测试等）
4. 推送分支 + 创建 PR
5. 任务完成后删除临时目录

**认证方式**：HTTPS + Gitea Token（URL 中嵌入 token，如 `https://ai-agent:{token}@gitea.example.com/...`）

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

## 六、Agent 角色定义

### 6.1 需求分析 Agent

**触发条件**：Issue 被打上 `agent:analyze` 标签

**职责**：
- 读取 Issue 描述，分析需求完整性
- 识别缺失信息（边界条件、验收标准、技术约束）
- 在 Issue 评论中输出结构化的需求分析报告
- 自动打上 `analyzed` 标签

**输出模板**：
```markdown
## 📋 需求分析报告

### 需求理解
- [简要描述需求要点]

### 缺失信息
- [ ] 边界条件：...
- [ ] 验收标准：...
- [ ] 技术约束：...

### 建议的技术方案方向
- [方案建议]

### 风险评估
- [潜在风险点]

---
*由 AI 需求分析 Agent 自动生成*
```

### 6.2 研发 Agent

**触发条件**：Issue 被打上 `agent:dev` 标签，且已被 Assign

**职责**：
- 根据 Issue 描述和需求分析报告，生成代码实现
- 创建新分支，提交代码
- 创建 PR，关联 Issue
- 在 PR 描述中说明实现思路

**工作流程**：
1. 临时目录 clone 仓库
2. 创建 `feat/issue-{number}-{short-desc}` 分支
3. 实现代码
4. 提交并推送
5. 创建 PR，关联 Issue
6. 在 Issue 评论中通知已提交 PR
7. 清理临时目录

### 6.3 Bug 修复 Agent

**触发条件**：Issue 被打上 `agent:bugfix` 标签，且 Label 为 `bug`

**职责**：
- 分析 Issue 中的错误描述和复现步骤
- 定位问题代码
- 生成修复代码
- 创建修复 PR，附带修复说明

### 6.4 PR 审查 Agent

**触发条件**（二选一）：
- **方式 A**：PR 关联的 Issue 有 `agent:dev` 或 `agent:bugfix` 标签 → 自动触发
- **方式 B**：仓库打上 `agent:auto-review` 标签 → 所有 PR 自动触发审查（Phase 2）

**职责**：
- 审查代码变更（逻辑、安全、性能、风格）
- 检查是否符合项目编码规范
- 在 PR 评论中输出审查报告
- 自动打上 `reviewed-by-agent` 标签
- 通知相关人员审核

**输出模板**：
```markdown
## 🔍 Code Review Report

### 整体评价
- 代码质量：⭐⭐⭐⭐ (4/5)
- 风险等级：低/中/高

### 发现的问题
#### 🔴 严重 (Must Fix)
- [问题描述 + 文件位置 + 建议修复]

#### 🟡 建议 (Should Fix)
- [问题描述 + 建议]

#### 🟢 优化 (Nice to Have)
- [优化建议]

### 测试覆盖
- [ ] 单元测试是否覆盖
- [ ] 边界条件是否测试

---
*由 AI PR 审查 Agent 自动生成，请人工复核后合并*
```

> **TODO (Phase 2)**：仓库级 `agent:auto-review` 标签，所有 PR 自动触发审查，无需 Issue 关联。

### 6.5 测试 Agent

**触发条件**：PR 被打上 `agent:test` 标签

**职责**：
- 根据 PR 变更生成测试用例
- 运行测试并报告结果
- 检查测试覆盖率

## 七、事件流转设计

### 7.1 完整流转（需求→开发→审查→合并）

```
1. PM/研发 创建 Issue
   └─ Issue: "实现用户登录功能"

2. PM 打标签 `agent:analyze`
   └─ 触发 → 需求分析 Agent
   └─ Agent 输出需求分析报告，打标签 `analyzed`

3. 研发 Review 需求分析报告
   └─ 确认无误后，打标签 `agent:dev`，Assign 给研发 Agent

4. 研发 Agent 开始工作
   └─ clone 仓库 → 创建分支 → 实现 → 推送 → 创建 PR
   └─ PR 关联 Issue，打标签 `agent:dev:done`
   └─ 清理临时目录

5. PR 审查 Agent 自动触发
   └─ 审查代码 → 输出报告 → 打标签 `reviewed-by-agent`
   └─ 通知研发人员审核

6. 研发人员审核 PR
   └─ 通过 → 合并
   └─ 不通过 → 评论修改意见，打标签 `agent:dev` 重新触发研发 Agent
```

### 7.2 Bug 修复流转

```
1. 测试/用户 创建 Issue（Label: `bug`）
   └─ 描述 Bug 症状和复现步骤

2. 研发 打标签 `agent:bugfix`
   └─ 触发 → Bug 修复 Agent
   └─ Agent 分析 → 定位 → 修复 → 提交 PR

3. PR 审查 Agent 自动触发
   └─ 审查修复代码

4. 研发人员审核合并
```

## 八、Label 规范

| Label | 含义 | 触发动作 |
|-------|------|----------|
| `agent:analyze` | 需求分析 | 触发需求分析 Agent |
| `agent:dev` | 研发实现 | 触发研发 Agent |
| `agent:bugfix` | Bug修复 | 触发 Bug 修复 Agent |
| `agent:test` | 测试 | 触发测试 Agent |
| `agent:review` | PR审查 | 触发 PR 审查 Agent |
| `analyzed` | 已分析 | 需求分析 Agent 完成后自动添加 |
| `agent:dev:done` | 研发完成 | 研发 Agent 完成后自动添加 |
| `reviewed-by-agent` | 已审查 | PR 审查 Agent 完成后自动添加 |
| `human:review-needed` | 需人工审核 | Agent 完成后通知人工 |
| `agent:auto-review` | 仓库级自动审查 | Phase 2：所有 PR 自动触发审查 |

## 九、Webhook 配置方式

### 9.1 Phase 1：仓库级 Webhook（手动配置）

每个仓库单独在 Gitea 设置中添加 Webhook：
1. 进入仓库 → 设置 → Webhooks → 添加 Webhook → Gitea
2. 目标 URL：`http://your-server:8080/webhook/gitea`
3. 密钥：配置中的 Webhook 密钥
4. 选择事件：Issues、Pull Request、Issue Comment
5. 激活

### 9.2 Phase 2：API 自动注册

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

### 9.3 Phase 2：系统级 Webhook

> **TODO**：支持 Gitea 系统级 Webhook（管理员 → 系统设置 → Webhooks），一个 Gateway 服务所有仓库，无需逐个配置。

## 十、通知机制

### 10.1 Phase 1：Gitea 评论通知

Agent 完成任务后，通过 Gitea Issue/PR 评论通知相关人员。评论中 @ 相关人员触发 Gitea 邮件通知。

### 10.2 Phase 2：IM 通知

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

## 十一、技术选型

### 11.1 Webhook Gateway

- **语言**：Go 1.22+
- **前端**：Vue 3 + Element Plus（`go:embed` 打包）
- **存储**：SQLite（配置 + 任务队列 + 操作日志 + 幂等去重）
- **部署**：单二进制，零依赖

### 11.2 Agent 实现

- **LLM**：通过可配置的 Provider 调用（OpenAI 兼容 / Anthropic）
- **代码操作**：Git CLI（HTTPS + Token 认证）
- **Gitea API**：Go SDK（`gitea.com/gitea/go-sdk`）

### 11.3 配置示例

```yaml
server:
  host: "0.0.0.0"
  port: 8080

gitea:
  url: "https://git.example.com"
  token: "${GITEA_TOKEN}"
  webhook_secret: "${WEBHOOK_SECRET}"
  agent_username: "ai-agent"  # 专用 Agent 账号

workspace:
  base_dir: "/tmp/gateway-work"
  cleanup_after: "24h"        # 失败任务保留时间
  max_disk_usage: "10GB"      # 临时目录最大磁盘占用

dispatcher:
  max_concurrent: 3
  retry_count: 1              # 超时后自动重试次数
  timeout: 300                # 单任务超时（秒）
  queue_size: 100

database:
  path: "./data/gateway.db"

logging:
  level: "info"
  path: "./data/logs"

# Phase 2: IM 通知
# notify:
#   channels:
#     - type: "dingtalk"
#       webhook: "..."
```

## 十二、安全考虑

1. **Webhook 签名验证**：HMAC-SHA256 验证 `X-Gitea-Signature`
2. **幂等去重**：基于 `X-Gitea-Delivery` 去重，防止重试导致重复执行
3. **API Token 权限最小化**：只授予 `write:issue` + `write:repository`
4. **分支保护**：Agent 不能直接推送到 main/master，必须通过 PR
5. **人工审核必须**：Agent 的所有代码变更必须经过人工审核才能合并
6. **操作审计**：所有 Agent 操作记录到 SQLite 审计日志
7. **回滚机制**：Agent 操作可一键回滚（Revert PR）
8. **临时目录清理**：任务完成后删除，失败任务 24h 后自动清理

> **TODO (Phase 2)**：Docker 沙箱隔离 Agent 执行环境。

## 十三、实施路径

### Phase 1：基础框架（2周）
- [ ] 搭建 Webhook Gateway（Go 单二进制）
- [ ] 实现 Agent Dispatcher + 任务队列
- [ ] 实现 Gitea API 回写层（Go SDK）
- [ ] 基础配置管理（config.yaml + SQLite）
- [ ] 幂等去重（`X-Gitea-Delivery`）
- [ ] 临时目录管理 + 自动清理

### Phase 2：核心 Agent（3周）
- [ ] 需求分析 Agent
- [ ] PR 审查 Agent（支持大 Diff 降级）
- [ ] 基础测试覆盖
- [ ] Web UI 可视化配置

### Phase 3：研发 Agent（3周）
- [ ] 研发 Agent（代码生成 + PR 创建）
- [ ] Bug 修复 Agent
- [ ] 代码质量保障

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
- [ ] 多 Agent 账号支持
- [ ] 仓库级 `agent:auto-review` 标签

## 十四、预期收益

| 指标 | 当前 | 预期 | 提效 |
|------|------|------|------|
| 需求分析耗时 | 2-4h | 10-15min | 75%+ |
| Code Review 等待 | 4-8h | 即时 | 90%+ |
| Bug 定位时间 | 1-2h | 5-10min | 80%+ |
| 重复代码编写 | 人工 | Agent | 60%+ |

## 十五、与现有工具对比

| 方案 | 优势 | 劣势 |
|------|------|------|
| **本方案（Gitea+Webhook）** | 轻量、可控、与Git工作流无缝集成 | 需要自建维护 |
| MetaGPT | 开箱即用、角色齐全 | 黑盒、难定制、与现有Git流程割裂 |
| GitHub Copilot Workspace | 深度集成GitHub | 仅限GitHub、不开源 |
| Cursor/Windsurf | IDE内集成 | 单人工具、不支持团队协作流 |

本方案的核心优势：**以Git为唯一真相源，Agent是Git工作流的增强层，而非独立系统。**
