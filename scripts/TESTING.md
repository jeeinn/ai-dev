# 测试指南

## 测试架构

```
tests/
├── integration/              # 集成测试 (需要 TestEnv)
│   ├── helpers_test.go       # 测试辅助函数 (TestEnv, MockGitea, MockLLM)
│   ├── webhook_test.go       # Webhook 端到端测试
│   ├── agent_test.go         # Agent 生命周期测试
│   └── sandbox_test.go       # 沙箱集成测试 (完整工作流)
│
internal/                     # 单元测试 (各包内 _test.go)
├── agent/
│   └── tools_test.go         # Tool 注册、执行测试
├── agents/
│   └── runners_test.go       # Runner 工厂测试
├── api/
│   └── auth_test.go          # 认证中间件测试
├── dispatcher/
│   ├── dispatcher_test.go    # Dispatcher 集成测试
│   ├── router_test.go        # 路由匹配测试
│   └── template_test.go      # 模板渲染测试
├── gitea/
│   ├── client_test.go        # API 客户端测试
│   └── pr_test.go            # PR API 测试
├── llm/
│   └── provider_test.go      # Provider 接口测试
├── sandbox/
│   ├── sandbox_test.go       # 沙箱基础功能测试
│   └── git_test.go           # Git 操作 + 分支验证测试
└── webhook/
    └── parser_test.go        # 事件解析测试
```

## 测试分类标准

| 类型 | 标准 | 示例 |
|------|------|------|
| **单元测试** | 不依赖外部服务，测试单个函数/方法 | `TestSandboxIsAllowed`, `TestValidateBranchName` |
| **集成测试** | 需要 TestEnv（数据库、HTTP Server、Mock Gitea） | `TestWebhookIssueAssigned`, `TestAgentCRUD` |

### 判断原则

```
需要 TestEnv?
├── 是 → 放在 tests/integration/
└── 否 → 放在 internal/xxx/
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

### 运行特定包的测试

```bash
# 运行 sandbox 包的测试
go test ./internal/sandbox/ -v -count=1

# 运行 agent 包的测试
go test ./internal/agent/ -v -count=1
```

### 运行特定测试

```bash
# 运行特定测试函数
go test ./tests/integration/ -v -run TestWebhookIssueAssigned

# 运行匹配模式的测试
go test ./internal/sandbox/ -v -run "TestGit.*"
```

### 查看测试覆盖率

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 集成测试说明

### TestEnv (测试环境)

`TestEnv` 提供完整的测试环境，自动管理生命周期：

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

### 使用 TestEnv 的测试

```go
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
}
```

## 单元测试说明

### 统一使用 testify

所有单元测试统一使用 `testify` 的 `assert` 和 `require`：

```go
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestExample(t *testing.T) {
    result, err := SomeFunction()

    // require: 失败时立即停止当前测试
    require.NoError(t, err)
    require.NotNil(t, result)

    // assert: 失败时继续执行后续断言
    assert.Equal(t, "expected", result.Value)
    assert.True(t, result.Valid)
}
```

### assert vs require

| 函数 | 行为 | 使用场景 |
|------|------|----------|
| `require.NoError(t, err)` | 失败立即停止 | 前置条件，后续代码依赖此结果 |
| `assert.NoError(t, err)` | 失败继续执行 | 独立断言，可以收集多个失败 |

## 测试覆盖范围

| 模块 | 单元测试 | 集成测试 | 说明 |
|------|----------|----------|------|
| Agent Tools | ✅ | - | Tool 注册、执行、未知工具处理 |
| API Auth | ✅ | ✅ | 认证中间件、Token 验证 |
| Agent CRUD | - | ✅ | 创建、查询、更新、删除 |
| Route CRUD | - | ✅ | 创建、查询、删除 |
| Webhook | ✅ | ✅ | 事件解析、去重、无匹配路由 |
| Task Queue | ✅ | - | 入队、出队、持久化 |
| Task Execute | ✅ | ✅ | Runner 选择、LLM 调用 |
| LLM Provider | ✅ | - | Function Calling 支持 |
| Gitea API | ✅ | - | Issue/PR/Repo 操作 |
| Sandbox | ✅ | ✅ | 文件读写、命令执行、白名单 |
| Git Operations | ✅ | ✅ | Clone/Branch/Commit/Push |
| Branch Validate | ✅ | - | 分支名验证、生成 |
| Template Render | ✅ | - | Go template 渲染 |

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
    result, err := MyFunction()
    require.NoError(t, err)
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
2. **使用 TestEnv**: 集成测试自动管理环境和清理
3. **使用 Mock**: 避免依赖外部服务
4. **测试独立性**: 每个测试应该独立运行
5. **清理资源**: 使用 `defer` 确保资源释放
6. **有意义的断言**: 验证关键行为，而不是所有细节
7. **避免重复**: 单元测试和集成测试不重复覆盖相同场景
8. **遵循分类标准**: 需要 TestEnv 的放集成测试，否则放单元测试
