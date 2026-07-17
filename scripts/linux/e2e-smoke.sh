#!/usr/bin/env bash
# scripts/linux/e2e-smoke.sh — lightweight Linux helper around Mock tests + optional real Gitea notes.
# Full Assign E2E scenarios (E0–E13) currently live in scripts/windows/*.ps1.
# This script covers what Linux CI / Unix hosts can run without Windows PowerShell.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "==> go vet"
go vet ./...

echo "==> go test (unit + mock integration)"
go test ./... -count=1

echo ""
echo "Mock 路径已通过。"
echo "真实 Gitea Assign E2E（含 E13 Merge→done）请使用："
echo "  scripts/windows/e2e-run-scenarios.ps1"
echo "或在 Linux 上用 pwsh 运行同一脚本（需本机 Gitea + 凭据，见 scripts/TESTING.md）。"
echo "详见 docs/20260717-v2-merge-signoff.md"
