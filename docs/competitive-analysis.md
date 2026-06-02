# 开源项目对比分析报告

> 分析时间：2026-06-02
> 分析项目：wshm (Rust) / ai-git-bot (Java) / gitea-agent-gateway (Go)

---

## 一、项目概览

| 维度 | wshm | ai-git-bot | gitea-agent-gateway (我们) |
|------|------|------------|---------------------------|
| **语言** | Rust | Java (Spring Boot) | Go |
| **架构** | 单二进制 + Pipeline | Agent-based + Web UI | 单二进制 + Dispatcher/Runner |
| **Git 平台** | GitHub, GitLab, Gitea, Azure DevOps | Gitea, GitHub, GitLab, Bitbucket | Gitea (专注) |
| **AI 提供商** | 14+ (Claude, OpenAI, Ollama 等) | 多家 (Claude, OpenAI, Ollama 等) | OpenAI 兼容, Anthropic |
| **部署方式** | Docker / 单二进制 | Docker / JAR | 单二进制 |
| **数据库** | SQLite / PostgreSQL | H2 / PostgreSQL | SQLite |
| **Web UI** | ✅ 有 | ✅ 有 | ❌ 无 |
| **License** | SSPL-1.0 | MIT | - |

---

## 二、功能对比矩阵

### 2.1 核心功能

| 功能 | wshm | ai-git-bot | 我们 | 说明 |
|------|------|------------|------|------|
| **Issue 分析** | ✅ | ✅ | ✅ | 基础功能，三方都有 |
| **PR 审查** | ✅ | ✅ | ✅ | 基础功能，三方都有 |
| **@Mention 回复** | ✅ | ✅ | ✅ | 基础功能，三方都有 |
| **代码生成** | ✅ (Pro) | ✅ | ✅ | 我们用 Tool-Use Agent |
| **Issue → PR** | ✅ | ✅ | ✅ | 核心功能 |
| **Bug 修复** | ✅ | ✅ | ✅ | 核心功能 |

### 2.2 高级功能

| 功能 | wshm | ai-git-bot | 我们 | 差距分析 |
|------|------|------------|------|----------|
| **Merge Queue** | ✅ | ❌ | ❌ | wshm 独有，自动合并队列 |
| **Issue Triage** | ✅ | ❌ | ❌ | wshm 独有，自动分类标签 |
| **PR Risk Analysis** | ✅ | ❌ | ❌ | wshm 独有，风险评估 |
| **PR Health Check** | ✅ | ❌ | ❌ | wshm 独有，僵尸 PR 检测 |
| **Writer Agent** | ❌ | ✅ | ❌ | ai-git-bot 独有，优化 Issue |
| **E2E 测试生成** | ❌ | ✅ | ❌ | ai-git-bot 独有，Playwright |
| **单元测试生成** | ❌ | ✅ | ❌ | ai-git-bot 独有 |
| **Interactive Q&A** | ❌ | ✅ | ❌ | ai-git-bot 独有，PR 内对话 |
| **Tool-Use Agent** | ❌ | ❌ | ✅ | 我们独有，Function Calling |
| **代码上下文加载** | ❌ | ✅ | ✅ | 我们和 ai-git-bot 有 |

### 2.3 基础设施

| 功能 | wshm | ai-git-bot | 我们 | 差距分析 |
|------|------|------------|------|----------|
| **Web UI** | ✅ | ✅ | ❌ | 我们缺失 |
| **CLI** | ✅ | ❌ | ❌ | wshm 独有 |
| **TUI** | ✅ | ❌ | ❌ | wshm 独有 |
| **Docker 部署** | ✅ | ✅ | ❌ | 我们缺失 |
| **多数据库支持** | ✅ | ✅ | ❌ | 我们只有 SQLite |
| **密钥管理** | ✅ (Vault) | ✅ | ❌ | 我们缺失 |
| **遥测/监控** | ✅ | ❌ | ❌ | wshm 独有 |
| **自动更新** | ✅ | ❌ | ❌ | wshm 独有 |

---

## 三、架构对比

### 3.1 wshm 架构 (Rust)

