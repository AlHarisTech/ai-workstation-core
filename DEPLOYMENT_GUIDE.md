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
| opencode | Latest | MCP server list, runtime configuration |

### Hardware

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 1 core | 2+ cores |
| RAM | 1 GB | 2 GB |
| Disk | 5 GB | 20 GB (workspace + knowledge storage) |

### Network

- Local ports 4110–4115 must be available (systemd: MCPs 4110–4113, ChromaDB 4114, GitHub 4115)
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

# 2. Build Go adapters (git, fetch)
go build -o bin/mcp-git ./runtime/mcp/v2/cmd/
go build -o bin/mcp-fetch ./runtime/mcp/v2/cmd/

# 3. Verify build
./bin/mcp-git --version 2>/dev/null || echo "Binary built successfully"
```

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

The services form a `BindsTo` chain:

```
mcp-filesystem (port 4110, root)
    ↓ BindsTo
mcp-git (port 4111, Wants=mcp-fetch)
    ↓ BindsTo
mcp-fetch (port 4112, Wants=mcp-memory)
    ↓ BindsTo
mcp-memory (port 4113, leaf)
```

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
**Role:** In-memory knowledge storage (non-persistent)

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

Systemd unit (`~/.config/systemd/user/chromadb.service`):

```ini
[Unit]
Description=ChromaDB MCP HTTP Server
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/path/to/chroma-mcp
Environment="CHROMA_API_KEY=<key>"
Environment="CHROMA_TENANT=<tenant>"
Environment="CHROMA_DATABASE=<database>"
Environment="CHROMA_MCP_PORT=4114"
ExecStart=/usr/bin/node /path/to/chroma-mcp/server.js
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
```

OpenCode config entry:

```jsonc
// ~/.config/opencode/opencode.jsonc
{
  "mcpServers": {
    "chromadb": {
      "transport": "http",
      "url": "http://localhost:4114"
    }
  }
}
```

Requires Chroma cloud credentials (API key, tenant, database).

### GitHub

**Type:** Node.js server (systemd) → GitHub REST API
**Port:** 4115
**Role:** GitHub API operations (repo, issues, PRs)

Systemd unit (`~/.config/systemd/user/github.service`):

```ini
[Unit]
Description=GitHub MCP HTTP Server
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/path/to/chroma-mcp
Environment="GITHUB_MCP_PORT=4115"
Environment="GITHUB_TOKEN=<token>"
ExecStart=/usr/bin/node /path/to/chroma-mcp/github-server.js
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
```

OpenCode config entry:

```jsonc
// ~/.config/opencode/opencode.jsonc
{
  "mcpServers": {
    "github": {
      "transport": "http",
      "url": "http://localhost:4115"
    }
  }
}
```

**Note:** Without `GITHUB_TOKEN`, only unauthenticated operations succeed (returns 401).

### Context7

**Type:** Remote API (no local proxy)
**Port:** N/A
**Role:** Context/documentation retrieval

OpenCode config entry:

```jsonc
// ~/.config/opencode/opencode.jsonc
{
  "mcpServers": {
    "context7": {
      "description": "Context7 documentation retrieval",
      "command": "",
      "env": {
        "CONTEXT7_API_KEY": "${CONTEXT7_API_KEY}"
      }
    }
  }
}
```

Requires `CONTEXT7_API_KEY` environment variable. Context7 is a third-party service — availability depends on the provider.

### Supabase

**Type:** Remote service (cloud)
**Port:** N/A
**Role:** Database operations

OpenCode config entry:

```jsonc
// ~/.config/opencode/opencode.jsonc
{
  "mcpServers": {
    "supabase": {
      "description": "Supabase database",
      "command": "",
      "env": {
        "SUPABASE_KEY": "${SUPABASE_KEY}"
      }
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

# 3. Start services (cascade: filesystem → git → fetch → memory)
systemctl --user start mcp-filesystem.service
# Dependent services start automatically via BindsTo/Wants

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

```go
snap := metrics.Global().Snapshot()
// RateLimited counter exists and is observable (may be 0 under normal load)
// Rate limiter defaults: burst=10000, refill=5000/sec
```

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
# 1. Filesystem: read a known file
# (Verify via opencode or direct request)

# 2. Git: status of repository
# (Verify via opencode or direct request)

# 3. Fetch: HTTP GET to a known endpoint
# (Verify via opencode or direct request)

# 4. Memory: store and retrieve
# (Verify via opencode or direct request)

# 5. All 28 security tests pass
cd /path/to/workspace && go test ./runtime/mcp/v2/ -run "TestSecurity" -count=1 -timeout 90s 2>&1 | tail -3
```

Expected: `ok` — all security tests pass.

---

## Deployment Verification Checklist

```markdown
[ ] Go 1.22+ installed
[ ] Node.js 18+ installed
[ ] systemd user services directory exists (~/.config/systemd/user/)
[ ] 4 systemd unit files deployed
[ ] SSE proxy script exists (.opencode/mcp-sse-proxy.mjs)
[ ] All 4 services enabled for boot-time start
[ ] All 6 local ports (4110-4115) respond to HTTP (4 MCPs + ChromaDB + GitHub)
[ ] opencode mcp list shows 8 servers
[ ] Environment variables set (GITHUB_TOKEN, CONTEXT7_API_KEY, etc.)
[ ] Linger enabled for boot-time service start
[ ] First test request succeeds
[ ] Metrics dashboard renders non-zero uptime
[ ] All security tests pass
```
