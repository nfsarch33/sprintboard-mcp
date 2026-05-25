# sprintboard-mcp

Multi-agent sprint board MCP (Model Context Protocol) server for AI coding agent coordination.

## Features

- **Sprint lifecycle**: create, list, status, close sprints
- **Ticket management**: create, list, update, assign, search tickets
- **Atomic claiming**: prevent double-assignment with SQLite WAL + busy_timeout
- **Agent registry**: register, heartbeat, auto-expire agents
- **Handoff protocol**: publish/subscribe cross-agent handoffs
- **Agentrace**: NDJSON telemetry for all tool calls
- **Semantic search**: TF-IDF vector search across tickets and sprints

## Quick Start

```bash
go build -o sprintboard-mcp ./cmd/sprintboard-mcp/
./sprintboard-mcp  # stdio MCP server
```

## MCP Configuration

Add to `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "sprintboard": {
      "command": "<path-to>/sprintboard-mcp",
      "args": [],
      "env": {
        "AGENTRACE_ENABLED": "true",
        "AGENTRACE_LOG_PATH": "<home>/logs/runx/agentrace-mcp.ndjson"
      }
    }
  }
}
```

## Tools (33)

### Sprint Management

| Tool | Description |
|---|---|
| `sprint_create` | Create a new sprint |
| `sprint_list` | List all sprints |
| `sprint_status` | Sprint summary with ticket counts |
| `sprint_close` | Close a sprint |
| `sprint_history` | List recent sprints (ordered by creation) |
| `sprint_metrics` | Burndown, SLA, and velocity metrics |
| `sprint_search` | Semantic search across sprints |
| `sprint_distribute` | Auto-distribute tickets to agents |
| `sprint_kickoff_prompt` | Generate agent kickoff prompt |
| `sprint_handoff_template` | Generate session handoff document |
| `sprint_topo_sort` | Topological order respecting DAG dependencies |

### Ticket Management

| Tool | Description |
|---|---|
| `ticket_create` | Create a ticket with optional priority and acceptance criteria |
| `ticket_list` | List/filter tickets by sprint |
| `ticket_update` | Update ticket status, priority, or description |
| `ticket_assign` | Assign ticket to an agent |
| `ticket_search` | Semantic search across all tickets |
| `ticket_search_filter` | Filter tickets by sprint, status, or agent |
| `ticket_comment_add` | Append an audit comment to a ticket |
| `ticket_comment_list` | List comments on a ticket |

### DAG Dependencies

| Tool | Description |
|---|---|
| `ticket_depend_add` | Add a dependency (ticket A blocks ticket B) |
| `ticket_depend_remove` | Remove a dependency |
| `ticket_blocked_by` | List tickets blocking a given ticket |
| `ticket_ready_list` | List tickets with all dependencies satisfied |

### Agent Coordination

| Tool | Description |
|---|---|
| `task_claim` | Atomically claim a ticket (conflict-safe) |
| `task_complete` | Mark ticket done with evidence string |
| `task_recommend` | Suggest next ticket based on agent capabilities |
| `agent_register` | Register agent with capabilities |
| `agent_heartbeat` | Keep agent registration alive (30 min expiry) |
| `agent_list` | List registered agents |

### Handoffs

| Tool | Description |
|---|---|
| `handoff_create` | Create a handoff record |
| `handoff_list` | List handoffs for a sprint |
| `handoff_publish` | Publish cross-agent handoff with summary |
| `handoff_subscribe` | Check for pending handoffs targeting an agent |

## Architecture

- **SQLite WAL** backend for concurrent multi-agent access
- **TF-IDF** embedder for semantic search (no external dependencies)
- **Agentrace** NDJSON logging for all MCP tool invocations
- **stdio** transport (compatible with Cursor, Claude Code, Codex)

## License

MIT