```
┌─────────────────────────────────────────────────────┐
│                    wshm daemon                       │
├─────────────────────────────────────────────────────┤
│  Sync Engine (Webhooks + Polling)                    │
│      ↓                                               │
│  Pipelines                                           │
│  ├── Triage Pipeline (Issue 分类)                    │
│  ├── PR Analysis Pipeline (PR 风险分析)              │
│  ├── Merge Queue Pipeline (自动合并)                 │
│  ├── PR Health Pipeline (僵尸 PR 检测)               │
│  └── Auto-fix Pipeline (代码修复)                    │
│      ↓                                               │
│  AI Client (14+ providers)                           │
│      ↓                                               │
│  Storage (SQLite/PostgreSQL + FTS5 + AES-256-GCM)   │
└─────────────────────────────────────────────────────┘
```

**优点**：
- Pipeline 模式，职责清晰
- 多平台支持广泛
- 功能丰富（Triage, Merge Queue 等）

**缺点**：
- SSPL License，商业使用受限
- Rust 学习曲线陡峭

### 3.2 ai-git-bot 架构 (Java)

```
┌─────────────────────────────────────────────────────┐
│                Spring Boot Application               │
├─────────────────────────────────────────────────────┤
│  Webhook Layer (Gitea/GitHub/GitLab/Bitbucket)      │
│      ↓                                               │
│  PrWorkflowOrchestrator (PR 工作流编排)              │
│      ↓                                               │
│  Agent Layer                                         │
│  ├── CodingAgent (Issue → Code)                      │
│  ├── WriterAgent (Issue → Better Issue)              │
│  ├── CriticAgent (代码审查)                          │
│  └── ReviewAgent (PR 审查)                           │
│      ↓                                               │
│  Tool Layer (文件读写, Git, 构建工具)                │
│      ↓                                               │
│  AI Client (多家提供商)                              │
└─────────────────────────────────────────────────────┘
```

**优点**：
- Agent-based 架构，灵活
- Web UI 功能完善
- 有 Writer Agent 和 E2E 测试生成
- MIT License，商业友好

**缺点**：
- Java 依赖重，部署复杂
- 需要 JVM 环境

### 3.3 我们的架构 (Go)

```
┌─────────────────────────────────────────────────────┐
│                gitea-agent-gateway                    │
├─────────────────────────────────────────────────────┤
│  Webhook Handler (Gitea 专用)                        │
│      ↓                                               │
│  Dispatcher                                          │
│  ├── Router (事件路由)                               │
│  ├── TaskQueue (SQLite + 内存队列)                   │
│  └── Executor (并发执行)                             │
│      ↓                                               │
│  Runner Factory                                      │
│  ├── AnalyzeRunner (只读)                            │
│  ├── ReviewRunner (只读)                             │
│  ├── InteractionRunner (只读)                        │
│  ├── DevRunner (写入, Tool-Use)                      │
│  └── BugfixRunner (写入, Tool-Use)                   │
│      ↓                                               │
│  Agent Loop (多轮对话 + Tool Calling)                │
│      ↓                                               │
│  Sandbox (目录隔离 + 命令白名单)                     │
└─────────────────────────────────────────────────────┘
```

**优点**：
- Go 单二进制，部署简单
- Tool-Use Agent，代码理解能力强
- 轻量级沙箱，无需 Docker
- 测试覆盖完善

**缺点**：
- 只支持 Gitea
- 无 Web UI
- 功能相对较少

---

## 四、可借鉴的改进点

### 4.1 高优先级 (P0)

| 改进项 | 来源 | 说明 | 工作量 |
|--------|------|------|--------|
| **Issue Triage** | wshm | 自动分类 Issue 并打标签 | 1-2 周 |
| **PR Risk Analysis** | wshm | 分析 PR 风险等级 | 1 周 |
| **Writer Agent** | ai-git-bot | 优化 Issue 描述，添加验收标准 | 1-2 周 |
| **Web UI** | 两者 | Agent 管理、任务监控、配置 | 2-3 周 |

### 4.2 中优先级 (P1)

