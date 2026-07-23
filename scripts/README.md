# Scripts layout

| Path | Contents |
|------|----------|
| [`windows/`](windows/) | Local Gitea E2E PowerShell drivers (`.ps1`) |
| [`linux/`](linux/) | Bash helpers (`run-tests.sh`, `test-webhook.sh`) |
| [`common/`](common/) | Shared Go one-offs (`e2e-mock-mcp.go`, `setup-test.go`) |
| [`TESTING.md`](TESTING.md) | **统一测试指南**（单元 / Mock 集成 / **本机 E2E 前置条件**） |

详细环境准备、凭据、配置、场景矩阵与已知坑：**请读 [TESTING.md § 本机真实 Gitea E2E](TESTING.md#本机真实-gitea-e2e前置与复现)**。  
最近全量报告：[docs/archived/20260716-e2e-test-report.md](../docs/archived/20260716-e2e-test-report.md)。

## Local E2E（Windows，速查）

前置：Gitea `:3000`、Gateway `:8080`、可选 OpenCode `:4096` / Mock MCP `:18080`；凭据在 `data/e2e-env.local`（gitignore）。

```powershell
# 仓库根目录
go run scripts/common/e2e-mock-mcp.go          # 另开终端；测 MCP 时需要
opencode serve --port 4096                     # 另开终端；测 Path A 时需要
# 加载 data/e2e-env.local 后：
.\gateway.exe -config config.yaml

powershell -NoProfile -File scripts/windows/e2e-setup-gitea.ps1
powershell -NoProfile -File scripts/windows/e2e-create-agents.ps1
powershell -NoProfile -File scripts/windows/e2e-run-scenarios.ps1
```
