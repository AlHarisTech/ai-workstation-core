# Deployment Guide — MCP Runtime v3.1.1-stable

## Requirements

### Software

| Dependency | Version | Required For |
|-----------|---------|-------------|
| Go | 1.22+ | Building runtime components (git, fetch adapters) |
| Node.js | 18+ (tested 24.16.0) | Running MCP SSE proxy, ChromaDB server, GitHub server |
| npx | Included with Node.js | Running stdio-based MCP servers (filesystem, memory) |
| systemd | 255+ (user-mode) | Service management, auto-restart, BindsTo cascade |
| loginctl | systemd component | Enabling linger for boot-time service start |
| git | 2.x | Git MCP operations |
| curl | Any | Health checks, HTTP MCP operations |
| opencode | 0.21+ | MCP server list, runtime configuration |

### Hardware

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 1 core | 2+ cores |
| RAM | 1 GB | 2 GB |
| Disk | 5 GB | 20 GB (workspace + knowledge storage) |

### Network

- Local ports 4110–4113 required (core MCPs); ports 4114–4115 required only if ChromaDB/GitHub are deployed
- Outbound HTTPS to: `api.github.com`, `api.context7.com`, `api.trychroma.com`, Supabase endpoint
- No inbound ports required (all MCPs are local HTTP servers)

### Environment Variables

| Variable | Required For | Default |
|----------|-------------|---------|
| `GITHUB_TOKEN` | GitHub authenticated operations | (not set — operations limited to unauthenticated) |
| `CONTEXT7_API_KEY` | Context7 context retrieval | (not set — service unreachable without) |
| `CHROMA_API_KEY` | ChromaDB knowledge retrieval | (set in systemd unit) |
| `CHROMA_TENANT` | ChromaDB tenant ID | (set in systemd unit) |
| `CHROMA_DATABASE` | ChromaDB database name | (set in systemd unit) |

---

## Installation

### Build from Source

```bash
# 1. Clone repository
git clone https://github.com/AlHarisTech/ai-workstation-core.git
cd ai-workstation-core

# 2. Build Go adapter binary
go build -o bin/mcp-server ./runtime/mcp/v2/cmd/

# 3. Verify build
ls -l bin/mcp-server
```

**Note:** There is a single Go binary (`mcp-server`). The adapter type (git, fetch, etc.) is selected at runtime via the `--adapter` flag. All systemd units use `go run` for development flexibility; pre-built binaries are optional for production hardening.

### Systemd Service Installation

Each MCP server runs as a user-level systemd service behind an SSE-to-stdio proxy.

#### SSE Proxy Setup

The SSE proxy (`mcp-sse-proxy.mjs`) is a Node.js script that bridges HTTP (SSE) transport to stdio-based MCP servers. It is required for all local MCP services.

```bash
# Verify proxy script exists
ls -la .opencode/mcp-sse-proxy.mjs
# If missing, the proxy is included in the repository
```

#### Service Unit Files

Create unit files in `~/.config/systemd/user/`. Minimal template:

```ini
[Unit]
Description=MCP <Name> Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/path/to/workspace
Environment="NODE_ENV=production"
Environment="MCP_PORT=<port>"
Environment="MCP_CMD=<command>"
Environment="MCP_ARGS=<json-array-of-args>"
ExecStart=/usr/bin/node /path/to/.opencode/mcp-sse-proxy.mjs
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
```

### Service Dependency Cascade

The 4 core MCPs form a `BindsTo` failsafe chain. This is an **architectural tradeoff** (fail-fast, fail-closed):

```
mcp-filesystem (port 4110, root)
    ↓ BindsTo: if filesystem stops → git stops
mcp-git (port 4111, Wants=mcp-filesystem mcp-fetch)
    ↓ BindsTo: if git stops → fetch stops
mcp-fetch (port 4112, Wants=mcp-git mcp-memory)
    ↓ BindsTo: if fetch stops → memory stops
mcp-memory (port 4113, leaf)
```

