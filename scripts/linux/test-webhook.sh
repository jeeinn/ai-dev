#!/bin/bash
# Test webhook endpoint with simulated Gitea event
# Usage: ./scripts/linux/test-webhook.sh [port]

PORT=${1:-8080}
URL="http://localhost:${PORT}/webhook/gitea"

echo "Testing webhook at ${URL}"

# Simulate issue assignment event
curl -X POST "${URL}" \
  -H "Content-Type: application/json" \
  -H "X-Gitea-Event: issues" \
  -H "X-Gitea-Delivery: test-$(date +%s)" \
  -d '{
    "action": "assigned",
    "repository": {
      "id": 1,
      "name": "test-repo",
      "full_name": "admin/test-repo",
      "owner": {"id": 1, "login": "admin"},
      "clone_url": "http://localhost:3000/admin/test-repo.git",
      "default_branch": "main"
    },
    "issue": {
      "id": 100,
      "number": 1,
      "title": "Test issue for AI agent",
      "body": "Please analyze this issue and provide suggestions.",
      "state": "open",
      "user": {"id": 1, "login": "admin"},
      "assignees": [{"id": 2, "login": "ai-agent"}],
      "labels": [{"id": 1, "name": "need-analysis"}]
    },
    "sender": {"id": 1, "login": "admin"}
  }'

echo ""
echo "---"

# Simulate PR event
curl -X POST "${URL}" \
  -H "Content-Type: application/json" \
  -H "X-Gitea-Event: pull_request" \
  -H "X-Gitea-Delivery: test-pr-$(date +%s)" \
  -d '{
    "action": "opened",
    "repository": {
      "id": 1,
      "name": "test-repo",
      "full_name": "admin/test-repo",
      "owner": {"id": 1, "login": "admin"},
      "clone_url": "http://localhost:3000/admin/test-repo.git",
      "default_branch": "main"
    },
    "pull_request": {
      "id": 200,
      "number": 10,
      "title": "Add new feature",
      "body": "This PR adds a new feature.",
      "state": "open",
      "user": {"id": 1, "login": "admin"},
      "head": {"ref": "feature-branch", "repo": {"full_name": "admin/test-repo"}},
      "base": {"ref": "main", "repo": {"full_name": "admin/test-repo"}}
    },
    "sender": {"id": 1, "login": "admin"}
  }'

echo ""
echo "---"
echo "Check server logs for task execution details"
