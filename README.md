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

## Tools (23)

| Tool | Description |
|---|---|
| `sprint_create` | Create a new sprint |
| `sprint_list` | List all sprints |
| `sprint_status` | Sprint summary with ticket counts |
| `sprint_close` | Close a sprint |
| `ticket_create` | Create a ticket |
| `ticket_list` | List/filter tickets |
| `ticket_update` | Update ticket status |
| `ticket_assign` | Assign ticket to agent |
| `ticket_search` | Semantic search tickets |
| `sprint_search` | Semantic search sprints |
| `task_claim` | Atomically claim a ticket |
| `task_complete` | Mark ticket done with evidence |
| `task_recommend` | AI-powered task recommendation |
| `agent_register` | Register agent with capabilities |
| `agent_heartbeat` | Keep agent registration alive |
| `agent_list` | List registered agents |
| `handoff_create` | Create handoff record |
| `handoff_list` | List handoffs |
| `handoff_publish` | Publish cross-agent handoff |
| `handoff_subscribe` | Check for pending handoffs |
| `sprint_distribute` | Auto-distribute tickets to agents |
| `sprint_kickoff_prompt` | Generate agent kickoff prompt |
| `sprint_handoff_template` | Generate handoff document |

## Architecture

- **SQLite WAL** backend for concurrent multi-agent access
- **TF-IDF** embedder for semantic search (no external dependencies)
- **Agentrace** NDJSON logging for all MCP tool invocations
- **stdio** transport (compatible with Cursor, Claude Code, Codex)

## License

Apache-2.0
