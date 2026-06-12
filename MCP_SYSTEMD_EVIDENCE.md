# MCP Systemd Service Evidence

## Service Status Table

| Service       | Port | Backend               | Status    | Enabled | PID  | RSS    | CPU% |
|---------------|------|-----------------------|-----------|---------|------|--------|------|
| filesystem    | 4110 | npx server-filesystem | active    | enabled | 158017 | 51MB | 0.7% |
| git           | 4111 | go adapter `--adapter git` | active | enabled | 157110 | 51MB | 1.5% |
| fetch         | 4112 | go adapter `--adapter fetch` | active | enabled | 157276 | 51MB | 1.5% |
| memory        | 4113 | npx server-memory     | active    | enabled | 157277 | 51MB | 1.5% |

**OpenCode MCP List** — all 8 servers connected:
```
supabase  ✓ connected  https://mcp.supabase.com/...
chromadb  ✓ connected  http://localhost:4114
filesystem ✓ connected  http://localhost:4110
git       ✓ connected  http://localhost:4111
github    ✓ connected  http://localhost:4115
fetch     ✓ connected  http://localhost:4112
memory    ✓ connected  http://localhost:4113
context7  ✓ connected  https://mcp.context7.com/mcp
```

## Unit Files

All located in `~/.config/systemd/user/`:
- `mcp-filesystem.service` — `npx -y @modelcontextprotocol/server-filesystem /home/asem/workspace`
- `mcp-git.service` — `go run ./runtime/mcp/v2/cmd/ --adapter git`
- `mcp-fetch.service` — `go run ./runtime/mcp/v2/cmd/ --adapter fetch`
- `mcp-memory.service` — `npx -y @modelcontextprotocol/server-memory`

Each uses `Restart=always`, `RestartSec=2`, `Environment=MCP_PORT=N` pointing to proxy at `.opencode/mcp-sse-proxy.mjs`.

## Startup Sequence

All 4 services start independently. Example startup (filesystem):
```
[sub] Secure MCP Filesystem Server running on stdio
initialized with 14 tools
MCP proxy ready on port 4110:
```

Full startup from proxy start to initialize complete: ~2 seconds (limited by npx require time).

## Latency

Direct POST initialize handshake latency (all sub-millisecond processing, dominated by network round-trip):

| Port | Latency |
|------|---------|
| 4110 | 8ms     |
| 4111 | 10ms    |
| 4112 | 8ms     |
| 4113 | 10ms    |

## Failure Cases

### Service Stopped (mcp-git stopped)
```
git   ✗ failed    SSE error: Unable to connect. Is the computer able to access the url?
```
OpenCode gracefully shows "failed" with descriptive error. Other servers unaffected.

### Service Restored
```
git   ✓ connected  http://localhost:4111
```
No restart of OpenCode needed — `mcp list` reflects current state on each invocation.

### Kill -9 Auto-Recovery
```
kill -9 <filesystem-pid>
→ systemd detects exit, restarts within 2s (RestartSec=2)
→ New PID, initialization completes in ~4s total
→ Health endpoint returns tools=14
→ Service status: active
```

## Resource Usage

| Metric        | Value                             |
|---------------|-----------------------------------|
| Per-process   | ~51MB RSS, ~610MB VSZ, 0.3% mem  |
| Total (4)     | ~204MB RSS, ~2.4GB VSZ            |
| CPU           | ~1.5% each (idle), bursts on req  |
| Startup time  | ~2s (with npx), ~0.5s (go)        |

## Dependency Chain

```
mcp-filesystem.service (no deps, root)
  └─ BindsTo + Wants ─→ mcp-git.service (Wants: filesystem + fetch)
                          └─ BindsTo + Wants ─→ mcp-fetch.service (Wants: git + memory)
                                                  └─ BindsTo ─→ mcp-memory.service (Wants: none, leaf)
```

- `BindsTo=` — fail-close: stopping parent stops all dependents
- `Wants=` — start cascade: starting parent starts dependents
- `systemctl restart mcp-git.service` → auto-starts fetch → auto-starts memory
- All `WantedBy=default.target` with `Restart=always`

## Proxy Architecture

Single zero-dependency Node.js script (`mcp-sse-proxy.mjs`) using only built-in `http` module. Handles:
- **Direct POST /**: JSON-RPC (OpenCode primary protocol)
- **SSE GET /**: `Accept: text/event-stream` mode (compatibility)
- **GET /health**: Status + tool count
- Persistent subprocess with auto-restart on crash

Key fix: JSON-RPC notifications (no `id`) are forwarded without waiting for a response — prevents hang on `notifications/initialized`.

## Terminal Independence

```
loginctl enable-linger asem  ← confirmed
```
Services start at boot without user login. Linger is enabled.

## Key Discovery

- `@modelcontextprotocol/server-git` and `@modelcontextprotocol/server-fetch` do NOT exist on npm
- Go adapters in `./runtime/mcp/v2/cmd/ --adapter <name>` provide identical MCP stdio interface
- User-level systemd does not accept `User=` directive (exit code 216/GROUP error)
- Proper notification handling required for OpenCode type:remote to work