**Behavior:**
- `BindsTo=` triggers a **stop cascade** — if a parent fails, all dependents shut down (fail-close)
- `Wants=` triggers a **start cascade** — starting git auto-starts filesystem and fetch; starting fetch auto-starts memory
- Start cascade root: `systemctl --user start mcp-git.service` (starts all 4 via Wants)
- ChromaDB and GitHub are standalone (no BindsTo) — they do not affect core MCP stability

### Enable Linger

```bash
loginctl enable-linger asem
```

This ensures services start at boot without requiring an active user session.

---

## MCP Configuration

### Filesystem

**Type:** stdio-to-HTTP via SSE proxy
**Binary:** `npx -y @modelcontextprotocol/server-filesystem /path/to/workspace`
**Port:** 4110
**Role:** File read/write operations, workspace root restricted

```ini
[Unit]
Description=MCP Filesystem Server
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/path/to/workspace
Environment="NODE_ENV=production"
Environment="MCP_PORT=4110"
Environment="MCP_CMD=npx"
Environment="MCP_ARGS=[\"-y\",\"@modelcontextprotocol/server-filesystem\",\"/path/to/workspace\"]"
ExecStart=/usr/bin/node /path/to/workspace/.opencode/mcp-sse-proxy.mjs
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
```

### Git

**Type:** Go adapter via SSE proxy
**Binary:** `go run ./runtime/mcp/v2/cmd/ --adapter git`
**Port:** 4111
**Role:** Git status, diff, log operations

```ini
[Unit]
Description=MCP Git Server
After=network-online.target mcp-filesystem.service
BindsTo=mcp-filesystem.service
Wants=mcp-filesystem.service mcp-fetch.service

[Service]
Type=simple
WorkingDirectory=/path/to/workspace
Environment="NODE_ENV=production"
Environment="MCP_PORT=4111"
Environment="MCP_CMD=go"
Environment="MCP_ARGS=[\"run\",\"./runtime/mcp/v2/cmd/\",\"--adapter\",\"git\"]"
ExecStart=/usr/bin/node /path/to/workspace/.opencode/mcp-sse-proxy.mjs
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
```

### Fetch

**Type:** Go adapter via SSE proxy
**Binary:** `go run ./runtime/mcp/v2/cmd/ --adapter fetch`
**Port:** 4112
**Role:** HTTP fetch with 30s timeout

```ini
[Unit]
Description=MCP Fetch Server
After=network-online.target mcp-git.service
BindsTo=mcp-git.service
Wants=mcp-git.service mcp-memory.service

[Service]
Type=simple
WorkingDirectory=/path/to/workspace
Environment="NODE_ENV=production"
Environment="MCP_PORT=4112"
Environment="MCP_CMD=go"
Environment="MCP_ARGS=[\"run\",\"./runtime/mcp/v2/cmd/\",\"--adapter\",\"fetch\"]"
ExecStart=/usr/bin/node /path/to/workspace/.opencode/mcp-sse-proxy.mjs
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
```

### Memory

**Type:** stdio-to-HTTP via SSE proxy
**Binary:** `npx -y @modelcontextprotocol/server-memory`
**Port:** 4113
**Role:** In-memory knowledge graph (short-lived cache)
**⚠️ Critical:** All data is **lost on service restart** (no persistence). The knowledge graph (`memory_*`) and adaptive routing context are volatile. For persistent knowledge, use ChromaDB instead.

```ini
[Unit]
Description=MCP Memory Server
After=network-online.target mcp-fetch.service
BindsTo=mcp-fetch.service

[Service]
Type=simple
WorkingDirectory=/path/to/workspace
Environment="NODE_ENV=production"
Environment="MCP_PORT=4113"
Environment="MCP_CMD=npx"
Environment="MCP_ARGS=[\"-y\",\"@modelcontextprotocol/server-memory\"]"
ExecStart=/usr/bin/node /path/to/workspace/.opencode/mcp-sse-proxy.mjs
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
```

### ChromaDB

**Type:** Node.js server (systemd) → Chroma cloud API
**Port:** 4114
**Role:** Knowledge retrieval for adaptive routing (non-blocking)
**Source:** Third-party Chroma MCP server. The server code is **not** part of this repository — the operator must obtain it (e.g., from a Chroma MCP provider) and place it at the path referenced in `WorkingDirectory` and `ExecStart`.

