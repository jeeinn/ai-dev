# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Gitea Agent Gateway — a Go service that receives Gitea webhooks, routes events to AI agents, and writes results back as Gitea comments or PRs. Supports multi-turn tool-use agent loops for code generation tasks.

## Build & Run

```bash
# Build
go build -o gateway .

# Run
./gateway -config config.yaml

# Format & Vet
go fmt ./...
go vet ./...
```

## Testing

```bash
# All tests
go test ./... -count=1

# Unit tests only
go test ./internal/... -v -count=1

# Integration tests only (require TestEnv)
go test ./tests/integration/ -v -count=1

# Single package
go test ./internal/sandbox/ -v -count=1

# Single test function
go test ./tests/integration/ -v -run TestWebhookIssueAssigned

# Coverage
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out
```

Test framework: `testify` (assert + require). All tests use `require` for preconditions, `assert` for independent assertions.

## Architecture

### Request Flow

```
Gitea Webhook → webhook.Handler (verify sig, dedup)
  → Dispatcher.HandleEvent
    → Router.Match (find agent via route rules)
    → TaskQueue.Enqueue (persist to SQLite)
    → Executor.execute (concurrent workers)
      → RunnerFactory.GetRunner (select runner by task_type)
        → Runner.Run (LLM call or agent loop)
      → writeBackToGitea (post comment/PR)
```

### Key Packages

| Package | Responsibility |
|---------|---------------|
| `internal/webhook` | HTTP handler, signature verification, event parsing, deduplication |
| `internal/dispatcher` | Router (event→agent matching), TaskQueue (SQLite-backed), Executor (concurrent workers) |
| `internal/agents` | Runner interface + implementations (AnalyzeRunner, ReviewRunner, InteractionRunner, DevRunner, BugfixRunner), RunnerFactory |
| `internal/agent` | Tool-use agent loop: ToolRegistry, AgentLoop (multi-turn LLM conversation with tool calls) |
| `internal/llm` | Provider interface, Registry, OpenAI-compatible client (`openai.go`), Anthropic client (`anthropic.go`) |
| `internal/store` | SQLite store (WAL mode, single-writer), migrations, CRUD for agents/routes/tasks |
| `internal/sandbox` | Workspace isolation (directory-based, not Docker), command whitelisting, Git operations, audit logging |
| `internal/gitea` | Gitea API client (issues, PRs, repos, comments) |
| `internal/config` | YAML config loading with `${ENV_VAR}` expansion |
| `internal/api` | Management REST API (agent CRUD, route CRUD, task listing) with Bearer auth |

### Runner Strategy Pattern

Task types map to runners via `RunnerFactory.GetRunner`:
- `analyze_issue` / `trigger` → **AnalyzeRunner** — single LLM call, returns comment
- `review_pr` → **ReviewRunner** — fetches PR diff, LLM review, returns comment
- `reply_comment` → **InteractionRunner** — fetches comment history, LLM reply
- `solve_issue` → **DevRunner** — sandbox + agent loop + git clone/branch/commit/push + create PR
- `fix_bug` → **BugfixRunner** — same as DevRunner with different system prompt

### Agent Loop (`internal/agent`)

`AgentLoop.Run` implements a multi-turn tool-use loop:
1. Send messages + tool definitions to LLM
2. If LLM returns tool calls → execute them via `ToolRegistry.ExecuteTool` → append results → repeat
3. If LLM returns no tool calls → done (max 20 iterations)

Default tools: `read_file`, `write_file`, `list_files`, `search_code`, `run_command`, `apply_diff` — all operate within the sandbox workspace.

### Sandbox (`internal/sandbox`)

Not Docker-based. Uses directory isolation with:
- Per-task workspace under `BaseDir/task_<id>`
- Command whitelist (git, go, python, node, etc.)
- Output size limits (1MB default)
- Command timeout (5min default)
- Audit logging of all commands to DB

### Database

SQLite with WAL mode, single-writer connection pool. Tables: `agents`, `routes`, `tasks`, `prompt_history`, `processed_deliveries`, `operation_logs`. Migrations run automatically on startup in `store.Open`.

### Config

YAML file with `${VAR}` / `${VAR:-default}` env expansion. Copy `config.example.yaml` to `config.yaml`. Key sections: `server`, `gitea`, `workspace`, `dispatcher`, `llm`, `agents`, `api`.

### LLM Providers

Register providers via `llm.Registry`. Built-in: `openai.go` (OpenAI-compatible, works with DeepSeek/Ollama), `anthropic.go` (Claude). Tool use follows OpenAI function-calling format.

## Test Architecture

- **Unit tests**: Co-located in `internal/*/` packages. Test single functions/methods without external deps.
- **Integration tests**: In `tests/integration/`. Use `TestEnv` which provides in-memory SQLite, mock Gitea server, mock LLM provider, and full HTTP test server.
- Decision rule: if test needs TestEnv (DB/HTTP/Mock) → integration test; otherwise → unit test.
