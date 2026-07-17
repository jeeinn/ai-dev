# Linux scripts

| Script | Purpose |
|--------|---------|
| `run-tests.sh` | Run unit + selected integration tests with CGO |
| `test-webhook.sh` | Smoke-post a sample Gitea webhook payload |
| `e2e-smoke.sh` | `go vet` + `go test ./...`；说明如何跑完整 Assign E2E |

**完整真实 Gitea E2E（E0–E13）以 Windows PowerShell 为主**（`../windows/e2e-run-scenarios.ps1`）。  
在 Linux 上可安装 [PowerShell](https://learn.microsoft.com/powershell/)（`pwsh`）后直接运行同一脚本；或先跑 `./e2e-smoke.sh` 覆盖 Mock 路径。

Cross-platform Go helpers: [`../common/`](../common/).  
测试说明：[`../TESTING.md`](../TESTING.md)。