Systemd unit (`~/.config/systemd/user/chromadb-mcp.service`):

```ini
[Unit]
Description=ChromaDB MCP HTTP Server
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/path/to/mcp-servers
Environment="CHROMA_API_KEY=<key>"
Environment="CHROMA_TENANT=<tenant>"
Environment="CHROMA_DATABASE=<database>"
Environment="CHROMA_MCP_PORT=4114"
ExecStart=/usr/bin/node /path/to/mcp-servers/server.js
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
```

OpenCode config entry:

```jsonc
// ~/.config/opencode/opencode.jsonc
{
  "mcp": {
    "chromadb": {
      "type": "remote",
      "url": "http://localhost:4114",
      "enabled": true
    }
  }
}
```

Requires Chroma cloud credentials (API key, tenant, database).

### GitHub

**Type:** Node.js server (systemd) → GitHub REST API
**Port:** 4115
**Role:** GitHub API operations (repo, issues, PRs)
**Source:** Third-party GitHub MCP server. The server code is **not** part of this repository — the operator must obtain it (e.g., from a GitHub MCP provider) and place it at the path referenced in `WorkingDirectory` and `ExecStart`.

**Note:** Both ChromaDB and GitHub MCP servers may share the same source directory (`/path/to/mcp-servers/`). The directory name is a convention, not a ChromaDB-only location.

Systemd unit (`~/.config/systemd/user/github-mcp.service`):

```ini
[Unit]
Description=GitHub MCP HTTP Server
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/path/to/mcp-servers
Environment="GITHUB_MCP_PORT=4115"
Environment="GITHUB_TOKEN=<token>"
ExecStart=/usr/bin/node /path/to/mcp-servers/github-server.js
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
```

OpenCode config entry:

```jsonc
// ~/.config/opencode/opencode.jsonc
{
  "mcp": {
    "github": {
      "type": "remote",
      "url": "http://localhost:4115",
      "enabled": true
    }
  }
}
```

**Note:** Without `GITHUB_TOKEN`, only unauthenticated operations succeed (returns 401).

### Context7

**Type:** Remote provider (no local proxy)
**Port:** N/A
**Role:** Context/documentation retrieval

OpenCode config entry:

```jsonc
// ~/.config/opencode/opencode.jsonc
{
  "mcp": {
    "context7": {
      "type": "remote",
      "url": "https://mcp.context7.com/mcp",
      "headers": {
        "CONTEXT7_API_KEY": "${CONTEXT7_API_KEY}"
      },
      "enabled": true
    }
  }
}
```

