# Runbooks — MCP Runtime v3.1.1-stable

## Architecture Reference

**Version:** v3.1.1-stable

**Pipeline (11 stages, executed in order per request):**

| Stage | Component | Role |
|-------|-----------|------|
| 0 | Rate Limiter | Token bucket burst control |
| 1 | Validation | Request schema validation |
| 2 | Policy Engine | Action-level authorization |
| 3 | Router | Capability → server resolution |
| 4 | Knowledge Retrieval | ChromaDB context queries (non-blocking, additive) |
| 4.5 | Adaptive Override | Knowledge-driven server re-selection |
| 5 | Routing | MCP server dispatch |
| 5.5 | Enforcement | Control plane authorization gate |
| 6 | Execution | MCP server operation |
| 7 | Learning | Feedback + stability update + audit |
| 8 | Normalize | Response formatting |

**Core security principle:** Stages 2 (Policy) and 5.5 (Enforcement) enforce governance. Stage 4.5 (Adaptive Override) can influence server selection but **cannot bypass governance** — routing override occurs before enforcement.

## Operational Invariants

These invariants must never be violated. Any observed deviation is a P1 incident.

| ID | Invariant | Enforced By |
|----|-----------|-------------|
| I1 | Policy cannot be bypassed | Stage 2 Policy Engine — blocks before routing reaches server |
| I2 | Enforcement has final authority | Stage 5.5 — blocks after routing, regardless of override |
| I3 | Panics must be recovered | `recover()` in Process() + TimeoutAdapter goroutine |
| I4 | DecisionTrace must be attached | Deferred `resp.Meta.DecisionTrace = trace` at end of Process() |
| I5 | Adaptive Routing cannot bypass governance | Override (Stage 4.5) precedes enforcement (Stage 5.5) |
| I6 | Requests may fail closed | Every error path sets `Status: "error"` with explicit error code |
| I7 | Rate limiting occurs before validation | Stage 0 precedes Stage 1 in Process() |
| I8 | MCP failures must be isolated | Independent server instances, per-MCP systemd units |

## Operational Targets (SLO Baseline)

| Metric | Target | How to Measure |
|--------|--------|----------------|
| Availability | 99.9% | Gateway Process() returns non-nil response |
| Gateway Error Rate | < 5% | `snap.Gateway.RequestsFailed / snap.Gateway.RequestsTotal` |
| Recovered Panics | 0 expected | `snap.Gateway.Panics` — non-zero requires investigation |
| Rate Limited Requests | < 1% | `snap.Gateway.RateLimited / snap.Gateway.RequestsTotal` |
| Local MCP Health | 100% | All 4 local systemd services active (ports 4110–4113) |
| Remote MCP Reachability | No SLO (third-party dependency) | GitHub, Context7, ChromaDB, Supabase are external services |
| Decision Trace Coverage | 100% | Every response includes non-nil DecisionTrace |

## Runtime Operations

### Start Runtime

```bash
# Start all MCP services (user-level systemd)
systemctl --user start mcp-filesystem.service
systemctl --user start mcp-git.service
systemctl --user start mcp-fetch.service
systemctl --user start mcp-memory.service

# Verify all services are running
systemctl --user status mcp-*.service
```

Services form a **BindsTo cascade** — each service stops if its dependency fails:

```
mcp-filesystem  (root — no MCP dependencies)
    ↓ BindsTo
mcp-git  (also Wants=mcp-fetch)
    ↓ BindsTo
mcp-fetch  (also Wants=mcp-memory)
    ↓ BindsTo
mcp-memory  (leaf)
```

If linger is enabled (`loginctl enable-linger asem`), services start automatically at boot.

### Stop Runtime

```bash
# Stop all services (reverse dependency order)
systemctl --user stop mcp-memory.service
systemctl --user stop mcp-fetch.service
systemctl --user stop mcp-git.service
systemctl --user stop mcp-filesystem.service
```

Check for stuck processes:

```bash
systemctl --user list-units --failed
```

### Restart Runtime

```bash
# Restart a single service
systemctl --user restart mcp-git.service

# Restart the whole stack
systemctl --user restart mcp-*.service
```

### Health Verification

```bash
# Service status
systemctl --user is-active mcp-filesystem.service

# Network connectivity (all 4 local servers)
curl -s http://localhost:4110/health 2>/dev/null || echo "filesystem: down"
curl -s http://localhost:4111/health 2>/dev/null || echo "git: down"
curl -s http://localhost:4112/health 2>/dev/null || echo "fetch: down"
curl -s http://localhost:4113/health 2>/dev/null || echo "memory: down"

# MCP server list (requires opencode)
opencode mcp list
```

