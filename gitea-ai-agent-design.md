# Gitea + AI Agent 研发流程提效方案

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
- 签名验证：验证 Webhook 签名确保安全性

### 3.2 Agent Dispatcher

- 根据 Issue/PR 的 Label 或 Assignee 决定调用哪个 Agent
- 路由规则可配置（YAML/JSON 配置文件）
- 支持优先级和并发控制

### 3.3 Agent 回写层

- 统一封装 Gitea API 调用（评论、打标签、创建 PR、分配 Assignee）
- 处理 API 限流和重试
- 记录 Agent 操作日志

## 四、Agent 角色定义

### 4.1 需求分析 Agent

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

### 4.2 研发 Agent

**触发条件**：Issue 被打上 `agent:dev` 标签，且已被 Assign

**职责**：
- 根据 Issue 描述和需求分析报告，生成代码实现
- 创建新分支，提交代码
- 创建 PR，关联 Issue
- 在 PR 描述中说明实现思路

**工作流程**：
1. 拉取最新代码
2. 创建 `feat/issue-{number}-{short-desc}` 分支
3. 实现代码
4. 提交并推送
5. 创建 PR，关联 Issue
6. 在 Issue 评论中通知已提交 PR

### 4.3 Bug 修复 Agent

**触发条件**：Issue 被打上 `agent:bugfix` 标签，且 Label 为 `bug`

**职责**：
- 分析 Issue 中的错误描述和复现步骤
- 定位问题代码
- 生成修复代码
- 创建修复 PR，附带修复说明

### 4.4 PR 审查 Agent

**触发条件**：PR 被创建或更新，且关联的 Issue 有 `agent:dev` 或 `agent:bugfix` 标签

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

### 4.5 测试 Agent

**触发条件**：PR 被打上 `agent:test` 标签

**职责**：
- 根据 PR 变更生成测试用例
- 运行测试并报告结果
- 检查测试覆盖率

## 五、事件流转设计

### 5.1 完整流转（需求→开发→审查→合并）

```
1. PM/研发 创建 Issue
   └─ Issue: "实现用户登录功能"

2. PM 打标签 `agent:analyze`
   └─ 触发 → 需求分析 Agent
   └─ Agent 输出需求分析报告，打标签 `analyzed`

3. 研发 Review 需求分析报告
   └─ 确认无误后，打标签 `agent:dev`，Assign 给研发 Agent

4. 研发 Agent 开始工作
   └─ 拉代码 → 创建分支 → 实现 → 提交 PR
   └─ PR 关联 Issue，打标签 `agent:dev:done`

5. PR 审查 Agent 自动触发
   └─ 审查代码 → 输出报告 → 打标签 `reviewed-by-agent`
   └─ 通知研发人员审核

6. 研发人员审核 PR
   └─ 通过 → 合并
   └─ 不通过 → 评论修改意见，打标签 `agent:dev` 重新触发研发 Agent
```

### 5.2 Bug 修复流转

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

## 六、Label 规范

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

## 七、技术选型

### 7.1 Webhook Gateway

- **语言**：Go / Node.js / Python
- **框架**：轻量 HTTP 框架（Gin / Express / FastAPI）
- **部署**：Docker 容器，与 Gitea 同网络

### 7.2 Agent 实现

- **LLM**：Claude / GPT-4 / 本地模型（通过 API 调用）
- **代码操作**：Git CLI + Gitea API
- **工具链**：
  - 代码分析：AST 解析、Lint 工具
  - 测试生成：基于代码结构生成测试用例
  - 代码搜索：grep / ripgrep / 语义搜索

### 7.3 配置示例

```yaml
# config.yaml
gitea:
  url: "https://git.example.com"
  token: "${GITEA_TOKEN}"
  webhook_secret: "${WEBHOOK_SECRET}"

agents:
  analyze:
    model: "claude-sonnet-4-20250514"
    max_tokens: 4096
    temperature: 0.3

  dev:
    model: "claude-sonnet-4-20250514"
    max_tokens: 8192
    temperature: 0.2
    branch_prefix: "feat/"
    auto_pr: true

  bugfix:
    model: "claude-sonnet-4-20250514"
    max_tokens: 8192
    temperature: 0.1
    branch_prefix: "fix/"

  review:
    model: "claude-sonnet-4-20250514"
    max_tokens: 4096
    temperature: 0.2
    auto_notify: true

dispatcher:
  max_concurrent: 3
  retry_count: 2
  timeout: 300  # seconds
```

## 八、安全考虑

1. **Webhook 签名验证**：所有 Webhook 请求必须验证 HMAC 签名
2. **API Token 权限最小化**：Agent 使用的 Gitea Token 只授予必要权限
3. **分支保护**：Agent 不能直接推送到 main/master，必须通过 PR
4. **人工审核必须**：Agent 的所有代码变更必须经过人工审核才能合并
5. **操作审计**：所有 Agent 操作记录到审计日志
6. **回滚机制**：Agent 操作可一键回滚（Revert PR）

## 九、实施路径

### Phase 1：基础框架（2周）
- [ ] 搭建 Webhook Gateway
- [ ] 实现 Agent Dispatcher
- [ ] 实现 Gitea API 回写层
- [ ] 基础配置管理

### Phase 2：核心 Agent（3周）
- [ ] 需求分析 Agent
- [ ] PR 审查 Agent
- [ ] 基础测试覆盖

### Phase 3：研发 Agent（3周）
- [ ] 研发 Agent（代码生成 + PR 创建）
- [ ] Bug 修复 Agent
- [ ] 代码质量保障

### Phase 4：优化迭代（持续）
- [ ] Agent 效果评估
- [ ] Prompt 优化
- [ ] 测试 Agent
- [ ] 多语言/框架支持

## 十、预期收益

| 指标 | 当前 | 预期 | 提效 |
|------|------|------|------|
| 需求分析耗时 | 2-4h | 10-15min | 75%+ |
| Code Review 等待 | 4-8h | 即时 | 90%+ |
| Bug 定位时间 | 1-2h | 5-10min | 80%+ |
| 重复代码编写 | 人工 | Agent | 60%+ |

## 十一、与现有工具对比

| 方案 | 优势 | 劣势 |
|------|------|------|
| **本方案（Gitea+Webhook）** | 轻量、可控、与Git工作流无缝集成 | 需要自建维护 |
| MetaGPT | 开箱即用、角色齐全 | 黑盒、难定制、与现有Git流程割裂 |
| GitHub Copilot Workspace | 深度集成GitHub | 仅限GitHub、不开源 |
| Cursor/Windsurf | IDE内集成 | 单人工具、不支持团队协作流 |

本方案的核心优势：**以Git为唯一真相源，Agent是Git工作流的增强层，而非独立系统。**