Requires `CONTEXT7_API_KEY` environment variable (header-based auth per [Context7 docs](https://context7.com/docs/integrations/factory-ai)). Context7 is a third-party service — availability depends on the provider.

### Supabase

**Type:** Remote provider (no local proxy)
**Port:** N/A
**Role:** Database operations

OpenCode config entry:

```jsonc
// ~/.config/opencode/opencode.jsonc
{
  "mcp": {
    "supabase": {
      "type": "remote",
      "url": "https://mcp.supabase.com/mcp?project_ref=<project-ref>",
      "enabled": true
    }
  }
}
```

Supabase is a third-party service — availability depends on the provider.

---

## Start the Stack

### First-Time Start

```bash
# 1. Reload systemd to pick up new unit files
systemctl --user daemon-reload

# 2. Enable services for auto-start at boot
systemctl --user enable mcp-filesystem.service
systemctl --user enable mcp-git.service
systemctl --user enable mcp-fetch.service
systemctl --user enable mcp-memory.service
# Optional: enable ChromaDB/GitHub if deployed
systemctl --user enable chromadb-mcp.service github-mcp.service

# 3. Start services (cascade: git ← Wants → filesystem + fetch → memory)
systemctl --user start mcp-git.service
# Wants cascade: git starts filesystem + fetch, fetch starts memory

# 4. Verify all services are active
systemctl --user is-active mcp-filesystem.service mcp-git.service mcp-fetch.service mcp-memory.service
```

### Verify All 8 MCPs Connected

```bash
opencode mcp list
```

Expected output:
```
mcpServers:
  filesystem  http://localhost:4110
  git         http://localhost:4111
  fetch       http://localhost:4112
  memory      http://localhost:4113
  github      <remote>
  context7    <remote>
  chromadb    <remote>
  supabase    <remote>
```

### Verify Rate Limiter Active

```bash
# Run the rate limiter test to confirm it's active and functioning
cd /path/to/workspace && go test ./runtime/mcp/v2/ -run "TestSecurity_RateLimit" -count=1 -v 2>&1 | tail -5
```

Expected: `--- PASS: TestSecurity_RateLimit` — confirms rate limiter is wired and active.

**Note:** Metrics (`RateLimited` counter, burst/refill thresholds) are programmatically observable via `metrics.Global().Snapshot()` but not exposed via CLI or HTTP dashboard in the current runtime. Default rate limiter: burst=10000, refill=5000/sec.

---

## Verification

### Health Check

```bash
# 1. Service status
systemctl --user is-active mcp-*.service

# 2. Port responsiveness
for port in 4110 4111 4112 4113; do
    curl -s -o /dev/null -w "%{http_code}" http://localhost:$port && echo " :$port OK" || echo " :$port DOWN"
done
# Optional: check ChromaDB (4114) and GitHub (4115) if deployed
for port in 4114 4115; do curl -s -o /dev/null -w "%{http_code}" http://localhost:$port && echo " :$port OK" || echo " :$port SKIP"; done

# 3. MCP connectivity
opencode mcp list
```

### Metrics Check

```bash
# Check metrics via test utility
cd /path/to/workspace && go test ./runtime/metrics/ -v -count=1 -run TestMetrics 2>&1 | tail -10
```

Expected: All tests pass, counters are accessible.

### Integration Check

```bash
# 1. Filesystem: read a known file (expect 200 + JSON content)
curl -s -X POST http://localhost:4110 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"listAllowedDirectories"}' \
  | python3 -m json.tool | head -5

# 2. Git: repository status (expect 200 + branch info)
curl -s -X POST http://localhost:4111 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"git_status"}' \
  | python3 -m json.tool | head -5

# 3. Fetch: HTTP GET to a known endpoint (expect 200)
curl -s -X POST http://localhost:4112 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"fetch","params":{"url":"https://example.com"}}' \
  | python3 -m json.tool | head -5

# 4. Memory: store and retrieve (expect 200)
curl -s -X POST http://localhost:4113 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"listBuckets"}' \
  | python3 -m json.tool | head -5

# 5. Security test suite passes
cd /path/to/workspace && go test ./runtime/mcp/v2/ -run "TestSecurity" -count=1 -timeout 90s 2>&1 | tail -3
```

**Expected results:**
- Steps 1–4: HTTP 200 with valid JSON-RPC response
- Step 5: `ok` — all security tests pass

---

## Deployment Verification Checklist

```markdown
[ ] Go 1.22+ installed
[ ] Node.js 18+ installed
[ ] systemd user services directory exists (~/.config/systemd/user/)
[ ] 4–6 systemd unit files deployed (4 core + optional chromadb-mcp/github-mcp)
[ ] SSE proxy script exists (.opencode/mcp-sse-proxy.mjs)
[ ] Core services enabled for boot-time start (mcp-*.service)
[ ] Optional services enabled (chromadb-mcp, github-mcp) if deployed
[ ] Required ports 4110-4113 respond to HTTP (4 core MCPs)
[ ] Optional ports 4114-4115 respond to HTTP if ChromaDB/GitHub are deployed
[ ] opencode mcp list shows 8 servers
[ ] Environment variables set (GITHUB_TOKEN, CONTEXT7_API_KEY, etc.)
[ ] Linger enabled for boot-time service start
[ ] First test request succeeds
[ ] Metrics dashboard renders non-zero uptime
[ ] Full security test suite passes
```