Expected output from `opencode mcp list`:
- `filesystem` (systemd, port 4110)
- `git` (systemd, port 4111)
- `fetch` (systemd, port 4112)
- `memory` (systemd, port 4113)
- `supabase` (cloud/remote)
- `chromadb` (cloud/remote)
- `github` (cloud/remote)
- `context7` (cloud/remote)

### Runtime Health Checklist

```markdown
[ ] All 4 local MCP services active (systemctl --user is-active mcp-filesystem.service mcp-git.service mcp-fetch.service mcp-memory.service)
[ ] All 4 local ports respond (curl http://localhost:411{0,1,2,3}/health)
[ ] Metrics available (metrics.Global().Snapshot() returns data)
[ ] Dashboard responding (NewCLIDashboard(nil).Render())
[ ] Error rate < 5% (snap.Gateway.RequestsFailed / RequestsTotal)
[ ] Panic count stable (snap.Gateway.Panics — no unexpected increase)
[ ] Rate limit not saturated (RateLimited << RequestsTotal)
```

---

## MCP Inventory

| MCP | Type | Port | Criticality | Failure Impact |
|-----|------|------|-------------|----------------|
| Filesystem | Local (systemd) | 4110 | Critical | File operations unavailable |
| Git | Local (systemd) | 4111 | Critical | Version control operations unavailable |
| Fetch | Local (systemd) | 4112 | Critical | HTTP fetch operations unavailable |
| Memory | Local (systemd) | 4113 | Critical | Knowledge storage unavailable |
| GitHub | Remote | — | High | GitHub API operations degraded |
| Context7 | Remote | — | High | Context retrieval unavailable |
| ChromaDB | Remote | — | Medium | Knowledge retrieval degraded (non-blocking) |
| Supabase | Remote | — | Medium | Database operations degraded |

---

## MCP Operations

### Add MCP

Add a new MCP server by registering it in code:

```go
// runtime/mcp/v2/gateway.go
g.servers["<name>"] = &YourServer{}
```

Or via OpenCode configuration (`~/.config/opencode/opencode.jsonc`):

```jsonc
{
  "mcpServers": {
    "<name>": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-<name>"],
      "env": {
        "KEY": "value"
      }
    }
  }
}
```

After adding, create a systemd unit file for production:

```ini
# ~/.config/systemd/user/mcp-<name>.service
[Unit]
Description=MCP <Name> Server
After=network.target

[Service]
Type=simple
ExecStart=npx -y @modelcontextprotocol/server-<name>
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

Then:

```bash
systemctl --user daemon-reload
systemctl --user enable --now mcp-<name>.service
```

### Remove MCP

Removal depends on how the MCP was added:

**For systemd-managed MCPs (filesystem, git, fetch, memory):**

```bash
# Stop and disable the systemd unit
systemctl --user disable --now mcp-<name>.service
rm ~/.config/systemd/user/mcp-<name>.service
systemctl --user daemon-reload
```

**For OpenCode-configured MCPs (supabase, chromadb, github, context7):**

```jsonc
// Edit ~/.config/opencode/opencode.jsonc and remove the server entry
{
  "mcpServers": {
    // "<name>": { ... }  ← delete this block
  }
}
```

**For runtime-registered MCPs (code level):**

```go
// runtime/mcp/v2/gateway.go
// Either delete the server from registerDefaults()
// Or remove the g.servers["<name>"] = ... line
```

### Validate MCP

```go
// Validate filesystem MCP reads a file correctly
req := &MCPRequest{
    Action: ActionType{Type: "filesystem", Operation: "read"},
    Payload: Payload{Parameters: map[string]any{"path": "/tmp/test.txt"}},
    Policy: MCPPolicy{Allow: []string{"filesystem.*"}, Deny: []string{}},
}
resp := gw.Process(req)
// Expected: Status="success" with file content in resp.Result.Data
// If filesystem is down: Error.Code="EXECUTION_FAILED"
// If path is invalid: Error.Code="VALIDATION_ERROR"
```

Expected error codes from a healthy server:
- `EXECUTION_FAILED` — server responded but operation failed (e.g., file not found)
- `RATE_LIMITED` — burst limit exceeded, retry later
- `VALIDATION_ERROR` — malformed request parameters

### Check MCP Health

Each MCP server has independent health. Failure of one does not cascade to others (verified in `TestSecurity_MCPFailureIsolation`).

```bash
# Check individual systemd service
systemctl --user status mcp-filesystem.service

