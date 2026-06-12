# F1 — Stability Baseline Capture

**Date:** 2026-06-12T14:28:16Z
**Phase:** F (Runtime Stabilization & Governance)

## 1. Memory Baseline

| Service | RSS (KB) | VSZ (KB) | Process |
|---|---|---|---|
| mcp-filesystem | 51,556 | 610,708 | node |
| mcp-git | 51,628 | 610,344 | node |
| mcp-fetch | 51,652 | 610,300 | node |
| mcp-memory | 52,004 | 610,316 | node |
| chromadb-mcp | 78,516 | 21,626,976 | node |
| github-mcp | 77,248 | 11,140,280 | node |

Observations:
- Core services: ~51-52MB RSS, ~610MB VSZ — consistent across all 4
- chromadb: 78MB RSS, 21.6GB VSZ (mmap for embedding storage)
- github: 77MB RSS, 11.1GB VSZ

## 2. Restart / Reconnect Baseline

### Last hour (includes intentional E7.2 restarts)

| Service | Restarts | Started At |
|---|---|---|
| mcp-filesystem | 3 | 17:06:38 +03 |
| mcp-git | 6 | 17:06:38 +03 |
| mcp-fetch | 8 | 17:06:38 +03 |
| mcp-memory | 9 | 17:06:38 +03 |
| chromadb-mcp | 115 | 17:00:05 +03 |
| github-mcp | 116 | 17:00:05 +03 |

Note: chromadb/github counts include token error loop before mcp-tokens.env was restored.

### Lifetime

| Service | Total Restarts |
|---|---|
| mcp-filesystem | 21 |
| mcp-git | 20 |
| mcp-fetch | 24 |
| mcp-memory | 28 |
| chromadb-mcp | 126 |
| github-mcp | 334 |

Observations:
- chromadb: 126 lifetime — includes initial setup + token error loop
- github: 334 lifetime — includes 216 historical (missing GITHUB_TOKEN) + ~118 token error loop

## 3. MCP Latency Baseline

| Service | Port | HTTP | Latency |
|---|---|---|---|
| filesystem | 4110 | 200 | 9ms |
| git | 4111 | 200 | 8ms |
| fetch | 4112 | 200 | 8ms |
| memory | 4113 | 200 | 9ms |
| chromadb | 4114 | 200 | 8ms |
| github | 4115 | 200 | 8ms |

Observations:
- All services respond in 8-9ms — very consistent
- No outliers or timeouts
