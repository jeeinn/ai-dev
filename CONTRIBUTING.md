# 贡献指南

感谢关注 [Gitea Agent Gateway](https://github.com/jeeinn/ai-dev)。本文说明如何在本仓库参与开发与提交变更。

## 仓库地址

- 官方仓库：https://github.com/jeeinn/ai-dev
- Clone：

```bash
git clone https://github.com/jeeinn/ai-dev.git
cd ai-dev
```

## 开发环境

- Go 1.26+（以 `go.mod` 为准）
- Node.js 18+（仅构建 Web UI 时需要）
- 可选：本机 Gitea、LLM API Key（完整联调时需要）

## 构建

```bash
# 前端（嵌入二进制前需先构建）
cd web && npm install && npm run build && cd ..

# 后端
go build -o gateway .

# 格式与静态检查
go fmt ./...
go vet ./...
```

本地配置：

```bash
cp config.example.yaml config.yaml
# 可选：cp .env.example .env 后填入密钥（勿提交 config.yaml / .env）
./gateway -config config.yaml
```

默认 Web UI：`http://localhost:8080`（`admin` / `admin123`）。首次登录后请立即修改密码。

## 测试

```bash
# 全部测试
go test ./... -count=1

# 仅单元测试
go test ./internal/... -v -count=1

# 仅集成测试（Mock Gitea / Mock LLM，无需真实实例）
go test ./tests/integration/ -v -count=1
```

测试约定：`testify`；前置条件用 `require`，独立断言用 `assert`。更多说明见 [scripts/TESTING.md](scripts/TESTING.md)。

提交前请至少保证：

- `go vet ./...` 通过
- `go test ./... -count=1` 通过

CI 会在指向 `main` / `master` 的 PR 上自动跑上述检查。

## 分支与 Pull Request

1. 从最新的 `main`（或项目默认主干）拉出功能分支，例如 `feat/...`、`fix/...`、`docs/...`。
2. 保持提交粒度清晰：一个提交对应一个可理解的意图；commit message 说明「为什么」而非堆砌文件名。
3. 开 PR 到官方仓库：https://github.com/jeeinn/ai-dev
4. PR 描述请包含：变更摘要、如何自测、相关 Issue（如有）。
5. 避免提交密钥、`config.yaml`、`.env`、本地 `data/` 等敏感或生成物。

## 行为准则（简要）

- 讨论与评审保持建设性；聚焦技术事实。
- **不要**在公开 Issue / PR 中粘贴可利用的漏洞细节或 exploit；安全问题请按 [SECURITY.md](SECURITY.md) 私下报告。

## 文档与清单

- 架构：[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- 部署：[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)
- 开源准备（已完成归档）：[docs/archived/20260716-OPEN-SOURCE-CHECKLIST.md](docs/archived/20260716-OPEN-SOURCE-CHECKLIST.md)
- 任务清单：[docs/TASKS.md](docs/TASKS.md)

如有疑问，可在 GitHub Issues 提问（非安全敏感内容）。
