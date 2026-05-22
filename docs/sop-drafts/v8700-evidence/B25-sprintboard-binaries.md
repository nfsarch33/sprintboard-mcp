# Sprintboard B25 binary rebuild

Sprint: v8700 Block 3 (overnight 2026-05-23)
Branch: `feat/v8700-ticket-comments`
Worktree: `/tmp/v8700-sprintboard-wt`
Base: `origin/main` + commit `fac893c` (B23 ticket comments)

## Build matrix

`CGO_ENABLED=0 -trimpath -ldflags="-s -w -X main.version=v8700"`

| target | binary | size |
|---|---|---|
| darwin/arm64 | `sprintboard-mcp` | 9.3M |
| darwin/arm64 | `sprintboard-api` | 9.7M |
| windows/amd64 | `sprintboard-mcp.exe` | 9.8M |
| windows/amd64 | `sprintboard-api.exe` | 10M |

Outputs (under `~/runs/v8700/`):

```
25735dee6b6bec0160592f5a769efafe2568109b8107312a638f6f985fcb38e3  sprintboard-darwin-arm64/sprintboard-api
5b5e9a1e25353516645670fa5eee7b08c5390f1e17074af33351ab66fefe5535  sprintboard-darwin-arm64/sprintboard-mcp
c5543a230ef3bcdbc91787cd39bb8c8d0c67e0a2b66ba839a155faaee795b06b  sprintboard-windows-amd64/sprintboard-api.exe
517a2478a35e116b5f04e93cdbed36323dc10daa44f646c5a197285f75ced84e  sprintboard-windows-amd64/sprintboard-mcp.exe
```

Manifest: `~/runs/v8700/sprintboard-binaries.sha256`.

Toolchain: host `go1.25.6 darwin/arm64`, sprintboard-mcp go.mod is on
go1.23 so no `GOTOOLCHAIN=auto` fetch needed.

## Tool count after B23

`tools/list` over the new binary returns 30 tools = 23 base + 5 DAG
(B19) + 2 ticket-comment (B23: `ticket_comment_add`,
`ticket_comment_list`). Mirror REST routes:

* `POST /api/v1/tickets/{id}/comments`
* `GET  /api/v1/tickets/{id}/comments`
