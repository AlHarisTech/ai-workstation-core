# Runbooks — MCP Runtime v3.1.1-stable

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

Services start in cascade: `mcp-filesystem` → `mcp-git` (Wants mcp-fetch) → `mcp-fetch` (Wants mcp-memory).

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

```bash
# Disable and remove systemd unit
systemctl --user disable --now mcp-<name>.service
rm ~/.config/systemd/user/mcp-<name>.service
systemctl --user daemon-reload

# Remove from opencode config
# Edit ~/.config/opencode/opencode.jsonc and delete the entry
```

### Validate MCP

```go
// Test a server responds correctly via Process()
req := &MCPRequest{
    Action: ActionType{Type: "<type>", Operation: "<op>"},
    Payload: Payload{Parameters: map[string]any{}},
    Policy: MCPPolicy{Allow: []string{"<type>.*"}},
}
resp := gw.Process(req)
// resp.Status should be "success" or "error" (never nil)
// resp.Error.Code reveals the failure reason
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
