# v2 Merge → done Sign-off（S1）

> 日期：2026-07-17  
> 对照：[archived/20260709-v2-gitea-integration-checklist.md](archived/20260709-v2-gitea-integration-checklist.md) §2.4  
> 开源清单：[OPEN-SOURCE-CHECKLIST.md](OPEN-SOURCE-CHECKLIST.md) S1

## 验收结论

| 路径 | 覆盖 | 状态 |
|------|------|------|
| Mock 集成 | `TestWebhookPRClosedMerged`：`merged=true` → `stage=done` | **PASS**（`go test ./tests/integration/ -run TestWebhookPRClosedMerged`） |
| Mock 集成 | `TestWebhookPRClosedNotMerged`：未合并关闭不转 `done` | **PASS** |
| 真实 Gitea E2E | 场景 **E13**：merge 开放 PR → `GET /api/workflow-context` `stage=done` | 脚本已加；需本机 Gitea 跑 `-Only E13`（建议先跑 E5/E6/E7 留 open PR） |

§2.4「Merge → done + session archived」在 **Mock 路径已签收**；真实环境通过 **E13** 复验。Session archive 由 `SessionLifecycle.OnPRClosed(merged=true)` 触发，与 lifecycle 单测 / 集成测试一致。

## 复现

```bash
# Mock（CI / 任意平台）
go test ./tests/integration/ -count=1 -run 'TestWebhookPRClosed'

# 真实 Gitea（Windows PowerShell，或 Linux + pwsh）
.\scripts\windows\e2e-run-scenarios.ps1 -Only E5,E13
# 或仅 merge 已有 open PR：
.\scripts\windows\e2e-run-scenarios.ps1 -Only E13
```

## Sign-off

- [x] Mock：merged PR → `done`
- [x] E2E 脚本：E13 已纳入默认矩阵
- [ ] （可选）本机跑通 E13 并更新 [20260716-e2e-test-report.md](20260716-e2e-test-report.md) 附录
