# SprintBoard MCP -- Agent Guidelines

- Repo: `https://github.com/nfsarch33/sprintboard-mcp`
- **Purpose**: SQLite-backed multi-agent sprint board with MCP + REST interfaces.
  Atomic ticket claiming, DAG dependencies, semantic search, burndown metrics.
- **Binary**: `~/runs/sprintboard-mcp` (MCP stdio), `sprintboard-api` (REST :9400)

## Build & Test

```bash
go test -race -count=1 ./...    # 190+ tests
go vet ./...                     # static analysis
go build -o sprintboard-mcp ./cmd/sprintboard-mcp/
go build -o sprintboard-api ./cmd/sprintboard-api/
```

CI runs on GitLab (`.gitlab-ci.yml`): vet, race tests, build.

## Architecture

- `cmd/sprintboard-mcp/` -- MCP stdio server (33 tools)
- `cmd/sprintboard-api/` -- REST API server (:9400)
- `cmd/sprintboard-worker/` -- Temporal worker
- `internal/sprintboard/` -- SQLite store, claiming, agents, DAG, search, burndown
- `internal/api/` -- HTTP handlers
- `internal/temporal/` -- workflows + activities
- `internal/mcptelemetry/` -- Agentrace NDJSON logging

## MCP Tools (33)

Sprint: `sprint_create`, `sprint_list`, `sprint_status`, `sprint_close`,
  `sprint_history`, `sprint_metrics`
Tickets: `ticket_create`, `ticket_list`, `ticket_update`, `ticket_assign`,
  `ticket_search`, `ticket_search_filter`
Search: `sprint_search`
Agents: `agent_register`, `agent_heartbeat`, `agent_list`, `task_claim`,
  `task_complete`, `task_recommend`, `sprint_distribute`
Handoffs: `handoff_create`, `handoff_list`, `handoff_publish`,
  `handoff_subscribe`, `sprint_kickoff_prompt`, `sprint_handoff_template`
DAG: `ticket_depend_add`, `ticket_depend_remove`, `ticket_blocked_by`,
  `ticket_ready_list`, `sprint_topo_sort`
Comments: `ticket_comment_add`, `ticket_comment_list`

## Coding Conventions

- Go, strict typing. `go vet` + `go test -race` before commit.
- No secrets in committed files.
- Conventional commits: `type(scope): message`.
- No fleet hostnames, personal paths, or internal IPs (PUBLIC repo).

## Identity

- Personal repos: `nfsarch33` / SSH `~/.ssh/agtc`
