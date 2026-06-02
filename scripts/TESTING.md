# 测试指南

## 测试架构

```
tests/
├── integration/          # 集成测试 (testify)
│   ├── helpers_test.go   # 测试辅助函数
│   ├── webhook_test.go   # Webhook 端到端测试
│   ├── agent_test.go     # Agent 生命周期测试
│   └── sandbox_test.go   # 沙箱集成测试
└── internal/             # 单元测试 (各包内 _test.go)
    ├── api/
    ├── agents/
    ├── dispatcher/
    ├── gitea/
    ├── llm/
    ├── sandbox/
    └── webhook/
```

## 运行测试

### 运行所有测试

```bash
go test ./... -count=1
```

### 运行单元测试

```bash
go test ./internal/... -v -count=1
```

### 运行集成测试

```bash
go test ./tests/integration/ -v -count=1
```

### 运行特定测试

```bash
# 运行特定测试函数
go test ./tests/integration/ -v -run TestWebhookIssueAssigned

# 运行特定包的测试
go test ./internal/sandbox/ -v -count=1
```

### 查看测试覆盖率

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 集成测试说明

### TestEnv (测试环境)

`TestEnv` 提供完整的测试环境：

```go
env := NewTestEnv(t)
defer env.Cleanup()

// 包含:
// - 内存 SQLite 数据库
// - Mock Gitea 服务器
// - Mock LLM 提供者
// - 完整的 HTTP 服务器
// - Dispatcher 实例
```

### Webhook 测试

```go
// 测试 Issue 分配触发 AI 分析
func TestWebhookIssueAssigned(t *testing.T) {
    env := NewTestEnv(t)
    defer env.Cleanup()

    // 创建 Agent 和 Route
    agent := env.CreateTestAgent(t)
    env.CreateTestRoute(t, agent.ID, "issues", "assigned")

    // 启动 Dispatcher
    env.Dispatcher.Start()

    // 发送 Webhook 事件
    env.SendWebhook("issues", "delivery-001", payload)

    // 等待任务完成
    task := env.WaitForTask(t, 1, "success", 10*time.Second)

    // 验证结果
    assert.Equal(t, "analyze_issue", task.TaskType)
    assert.NotEmpty(t, task.Result)
}
```

### Agent 测试

```go
// 测试 Agent CRUD
func TestAgentCRUD(t *testing.T) {
    env := NewTestEnv(t)
    defer env.Cleanup()

    // 创建 Agent
    resp, _ := env.APIRequest("POST", "/api/agents", createReq)
    assert.Equal(t, http.StatusCreated, resp.StatusCode)

    // 获取 Agent
    resp, _ = env.APIRequest("GET", "/api/agents/1", nil)
    assert.Equal(t, http.StatusOK, resp.StatusCode)

    // 验证 Token 隐藏
    var agent map[string]interface{}
    parseJSON(resp, &agent)
    _, hasToken := agent["gitea_token"]
    assert.False(t, hasToken)
}
```

### Sandbox 测试

```go
// 测试沙箱完整工作流
func TestSandboxFullWorkflow(t *testing.T) {
    cfg := sandbox.Config{
        BaseDir:   t.TempDir(),
        Timeout:   30 * time.Second,
        MaxOutput: 1024 * 1024,
    }

    sb := sandbox.New(cfg, 999)
    sb.Setup()
    defer sb.Cleanup()

    // 初始化 Git
    sb.Execute("git", "init")
    sb.Execute("git", "config", "user.email", "test@test.com")
    sb.Execute("git", "config", "user.name", "Test")

    // 创建文件
    sb.WriteFile("main.go", []byte("package main"))

    // Git 操作
    git := sandbox.NewGit(sb)
    git.Add()
    git.Commit("initial commit")
    git.CreateBranch("ai/dev/task-999")

    // 验证
    branch, _ := git.GetCurrentBranch()
    assert.Equal(t, "ai/dev/task-999", branch)
}
```

## 测试覆盖范围

| 模块 | 单元测试 | 集成测试 | 状态 |
|------|----------|----------|------|
| API 认证 | ✅ | ✅ | 完成 |
| Agent CRUD | - | ✅ | 完成 |
| Route CRUD | - | ✅ | 完成 |
| Webhook 处理 | ✅ | ✅ | 完成 |
| Webhook 去重 | - | ✅ | 完成 |
| 任务队列 | ✅ | - | 完成 |
| 任务执行 | ✅ | ✅ | 完成 |
| LLM 调用 | ✅ | - | 完成 |
| Gitea API | ✅ | - | 完成 |
| 沙箱操作 | ✅ | ✅ | 完成 |
| Git 操作 | ✅ | ✅ | 完成 |
| 命令白名单 | ✅ | ✅ | 完成 |
| 分支验证 | ✅ | ✅ | 完成 |

## 添加新测试

### 添加单元测试

在对应包目录创建 `xxx_test.go` 文件：

```go
package mypackage

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMyFunction(t *testing.T) {
    result := MyFunction()
    assert.Equal(t, expected, result)
}
```

### 添加集成测试

在 `tests/integration/` 目录创建测试文件：

```go
package integration

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMyIntegration(t *testing.T) {
    env := NewTestEnv(t)
    defer env.Cleanup()

    // 使用 env 进行测试
    // - env.DB: 数据库
    // - env.Server: HTTP 服务器
    // - env.GiteaMock: Mock Gitea
    // - env.APIRequest(): 发送 API 请求
    // - env.SendWebhook(): 发送 Webhook
}
```

## 最佳实践

1. **使用 testify**: 断言更清晰，错误信息更友好
2. **使用 TestEnv**: 自动管理测试环境和清理
3. **使用 Mock**: 避免依赖外部服务
4. **测试独立性**: 每个测试应该独立运行
5. **清理资源**: 使用 defer 确保资源释放
6. **有意义的断言**: 验证关键行为，而不是所有细节