# Check auto-restart behavior
journalctl --user -u mcp-filesystem.service -n 20 --no-pager
```

Auto-recovery via `Restart=on-failure` with 5-second delay.

---

## Observability

### Read Metrics

Client library for programmatic access:

```go
import "github.com/AlHarisTech/ai-workstation-core/runtime/metrics"

snap := metrics.Global().Snapshot()

// Gateway counters
snap.Gateway.RequestsTotal     // Total requests processed
snap.Gateway.RequestsAllowed   // Requests that passed policy + enforcement
snap.Gateway.RequestsBlocked   // Requests blocked by policy or enforcement
snap.Gateway.RequestsFailed    // Execution failures
snap.Gateway.RateLimited       // Requests rate-limited by Stage 0 bucket
snap.Gateway.Panics            // Recovered panics

// Runtime health
snap.Runtime.UptimeSeconds     // Seconds since metrics initialized
snap.Runtime.ThroughputRPS     // Requests per second
snap.Runtime.BlockRate         // Blocked / total ratio
snap.Runtime.ErrorRate         // Failed / total ratio

// Enforcement stats
snap.Enforcement.Evaluations   // Enforcement checks performed
snap.Enforcement.Blocked       // Requests enforcement denied
snap.Enforcement.Violations    // Policy intelligence violations

// Per-stage invocation counts (in snap.Stages)
// Each entry: Label, Invocations, AvgLatencyNs, MaxLatencyNs, Failures
```

### Read Dashboard

```go
dash := metrics.NewCLIDashboard(nil)

// Full view
fmt.Println(dash.Render())

// Compact (single-line) view
fmt.Println(dash.RenderCompact())
```

Compact output format:

```
[<uptime>s] <rps> rps | req=<N> ok=<N> blk=<N> fail=<N> rate=<N> enfc=<N> blk=<N> | pi=<N> drf=<N> sug=<N> | learn=<N> ok=<N> fail=<N>
```

### Read Decision Traces

Every response from `gw.Process()` includes a `DecisionTrace` in `resp.Meta.DecisionTrace`:

```go
trace := resp.Meta.DecisionTrace

trace.TraceID         // Unique trace identifier
trace.RequestID       // Correlates to the original request
trace.SelectedServer  // Server chosen for execution
trace.ServerScores    // Map of server → score from Knowledge Scoring
trace.SecondBest      // Runner-up server (if applicable)
trace.ScoreDelta      // Score margin between selected and second-best
trace.KnowledgeUsed   // Knowledge collections queried
trace.Steps           // Ordered list of stage outcomes
```

Each step has:
- `Stage` — one of: `rate_limit`, `validate`, `policy`, `resolve`, `knowledge`, `override`, `route`, `enforcement`, `execute`, `panic`
- `Output` — stage-specific result (e.g., `allowed`, `blocked`, `ok`, `denied`)
- `Meta` — stage-specific metadata (errors, scores, server names)

### Investigate Routing

When a request routes to an unexpected server, examine the trace:

```go
for _, s := range resp.Meta.DecisionTrace.Steps {
    switch s.Stage {
    case "override":
        // Knowledge scoring caused a routing change
        // s.Output = "filesystem→github" (from → to)
        // s.Meta["from_score"], s.Meta["to_score"]
    case "knowledge":
        // Knowledge retrieval results
        // s.Meta["query"], s.Meta["docs"]
    }
}
```

Key principle: **Stage 5 (Route) can be overridden by Knowledge Scoring (Stage 4.5), but Stage 5.5 (Enforcement) always has the final say.** A routing override is not a security bypass.

To verify routing decisions:

```bash
# Check the log output (includes scores and stability)
journalctl --user -u mcp-git.service --no-pager | grep "routing_mode"
```

---

## Security Operations

### Policy Block Investigation

When a request is denied by policy:

```go
// Response code is POLICY_DENIED
resp.Error.Code // "POLICY_DENIED"
resp.Error.Message // Reason from policy engine

