# 集成测试指南

## 快速验证步骤

### 1. 编译并启动服务

```bash
# 编译
go build -o gateway.exe .

# 使用示例配置启动
cp config.example.yaml config.yaml
# 编辑 config.yaml 填入真实的 Gitea URL 和 Token
./gateway.exe -config config.yaml
```

### 2. 配置测试 Agent 和 Route

使用管理 API 创建 Agent 和 Route：

```bash
# 创建 Agent
curl -X POST http://localhost:8080/api/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-agent",
    "gitea_username": "ai-agent",
    "gitea_token": "your-gitea-token",
    "provider": "deepseek",
    "model": "deepseek-chat",
    "system_prompt": "You are a helpful AI assistant. Analyze issues and provide suggestions.",
    "status": "active"
  }'

# 创建 Route (Issue 分配给 ai-agent 时触发)
curl -X POST http://localhost:8080/api/routes \
  -H "Content-Type: application/json" \
  -d '{
    "event": "issues",
    "action": "assigned",
    "agent_id": 1,
    "priority": 10
  }'
```

### 3. 测试 Webhook 端点

```bash
# 运行测试脚本
chmod +x scripts/test-webhook.sh
./scripts/test-webhook.sh 8080
```

### 4. 查看结果

```bash
# 查看任务列表
curl http://localhost:8080/api/tasks

# 查看任务详情
curl http://localhost:8080/api/tasks/1

# 查看操作日志
curl http://localhost:8080/api/logs
```

## 运行 Go 单元测试

```bash
# 需要 CGO 支持 SQLite
CGO_ENABLED=1 go test ./internal/... -v

# 只运行 dispatcher 测试
CGO_ENABLED=1 go test ./internal/dispatcher/ -v -run "Test"
```

## 完整集成测试流程

### 场景 1: Issue 分配触发 AI 分析

1. 在 Gitea 创建一个 Issue
2. 将 Issue 分配给 `ai-agent` 用户
3. 观察服务日志，应该看到：
   - Webhook 事件接收
   - Route 匹配成功
   - Task 创建并入队
   - LLM 调用
   - 结果回写到 Issue 评论

### 场景 2: PR 审查

1. 配置 PR 相关的 Route
2. 创建或更新 PR
3. AI Agent 自动审查并评论

### 场景 3: 评论回复

1. 配置 `issue_comment` Route
2. 在 Issue 中 @ai-agent
3. AI 自动回复

## 故障排查

### 任务卡在 pending 状态

- 检查 LLM 配置是否正确
- 查看服务日志中的错误信息
- 确认 Agent 状态为 `active`

### 结果未回写到 Gitea

- 检查 Agent 的 `gitea_token` 是否有评论权限
- 查看日志中的 writeback 错误
- 确认 Issue ID 正确

### 重复任务创建

- 检查 `processed_deliveries` 表
- 确认 Webhook Secret 配置正确