| 改进项 | 来源 | 说明 | 工作量 |
|--------|------|------|--------|
| **Merge Queue** | wshm | 自动合并队列，阈值控制 | 2 周 |
| **Unit Test 生成** | ai-git-bot | 自动生成单元测试 | 1-2 周 |
| **多数据库支持** | 两者 | 支持 PostgreSQL | 1 周 |
| **Docker 部署** | 两者 | 提供 Dockerfile 和 docker-compose | 1 周 |

### 4.3 低优先级 (P2)

| 改进项 | 来源 | 说明 | 工作量 |
|--------|------|------|--------|
| **多平台支持** | 两者 | 支持 GitHub, GitLab | 4-6 周 |
| **E2E 测试生成** | ai-git-bot | Playwright 测试生成 | 3-4 周 |
| **密钥管理** | wshm | Vault/密钥加密存储 | 1-2 周 |
| **CLI 工具** | wshm | 命令行管理工具 | 2 周 |

---

## 五、我们的独特优势

### 5.1 Tool-Use Agent

我们实现了 Go 原生的 Tool-Use Agent，这是其他两个项目没有的：

```go
// LLM 通过 Function Calling 调用工具
tools := []Tool{
    read_file, write_file, list_files,
    search_code, run_command, apply_diff,
}

// Agent Loop 多轮对话
for i := 0; i < maxIterations; i++ {
    resp, _ := llm.ChatCompletion(ctx, messages, tools)
    if resp.FinishReason == "stop" {
        break
    }
    // 执行工具调用
    for _, call := range resp.ToolCalls {
        result := executeTool(call)
        messages = append(messages, ToolMessage(result))
    }
}
```

**优势**：
- LLM 可以真正理解代码库
- 支持迭代式开发（读 → 写 → 测试 → 修复）
- 比一次性生成代码更可靠

### 5.2 轻量级沙箱

不依赖 Docker，使用目录隔离 + 命令白名单：

```go
type Sandbox struct {
    BaseDir     string
    TaskID      int64
    AllowedCmds map[string]bool  // 命令白名单
    Timeout     time.Duration    // 超时控制
    MaxOutput   int              // 输出限制
}
```

**优势**：
- 部署简单，无需 Docker
- 安全可控，命令白名单
- 资源限制，防止滥用

### 5.3 Gitea 专注

我们专注于 Gitea，可以做得更深：
- 更好的 Gitea API 集成
- 更了解 Gitea 的特性
- 更轻量，无多平台抽象层

---

## 六、建议的改进路线图

### v0.4：功能增强 (2-3 周)

```
├── Issue Triage (自动分类标签)
├── PR Risk Analysis (风险评估)
├── Web UI (基础版)
└── Docker 部署支持
```

### v0.5：高级功能 (3-4 周)

```
├── Writer Agent (优化 Issue)
├── Unit Test 生成
├── Merge Queue
└── 多数据库支持 (PostgreSQL)
```

### v0.6+：长期演进

```
├── 多平台支持 (GitHub, GitLab)
├── E2E 测试生成
├── CLI 工具
└── 密钥管理
```

---

## 七、总结

### 我们的定位

**轻量级、Gitea 专注、Tool-Use 驱动的 AI Agent Gateway**

### 差异化竞争

| 维度 | wshm | ai-git-bot | 我们 |
|------|------|------------|------|
| **核心优势** | 功能全面 | Agent 灵活 | Tool-Use 代码理解 |
| **目标用户** | 大型团队 | Java 生态 | Gitea 用户 |
| **部署复杂度** | 中 | 高 | 低 |
| **代码理解能力** | 中 | 高 | 高 (Tool-Use) |

### 可借鉴的关键点

1. **从 wshm 借鉴**：Issue Triage, PR Risk Analysis, Merge Queue
2. **从 ai-git-bot 借鉴**：Writer Agent, Unit Test 生成, Agent 架构设计
3. **保持我们的优势**：Tool-Use Agent, 轻量级沙箱, Gitea 专注

---

## 附录：项目链接

- **wshm**: https://github.com/rtk-ai/wshm
- **ai-git-bot**: https://github.com/tmseidel/ai-git-bot
- **gitea-agent-gateway**: 当前项目
