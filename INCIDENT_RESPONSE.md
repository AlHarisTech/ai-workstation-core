# Incident Response — MCP Runtime v3.1.1-stable

## Detection

### Alert

| Source | What to Watch | Threshold | Action |
|--------|--------------|-----------|--------|
| Metrics | `Panics` counter | See Panic Alert Thresholds below | Investigate panic source (logs, trace) |
| Metrics | `ErrorRate > 5%` | > 5% over 5 min | Check MCP health, recent changes |
| Metrics | `RateLimited / Total > 1%` | > 1% | Rate limiter too aggressive or attack in progress |
| Systemd | Service `failed` or `inactive` | Any | Auto-restart already triggered; check if persistent |
| Systemd | Cascade stop (`BindsTo` chain) | > 1 service down | Root service failure cascaded; fix root first |
| Port check | `localhost:411{0,1,2,3}` unreachable | Any | MCP service down or network issue |

### Panic Alert Thresholds

| Count | Severity | Action |
|-------|----------|--------|
| `Panics > 0` | Alert | Investigate panic source (logs, trace) |
| `Panics > 3 / hour` | P2 | Root cause analysis, rollback if code change |
| `Panics > 10 / hour` | P1 | Immediate containment, engage engineering |

### Metric

```go
snap := metrics.Global().Snapshot()

// Immediate red flags
// See Panic Alert Thresholds table for panic escalation
if float64(snap.Gateway.RequestsFailed)/float64(snap.Gateway.RequestsTotal) > 0.05 {
    // P2 incident — error rate exceeds SLO
}
if float64(snap.Gateway.RateLimited)/float64(snap.Gateway.RequestsTotal) > 0.01 {
    // P2 incident — rate limiting too aggressive
}
```

### Log

```bash
# Check all MCP services for errors
journalctl --user -u 'mcp-*.service' -n 100 --no-pager | grep -E "panic|error|fatal|FATAL|PANIC"

# Check for PANIC RECOVERED events (recovered gracefully, but still noteworthy)
journalctl --user -u 'mcp-*.service' --no-pager | grep "PANIC RECOVERED"

# Check for enforcement blocks
journalctl --user -u 'mcp-*.service' --no-pager | grep "enforcement blocked"

# Check for routing anomalies
journalctl --user -u 'mcp-*.service' --no-pager | grep "routing override"
```

### User Report

| Report | Likely Code | Initial Triage |
|--------|------------|----------------|
| "Request failed" | `EXECUTION_FAILED` | P3 — check MCP health |
| "Request blocked" | `POLICY_DENIED` or `ENFORCEMENT_BLOCKED` | P3 — check policy/rules |
| "Rate limited" | `RATE_LIMITED` | P3 — check rate limiter config |
| "Server not found" | `ROUTE_NOT_FOUND` | P3 — check routing table |
| "Nothing works" | Multiple errors | P1 — check systemd, cascade chain |
| "Validation error" | `VALIDATION_ERROR` | P4 — check request parameters |

---

## Triage

### P1 — Critical

System is unavailable or has active data loss/security risk.

| Trigger | Initial Response |
|---------|-----------------|
| All local MCPs down (cascade failure) | Check cascade root (filesystem), restore from there |
| Security breach detected | Immediately block all routes, engage security team |
| Data loss or corruption | Stop affected services, restore from backup |
| Sustained abnormal traffic causing rate-limit saturation | Reduce rate limiter burst temporarily, monitor traffic pattern |

**Response time:** Immediate. All available operators.

**Containment first, root cause second.**

### P2 — Major

Core functionality degraded. Single local MCP down.

| Trigger | Initial Response |
|---------|-----------------|
| Single local MCP down (filesystem/git/fetch/memory) | Restart service, check logs for failure cause |
| Rate limiter blocking legitimate traffic | Increase burst/refill, monitor impact |
| Panic events recurring (> 3 in 1 hour) | Investigate panic trace, rollback if code change |
| Enforcement blocking wrong requests | Review and update enforcement rules |
| Error rate exceeds 5% | Investigate execution failures, check MCP health |

**Response time:** Within 15 minutes.

### P3 — Minor

Non-critical functionality affected. Remote MCP unavailable.