// Trace shows which stage blocked it
for _, s := range resp.Meta.DecisionTrace.Steps {
    if s.Stage == "policy" {
        // s.Output is "denied"
        // s.Meta["reason"] contains the policy rule that triggered
    }
}
```

Common causes:
- Operation not in `Policy.Allow` list
- Operation in `Policy.Deny` list
- Default deny when no allow rule matches

### Rate Limit Investigation

When a request is rate limited:

```go
// Response code is RATE_LIMITED
resp.Error.Code // "RATE_LIMITED"

// Trace shows rate_limit:blocked at Stage 0
for _, s := range resp.Meta.DecisionTrace.Steps {
    if s.Stage == "rate_limit" {
        // s.Output is "blocked"
    }
}

// Metrics counter
snap := metrics.Global().Snapshot()
snap.Gateway.RateLimited // Total rate-limited requests
```

Rate limiter defaults: burst=10000, refill=5000/sec. Configurable at:

```go
// runtime/mcp/v2/gateway.go, NewGateway()
rateLimiter: NewTokenBucket(10000, 5000),
```

If rate limiting is too aggressive, increase these values. If too permissive (DoS risk), decrease them.

### Panic Recovery Verification

Panics are caught by `recover()` in `Gateway.Process()` and `TimeoutAdapter` goroutines.

```go
// Response after a recovered panic
resp.Status        // "error"
resp.Error.Code    // "INTERNAL_PANIC"
resp.Error.Message // "panic: <original panic value>"

// Trace contains panic:recovered step
for _, s := range resp.Meta.DecisionTrace.Steps {
    if s.Stage == "panic" {
        // s.Output is "recovered"
        // s.Meta["panic"] contains the original panic value
    }
}

// Metrics counter increments
metrics.Global().Snapshot().Gateway.Panics
```

If `Panics` counter is non-zero, investigate the root cause:

```bash
# Check logs for PANIC RECOVERED messages
journalctl --user -u mcp-git.service --no-pager | grep "PANIC RECOVERED"
```

---

## Incident Classification

### P1 — Critical

System is unavailable or data is at risk.

| Condition | Action |
|-----------|--------|
| Gateway process crash (not recovered) | Restart gateway, investigate crash dump |
| All MCP servers down | Check systemd, kernel logs, resource exhaustion |
| Data loss or corruption | Restore from backup, engage engineering |
| Security breach detected | Immediately block all routes, engage security team |

### P2 — Major

Core functionality degraded but system is running.

| Condition | Action |
|-----------|--------|
| Single MCP server down (e.g., filesystem) | Restart service, check logs for failure cause |
| Rate limiter blocking legitimate traffic | Increase burst/refill, monitor impact |
| Enforcement blocking wrong requests | Review enforcement rules, update configuration |
| Metrics show high error rate (>5%) | Investigate execution failures, check MCP health |

### P3 — Minor

Non-critical functionality affected.

| Condition | Action |
|-----------|--------|
| Knowledge retrieval failed (non-blocking) | Check ChromaDB connectivity, verify collection exists |
| Single request returning unexpected error | Check request parameters, trace through stages |
| Dashboard showing stale metrics | Verify metrics initialization, check uptime |
| Routing suboptimal but not wrong | Review knowledge scoring weights, check convergence |

### P4 — Informational

No user impact, but worth noting.

| Condition | Action |
|-----------|--------|
| Unexplained routing override | Trace the decision, log for later analysis |
| Rate limit almost reached (>80% burst) | Consider scaling or increasing limits |
| Learning engine weight shift | Review recent success/failure patterns |
| Policy intelligence drift detection | Review recorded events for policy gaps |

---

## Known Limitations

| Limitation | Impact | Workaround |
|------------|--------|------------|
| GitHub authenticated operations require `GITHUB_TOKEN` | GitHub MCP cannot perform authenticated operations | Set `GITHUB_TOKEN` environment variable before starting service |
| Adaptive routing depends on knowledge scoring quality | Routing may be suboptimal with sparse knowledge | Ensure ChromaDB collection is populated with relevant context |
| Remote MCP availability depends on external providers | GitHub, Context7, ChromaDB, Supabase may be unreachable | Built-in fail-close: requests return error codes; no cascade |
| Rate limiting is process-local, not distributed | Multiple gateway instances have independent rate limiters | Coordinate burst/refill values across instances manually |
| No user auth / identity layer | All requests treated as single operator | By design — single-operator runtime; not a security gap |
| No persistent audit log storage | Audit records lost on process restart | Pipe `LogAudit()` output to file or external log sink |
