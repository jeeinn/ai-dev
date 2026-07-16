# Scripts layout

| Path | Contents |
|------|----------|
| [`windows/`](windows/) | Local Gitea E2E PowerShell drivers (`.ps1`) |
| [`linux/`](linux/) | Bash helpers (`run-tests.sh`, `test-webhook.sh`) |
| [`common/`](common/) | Shared Go one-offs (`e2e-mock-mcp.go`, `setup-test.go`) |
| [`TESTING.md`](TESTING.md) | Unit / integration test guide |

## Local E2E (Windows)

From repo root (credentials in `data/e2e-env.local`, gitignored):

```powershell
# Mock MCP (optional for MCP scenarios)
go run scripts/common/e2e-mock-mcp.go

powershell -NoProfile -File scripts/windows/e2e-setup-gitea.ps1
powershell -NoProfile -File scripts/windows/e2e-create-agents.ps1
powershell -NoProfile -File scripts/windows/e2e-run-scenarios.ps1
```