| Trigger | Initial Response |
|---------|-----------------|
| Remote MCP down (GitHub, Context7, ChromaDB, Supabase) | Verify external service status, check network |
| Single operation returning unexpected error | Check request parameters, trace through stages |
| Knowledge retrieval failed (non-blocking) | Check ChromaDB connectivity, verify collection |
| Routing suboptimal but not wrong | Review knowledge scoring weights, check convergence |

**Response time:** Within 1 hour.

### P4 — Informational

No user impact, but worth noting.

| Trigger | Initial Response |
|---------|-----------------|
| Unexplained routing override | Trace the decision, log for later analysis |
| Rate limit approaching 80% of burst | Consider scaling or increasing limits |
| Learning engine weight shift | Review recent success/failure patterns |
| Dashboard shows stale metrics | Verify metrics initialization, check uptime |

**Response time:** Log for follow-up. No immediate action.

---

## Containment

### Disable MCP

Stop the specific MCP service to prevent further damage or error cascades:

```bash
# Disable a single systemd-managed MCP
systemctl --user stop mcp-filesystem.service

# Note: dependent services (git, fetch, memory) will also stop via BindsTo
systemctl --user status mcp-git.service mcp-fetch.service mcp-memory.service

# Prevent auto-restart during investigation
systemctl --user mask mcp-filesystem.service
# ⚠ Warning: `mask` persists after reboot. Must explicitly `unmask` to re-enable.
# Use `stop` instead of `mask` if you only need temporary containment.
```

**Re-enable when resolved (only needed if `mask` was used):**

```bash
systemctl --user unmask mcp-filesystem.service
systemctl --user restart mcp-filesystem.service
systemctl --user status mcp-git.service mcp-fetch.service mcp-memory.service
```

**If `stop` was used instead of `mask`:** simply `systemctl --user start mcp-filesystem.service` to re-enable.

### Block Route

Block a specific operation or server during an incident using available operator controls:

**Option 1 — Stop the service (fastest containment):**

```bash
systemctl --user stop mcp-filesystem.service
# All operations to this server will return EXECUTION_FAILED
# Dependent services stop via BindsTo cascade
```

**Option 2 — Remove the route from OpenCode config (persistent block):**

```bash
# 1. Edit the OpenCode config
# Remove the server entry from ~/.config/opencode/opencode.jsonc

# 2. Reload opencode
opencode mcp list  # Verify the server no longer appears
```

**Option 3 — Add enforcement rule at code level (requires restart):**

```go
// Edit runtime/mcp/v2/gateway.go and add:
g.enforcement.AddRule("filesystem", "filesystem.read",
    EnforcementRule{Allowed: false, Reason: "incident containment"})

// Rebuild and restart the service
go build ./runtime/mcp/v2/cmd/ && systemctl --user restart mcp-filesystem.service
```

To re-enable: reverse the action taken (start service, restore config, remove rule and restart).

### Fallback

When a service cannot be immediately restored, use fallback procedures:

| Fallback Scenario | Action |
|------------------|--------|
| Filesystem unavailable | Route file operations to alternative path or defer |
| Git unavailable | Use local git commands directly, bypass MCP |
| Fetch unavailable | Use curl/wget directly, bypass MCP |
| Memory unavailable | Knowledge-dependent routing falls back to default scoring (non-blocking) |
| Remote MCP unavailable | Fail-close: error is returned, no cascade to other services |

**Important:** There is no automatic failover between MCPs. Each operation is mapped to a specific server. If that server is down, the operation returns `EXECUTION_FAILED`. This is intentional — automatic failover between different MCP servers would violate the routing contract.

---

## Resolution

### Fix

Follow the recovery procedure for the specific component:

