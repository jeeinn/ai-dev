# 部署指南

本文档说明如何部署 Gitea Agent Gateway 到生产环境。

## 目录

- [环境要求](#环境要求)
- [快速部署](#快速部署)
- [配置说明](#配置说明)
- [Systemd 服务](#systemd-服务)
- [Docker 部署](#docker-部署)
- [反向代理](#反向代理)
- [Gitea 配置](#gitea-配置)
- [运维管理](#运维管理)
- [故障排查](#故障排查)

## 环境要求

| 项目 | 要求 |
|------|------|
| 操作系统 | Linux (推荐) / macOS / Windows |
| Go | 1.26+（仅从源码构建时） |
| 内存 | ≥ 512MB |
| 磁盘 | ≥ 1GB（含工作空间） |
| 网络 | 能访问 Gitea 和 LLM API |

## 快速部署

### 方式一：从源码构建

```bash
# 克隆代码
git clone https://github.com/your-org/gitea-agent-gateway.git
cd gitea-agent-gateway

# 构建前端
cd web && npm install && npm run build && cd ..

# 构建后端（前端资源通过 go:embed 打包进二进制）
go build -o gateway .

# 准备配置
cp config.example.yaml config.yaml
# 编辑 config.yaml

# 运行
./gateway -config config.yaml
```

### 方式二：使用预编译二进制

从 Releases 页面下载对应平台的二进制文件，直接运行：

```bash
chmod +x gateway
./gateway -config config.yaml
```

## 配置说明

### 环境变量

配置文件支持 `${VAR}` 和 `${VAR:-default}` 语法引用环境变量：

```yaml
gitea:
  admin_token: "${GITEA_ADMIN_TOKEN}"
  webhook_secret: "${GITEA_WEBHOOK_SECRET:-default-secret}"

llm:
  providers:
    deepseek:
      api_key: "${DEEPSEEK_API_KEY}"
```

建议通过环境变量管理敏感信息，不要将 Token 直接写入配置文件。

### 核心配置段

```yaml
server:
  host: "0.0.0.0"    # 监听地址
  port: 8080          # 监听端口

gitea:
  url: "https://gitea.example.com"
  admin_token: "${GITEA_ADMIN_TOKEN}"
  webhook_secret: "${GITEA_WEBHOOK_SECRET}"

database:
  path: "./data/gateway.db"   # SQLite 数据库路径

workspace:
  base_dir: "./data/work"     # Agent 工作目录
  cleanup_after: "24h"        # 失败任务保留时间

dispatcher:
  max_concurrent: 3           # 最大并发 Agent 数
  task_retry_count: 1         # 整任务失败重试次数
  rate_limit_backoff: 30      # LLM 429 退避（秒）
  queue_size: 100             # 任务队列大小

llm:
  providers:
    deepseek:
      base_url: "https://api.deepseek.com/v1"
      api_key: "${DEEPSEEK_API_KEY}"
  defaults:
    provider: "deepseek"
    model: "deepseek-chat"
  rate_limit_retries: 1       # 单次 ChatCompletion 遇 429 后的重试次数

auth:
  jwt_secret: "${JWT_SECRET:-change-me-in-production}"
  jwt_expiration: "24h"
  default_admin_password: "${ADMIN_PASSWORD:-admin123}"

api:
  auth_token: "${API_AUTH_TOKEN}"   # 管理 API 认证 Token

agents:
  defaults:
    max_output_tokens: 8192
    max_input_tokens: 115200
    temperature: 0.3
    timeout: "5m"             # 单次任务超时
  loop:
    max_iterations: 20
    total_timeout: "30m"      # 多轮任务总超时
```

### 配置 LLM Provider

支持多个 Provider 同时配置，Agent 通过 `provider` 字段选择：

```yaml
llm:
  providers:
    deepseek:
      base_url: "https://api.deepseek.com/v1"
      api_key: "${DEEPSEEK_API_KEY}"
    openai:
      base_url: "https://api.openai.com/v1"
      api_key: "${OPENAI_API_KEY}"
    claude:
      api_key: "${ANTHROPIC_API_KEY}"
    ollama:
      base_url: "http://localhost:11434/v1"
      api_key: "ollama"
```

## Systemd 服务

创建服务文件 `/etc/systemd/system/gateway.service`：

```ini
[Unit]
Description=Gitea Agent Gateway
After=network.target

[Service]
Type=simple
User=gateway
Group=gateway
WorkingDirectory=/opt/gateway
ExecStart=/opt/gateway/gateway -config /opt/gateway/config.yaml
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# 安全加固
NoNewPrivileges=yes
ProtectSystem=strict
ReadWritePaths=/opt/gateway/data

# 环境变量
EnvironmentFile=/opt/gateway/.env

[Install]
WantedBy=multi-user.target
```

创建环境文件 `/opt/gateway/.env`：

```bash
GITEA_ADMIN_TOKEN=your-token-here
GITEA_WEBHOOK_SECRET=your-secret-here
DEEPSEEK_API_KEY=sk-xxx
JWT_SECRET=your-jwt-secret
ADMIN_PASSWORD=your-admin-password
API_AUTH_TOKEN=your-api-token
```

启动服务：

```bash
# 创建用户和目录
sudo useradd -r -s /bin/false gateway
sudo mkdir -p /opt/gateway/data
sudo cp gateway config.yaml /opt/gateway/
sudo cp .env /opt/gateway/
sudo chown -R gateway:gateway /opt/gateway

# 启动
sudo systemctl daemon-reload
sudo systemctl enable gateway
sudo systemctl start gateway

# 查看状态
sudo systemctl status gateway
sudo journalctl -u gateway -f
```

## Docker 部署

### Dockerfile

```dockerfile
# 构建阶段
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git nodejs npm
WORKDIR /src
COPY . .

# 构建前端
RUN cd web && npm install && npm run build

# 构建后端
RUN go build -o gateway .

# 运行阶段
FROM alpine:3.19
RUN apk add --no-cache ca-certificates git
WORKDIR /app
COPY --from=builder /src/gateway .
COPY --from=builder /src/config.example.yaml config.yaml

RUN mkdir -p /app/data
VOLUME /app/data

EXPOSE 8080
ENTRYPOINT ["./gateway"]
CMD ["-config", "config.yaml"]
```

### Docker Compose

```yaml
version: '3.8'

services:
  gateway:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - gateway-data:/app/data
      - ./config.yaml:/app/config.yaml:ro
    environment:
      - GITEA_ADMIN_TOKEN=${GITEA_ADMIN_TOKEN}
      - GITEA_WEBHOOK_SECRET=${GITEA_WEBHOOK_SECRET}
      - DEEPSEEK_API_KEY=${DEEPSEEK_API_KEY}
      - JWT_SECRET=${JWT_SECRET}
      - ADMIN_PASSWORD=${ADMIN_PASSWORD}
    restart: unless-stopped

volumes:
  gateway-data:
```

```bash
docker compose up -d
```

## 反向代理

### Nginx

```nginx
server {
    listen 443 ssl;
    server_name gateway.example.com;

    ssl_certificate /etc/ssl/certs/gateway.crt;
    ssl_certificate_key /etc/ssl/private/gateway.key;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket 支持（如果需要）
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # 超时设置（Agent 任务可能较长）
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```

### Caddy

```
gateway.example.com {
    reverse_proxy localhost:8080
}
```

## Gitea 配置

### 创建管理员 Token

1. 登录 Gitea 管理员账号
2. 进入 **个人设置 → 应用 → 管理访问令牌**
3. 创建 Token，权限选择：**全部**（需要 admin 权限来创建 Agent 账号）
4. 将 Token 填入 `config.yaml` 的 `gitea.admin_token`

### 配置 Webhook

在需要 AI Agent 的仓库中：

1. 进入 **仓库设置 → Webhooks → 添加 Webhook → Gitea**
2. 配置：
   - **目标 URL**: `https://gateway.example.com/webhook/gitea`
   - **密钥**: 与 `config.yaml` 中的 `webhook_secret` 一致
   - **触发事件**: 勾选 `Push Events`、`Pull Requests`、`Issue Events`、`Issue Comment Events`
3. 保存并测试

### 使用 Agent

在 Issue 或 PR 中通过标签触发 Agent：

| 操作 | 方式 |
|------|------|
| 需求分析 | 给 Issue 添加 `ai:analyze` 标签 |
| 代码审查 | 给 PR 添加 `ai:review` 标签 |
| 开发实现 | 给 Issue 添加 `ai:solve` 标签 |
| Bug 修复 | 给 Issue 添加 `ai:fix` 标签 |
| 评论互动 | 在评论中 @Agent 用户名 |

## 运维管理

### 管理 API

通过 HTTP API 管理 Agent、任务和路由：

```bash
# 设置认证 Token
TOKEN="your-api-auth-token"

# 列出所有 Agent
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/agents

# 创建 Agent
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"dev-agent","type":"solve","provider":"deepseek","model":"deepseek-chat"}' \
  http://localhost:8080/api/agents

# 查看任务列表
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/tasks

# 查看统计
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/stats
```

### Web UI

访问 `https://gateway.example.com` 使用 Web 管理界面：

- **Dashboard**: 任务统计、成功率、系统状态
- **Agent 管理**: 创建/编辑/启用/禁用 Agent
- **任务列表**: 查看/取消/重试任务
- **Prompt 编辑**: 管理 System Prompt 和 User Template
- **用户管理**: 管理 Web UI 用户（仅 admin）

### 数据备份

```bash
# 备份数据库
cp data/gateway.db data/gateway.db.bak

# 或使用 SQLite 命令
sqlite3 data/gateway.db ".backup data/gateway-backup.db"
```

### 日志查看

日志输出到 stdout/stderr，使用 Systemd 时通过 journalctl 查看：

```bash
# 实时日志
journalctl -u gateway -f

# 最近 100 行
journalctl -u gateway -n 100

# 按时间过滤
journalctl -u gateway --since "2024-01-01" --until "2024-01-02"
```

日志级别在配置中设置：`debug` / `info` / `warn` / `error`。

## 故障排查

### 常见问题

**Q: Webhook 返回 401 Unauthorized**
- 检查 `webhook_secret` 是否与 Gitea Webhook 配置一致
- 检查 Gitea Webhook 的 Secret 字段是否填写

**Q: Agent 任务一直处于 pending 状态**
- 检查 LLM API Key 是否正确
- 检查网络是否能访问 LLM API
- 查看日志中的错误信息

**Q: Agent 执行超时**
- 调整 `agents.defaults.timeout`（单次任务超时，如 analyze/review）
- 调整 `agents.loop.total_timeout`（多轮任务总超时）
- 调整 `agents.defaults.max_output_tokens` / `max_input_tokens`（LLM 预算）
- 检查 LLM API 响应速度

**Q: 创建 Agent 失败**
- 检查 `gitea.admin_token` 是否有管理员权限
- 检查 Gitea API 是否可访问

**Q: 前端页面空白**
- 确认构建时前端已打包（`go:embed` 需要 `web/dist` 目录）
- 检查浏览器控制台是否有错误

### 健康检查

```bash
curl http://localhost:8080/health
# {"status":"ok","version":"0.1.0"}
```

### 数据库检查

```bash
sqlite3 data/gateway.db

# 查看 Agent 列表
SELECT id, name, gitea_username, status FROM agents;

# 查看任务状态分布
SELECT status, COUNT(*) FROM tasks GROUP BY status;

# 查看最近的任务
SELECT id, task_type, status, created_at FROM tasks ORDER BY id DESC LIMIT 10;
```
