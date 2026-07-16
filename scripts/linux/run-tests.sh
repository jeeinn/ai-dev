#!/bin/bash
# Run integration tests with CGO enabled for SQLite
# Usage: ./scripts/linux/run-tests.sh

set -e

echo "=== Running Unit Tests ==="
CGO_ENABLED=1 go test ./internal/... -v -count=1 -timeout 30s

echo ""
echo "=== Running Integration Tests ==="
CGO_ENABLED=1 go test ./internal/dispatcher/ -v -count=1 -timeout 60s -run "Test"

echo ""
echo "=== Build Verification ==="
go build -o /dev/null ./...

echo ""
echo "All tests passed!"