| Component | Procedure Location |
|-----------|-------------------|
| Host application crash | `RECOVERY_PROCEDURES.md` → Host Application Crash |
| Panic event | `RECOVERY_PROCEDURES.md` → Panic Event |
| Deadlock | `RECOVERY_PROCEDURES.md` → Deadlock |
| High memory | `RECOVERY_PROCEDURES.md` → High Memory Usage |
| Filesystem down | `RECOVERY_PROCEDURES.md` → Filesystem Down |
| Git down | `RECOVERY_PROCEDURES.md` → Git Down |
| Fetch down | `RECOVERY_PROCEDURES.md` → Fetch Down |
| Memory down | `RECOVERY_PROCEDURES.md` → Memory Down |
| Context7 down | `RECOVERY_PROCEDURES.md` → Context7 Down |
| GitHub failure | `RECOVERY_PROCEDURES.md` → GitHub Failure |
| Rate limiter misconfig | `RECOVERY_PROCEDURES.md` → Rate Limiter Misconfiguration |
| Enforcement block | `RECOVERY_PROCEDURES.md` → Enforcement Engine Blocking |
| Router failure | `RECOVERY_PROCEDURES.md` → Router Resolution Failure |
| Knowledge degraded | `RECOVERY_PROCEDURES.md` → Knowledge Engine Degraded |

### Validate

After applying the fix, run the functional verification:

```go
// 1. Verify specific operation that failed now succeeds
req := &MCPRequest{
    Action:  ActionType{Type: "filesystem", Operation: "read"},
    Payload: Payload{Parameters: map[string]any{"path": "/tmp/test.txt"}},
    Policy:  MCPPolicy{Allow: []string{"filesystem.*"}},
}
resp := gw.Process(req)
if resp.Status != "success" {
    // Fix not complete — investigate further
}
```

```bash
# 2. Verify service health
systemctl --user is-active mcp-filesystem.service

# 3. Verify all 8 MCPs connected (4 local + 4 remote)
opencode mcp list  # Expect: filesystem, git, fetch, memory, supabase, chromadb, github, context7

# 4. Verify error rate — count recent errors from audit logs
journalctl --user -u mcp-filesystem.service --since "10 min ago" --no-pager | grep -c '"status":"error"' | xargs echo "Errors in last 10 min:"
```

### Document

For every incident, record:

```yaml
incident:
  id: "INC-YYYYMMDD-NNN"
  severity: P1|P2|P3|P4
  detected: "<timestamp>"
  resolved: "<timestamp>"
  component: "<service or component name>"
  symptom: "<what the user/operator observed>"
  root_cause: "<what caused the failure>"
  fix: "<what was done to resolve it>"
  rollback: "<was rollback needed? Y/N>"
  verification: "<how fix was confirmed>"
```

---

## Postmortem

### Root Cause Analysis

For P1 and P2 incidents, conduct a formal root cause analysis:

```bash
# Gather evidence
journalctl --user -u 'mcp-*.service' --since "1 hour before" --until "now" > incident-YYYYMMDD-logs.txt
```

```go
// Snapshot metrics at time of incident
snap := metrics.Global().Snapshot()
// Save for analysis
```

**Analysis questions:**
1. Was this a code defect, configuration error, or external dependency failure?
2. Was there a recent deployment that correlates with the incident?
3. Did the system behave as designed (fail-close, panic recovery)?
4. Were alerts triggered before the incident was reported?
5. Was the recovery procedure followed? If not, why?

### Impact Assessment

| Dimension | Assessment |
|-----------|------------|
| Requests affected | Count of failed/blocked requests |
| Duration | Time from detection to resolution |
| Services affected | Which MCPs were impacted |
| Data loss | Was any data lost? Recoverable? |
| User visibility | Was the impact internal or external? |

### Lessons Learned

For each incident, answer:

1. **What went well?** (e.g., panic recovery worked, fail-close prevented cascade)
2. **What went wrong?** (e.g., alert threshold too high, runbook missing a step)
3. **What will we change?** (e.g., add monitoring, update runbook, fix code)

---

## Incident Response Flow

```
Detection
   │
   ▼
Triage ──── P1 → Immediate containment
   │
   ├── P2 → Respond within 15 min
   │
   ├── P3 → Respond within 1 hour
   │
   └── P4 → Log for follow-up
   │
   ▼
Containment
   │
   ├── Disable MCP
   ├── Block Route
   └── Fallback
   │
   ▼
Resolution
   │
   ├── Fix (follow RECOVERY_PROCEDURES.md)
   ├── Validate
   └── Document
   │
   ▼
Postmortem (P1/P2 only)
   │
   ├── Root Cause
   ├── Impact
   └── Lessons Learned
```
