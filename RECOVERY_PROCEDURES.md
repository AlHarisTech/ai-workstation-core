# Recovery Procedures — MCP Runtime v3.1.1-stable

## Severity Classification

| Level | Label | Example | Response |
|-------|-------|---------|----------|
| P1 | Critical | Multiple MCPs down, data loss, security breach | Immediate response, engage all available operators |
| P2 | Major | Single local MCP down, rate limiter misconfigured | Respond within 15 minutes, rollback if needed |
| P3 | Minor | Remote MCP unreachable, single request failure | Respond within 1 hour, investigate root cause |
| P4 | Informational | Metrics anomaly, trace inconsistency | Log for follow-up, no immediate action |

---

## Runtime Recovery

### Host Application Crash

**Severity:** P1

**Symptoms:** An MCP process exits unexpectedly; systemd shows `inactive` or `failed` for a specific service.

**Detection:**

```bash
# Identify which service failed
systemctl --user list-units --failed
systemctl --user status 'mcp-*.service' | grep -E "●|Active"
```

**Procedure:**

```bash
# 1. Check status of the failing service (example: filesystem — root of cascade)
systemctl --user status mcp-filesystem.service

# 2. View recent logs for crash reason
journalctl --user -u mcp-filesystem.service -n 50 --no-pager

# 3. Check for panic or fatal error
journalctl --user -u mcp-filesystem.service --no-pager | grep -E "panic|fatal|error|FATAL"

# 4. Restart the service
systemctl --user restart mcp-filesystem.service

# 5. Verify cascade health (dependent services restart automatically via BindsTo)
systemctl --user is-active mcp-filesystem.service mcp-git.service mcp-fetch.service mcp-memory.service
```

**Expected recovery time:** < 5 seconds (systemd `RestartSec=2` for most services).

**Rollback criteria:**
- If same service fails 3 consecutive restarts within 60 seconds → **do not auto-restart**
- If crash is caused by a recent code change → rollback to previous release
- If crash is caused by environment (disk full, OOM) → resolve resource issue first, then restart

**Prevention:**
- Panics are caught by `recover()` in `Gateway.Process()` — process should not crash from request handling
- `Listen()` uses graceful error handling (no `log.Fatalf`) — malformed input should not cause crash
- If crash persists, it indicates a code-level defect (e.g., nil dereference outside Process), not a runtime condition

### Panic Event

**Severity:** P2 (P1 if persistent across multiple operations)

**Symptoms:** Metrics show `Panics` counter > 0; log contains `PANIC RECOVERED`; requests return `INTERNAL_PANIC` error code.

**Detection:**

```go
snap := metrics.Global().Snapshot()
if snap.Gateway.Panics > 0 {
    // Investigate
}
```

**Procedure:**

```bash
# 1. Extract panic details from logs of the affected service
journalctl --user -u mcp-filesystem.service --no-pager | grep "PANIC RECOVERED"
```

```go
// 2. Check which requests are failing with panic
for _, s := range resp.Meta.DecisionTrace.Steps {
    if s.Stage == "panic" {
        // s.Meta["panic"] contains original panic value
        // s.Output is "recovered"
    }
}
```

```bash
# 3. If panics persist on the same service, restart it
systemctl --user restart mcp-filesystem.service
```

**Rollback criteria:**
- If panics began after a code deployment → rollback to previous release
- If panics are environment-specific (e.g., only on certain inputs) → fix input or add guard
- If `Panics` counter exceeds 10 in one hour → escalate to P1

### Deadlock

**Severity:** P1

**Symptoms:** Requests hang indefinitely; no response; no error in logs; systemd shows `active` but not responding.

**Detection:**

```bash
# Check for stuck processes (state D or长时间 non-responsive)
ps aux | grep mcp | grep -v grep

# Check if port is open but not responding
timeout 3 curl -s http://localhost:4110/health || echo "NOT RESPONDING"
```

**Procedure:**

```bash
# 1. Force kill the stuck service
systemctl --user kill -s SIGKILL mcp-filesystem.service

# 2. Verify it stopped
systemctl --user is-active mcp-filesystem.service

# 3. Restart
systemctl --user restart mcp-filesystem.service

# 4. Check cascade — dependent services restart automatically via BindsTo
systemctl --user status mcp-git.service mcp-fetch.service mcp-memory.service
```

**Rollback criteria:**
- If deadlock reproduces on restart → rollback to previous release immediately
- If deadlock correlates with high concurrency → check TimeoutAdapter configuration
- Permanent fix requires root cause analysis (goroutine dump, mutex analysis)

**Root cause investigation:**
- Likely a goroutine blocked on channel, mutex, or network I/O
- Check for missing `context.WithTimeout` in custom server implementations
- The `TimeoutAdapter` wraps all MCP calls with configurable timeout — verify it is used

### High Memory Usage

**Severity:** P2 (P1 if cascading to OOM kills)

**Threshold:** Memory > 512MB RSS for a single MCP process.

**Detection:**

```bash
ps -o pid,rss,comm -p $(pgrep -f "mcp-")
```

**Procedure:**

```bash
# 1. Restart the leaking service
systemctl --user restart mcp-filesystem.service

# 2. Monitor memory after restart
sleep 10 && ps -o pid,rss,comm -p $(pgrep -f "mcp-")
```

**Rollback criteria:**
- If memory grows to > 512MB within 5 minutes of restart → rollback to previous release
- If memory growth is gradual but relentless → investigate for goroutine leak

**If memory grows again:**
- Check for unbounded result sets in server responses
- Check for goroutine leak (open connections not closed)
- Review `TimeoutAdapter` configuration — long timeouts can accumulate goroutines

---

## Runtime Component Failures

These are logical components of the Gateway pipeline (not separate processes). They cannot "crash" independently, but can enter misconfigured or degraded states.

### Rate Limiter Misconfiguration

**Severity:** P2

**Symptoms:** Legitimate traffic returning `RATE_LIMITED`; or no rate limiting observed despite high request volume.

**Detection:**

```go
snap := metrics.Global().Snapshot()
// If rate limited ratio > 1% of total requests, investigate
rateRatio := float64(snap.Gateway.RateLimited) / float64(snap.Gateway.RequestsTotal)
if rateRatio > 0.01 {
    // Rate limiting may be too aggressive
}
```

**Recovery:**

```go
// Adjust token bucket parameters in NewGateway()
// runtime/mcp/v2/gateway.go
rateLimiter: NewTokenBucket(20000, 10000),  // increase burst & refill
```

```bash
# Restart the affected service after config change
systemctl --user restart mcp-filesystem.service
```

**Rollback:** Undo the parameter change and revert to previous values. Default: burst=10000, refill=5000/sec.

### Enforcement Engine Blocking

**Severity:** P2 (P3 if single operation)

**Symptoms:** Requests denied with `ENFORCEMENT_BLOCKED` unexpectedly.

**Detection:**

```go
for _, s := range resp.Meta.DecisionTrace.Steps {
    if s.Stage == "enforcement" && s.Output == "blocked" {
        // s.Meta["reason"] explains the block
        // s.Meta["server"] shows which server was blocked
    }
}
```

**Recovery:** Update enforcement rules in `EnforcementEngine` configuration. No restart required — rules are evaluated at runtime.

**Rollback:** Remove or revert the enforcement rule that triggered the block.

### Router Resolution Failure

**Severity:** P3

**Symptoms:** Requests return `ROUTE_NOT_FOUND` for operations that previously worked.

**Detection:**

```go
for _, s := range resp.Meta.DecisionTrace.Steps {
    if s.Stage == "resolve" && s.Output == "not_found" {
        // s.Meta["error"] contains resolution failure reason
    }
}
```

**Recovery:** Verify the capability-to-server mapping in `Router`. If a server was removed or renamed, update the routing table.

**Rollback:** Restore the previous capability mapping.

### Knowledge Engine Degraded

**Severity:** P3 (non-blocking)

**Symptoms:** Trace shows `knowledge:failed` instead of `knowledge:<N>_docs`; ChromaDB connectivity issue.

**Detection:**

```go
for _, s := range resp.Meta.DecisionTrace.Steps {
    if s.Stage == "knowledge" && s.Output == "failed" {
        // s.Meta["error"] explains the failure
    }
}
```

**Recovery:** Check ChromaDB service connectivity. Knowledge retrieval is non-blocking — routing continues without it.

**Rollback:** No rollback needed. The system operates correctly without knowledge (fallback to default scoring).

### Learning Engine Stall

**Severity:** P4

**Symptoms:** Server weights not updating; learning snapshots show no change over time.

**Detection:** Compare `snap.Learning.Updates` over two time intervals. If no change, the engine may be stalled.

**Recovery:** This is self-correcting — new requests with outcomes will eventually update weights. If permanently stalled, check `LearningEngine` initialization.

**Rollback:** No rollback needed. Learning is additive and non-critical for correctness.

---

## MCP Recovery

### Filesystem Down

**Severity:** P2

**Symptoms:** Requests to filesystem operations return `EXECUTION_FAILED`; port 4110 unreachable.

**Detection:**

```bash
curl -s http://localhost:4110/health || echo "FILESYSTEM DOWN"
```

**Procedure:**

```bash
# 1. Check systemd status
systemctl --user status mcp-filesystem.service

# 2. Restart
systemctl --user restart mcp-filesystem.service

# 3. Verify
curl -s http://localhost:4110/health
```

**Rollback:** If restart fails 3 times, check journal for permission errors or path configuration. Filesystem is the cascade root — if it stays down, all dependent services (git, fetch, memory) will also stop via `BindsTo`.

### Git Down

**Severity:** P2

**Symptoms:** Git operations return `EXECUTION_FAILED`; port 4111 unreachable.

**Detection:**

```bash
curl -s http://localhost:4111/health || echo "GIT DOWN"
```

**Procedure:**

```bash
# 1. Check systemd
systemctl --user status mcp-git.service

# 2. Restart
systemctl --user restart mcp-git.service

# 3. Verify
curl -s http://localhost:4111/health
```

**Rollback:** If git binary not in PATH → fix PATH in unit file and restart. If repo path invalid → fix parameters in caller.

### Fetch Down

**Severity:** P2

**Symptoms:** HTTP fetch operations return `EXECUTION_FAILED`; port 4112 unreachable; timeout after 30s.

**Detection:**

```bash
curl -s http://localhost:4112/health || echo "FETCH DOWN"
```

**Procedure:**

```bash
# 1. Check systemd
systemctl --user status mcp-fetch.service

# 2. Restart
systemctl --user restart mcp-fetch.service

# 3. Verify
curl -s http://localhost:4112/health
```

**Rollback:** If timeouts are caused by slow targets, no rollback needed — timeout is per-request. If all fetches timeout, check network connectivity.

### Memory Down

**Severity:** P2

**Symptoms:** Memory/knowledge operations return `EXECUTION_FAILED`; port 4113 unreachable.

**Detection:**

```bash
curl -s http://localhost:4113/health || echo "MEMORY DOWN"
```

**Procedure:**

```bash
# 1. Check systemd
systemctl --user status mcp-memory.service

# 2. Restart
systemctl --user restart mcp-memory.service

# 3. Verify
curl -s http://localhost:4113/health
```

**Rollback:** Restart causes in-memory data loss. Current implementation (`NewMemoryServer("")`) does not support persistence. For future persistent mode: backup database file before restart, restore after.

**Data persistence note:**
- Current (`NewMemoryServer("")`): in-memory only, no backup required, all data lost on restart
- Future persistent mode (`NewMemoryServer("/path/to/db")`): requires `cp` backup before restart, restore from backup after

### Context7 Down

**Severity:** P3

**Symptoms:** Context7 operations return `EXECUTION_FAILED` with DNS or connection error.

**Detection:**

```go
resp.Error.Code == "EXECUTION_FAILED"
resp.Error.Message contains "context7 service unreachable"
```

**Procedure:**

```bash
# 1. Check DNS resolution
nslookup api.context7.com

# 2. Check network connectivity
curl -s https://api.context7.com/v1/context

# 3. If API key issue, verify environment
echo $CONTEXT7_API_KEY
```

**Rollback:** Context7 is a third-party dependency — no rollback available. Fail-close: requests return error code without cascading.

### GitHub Failure

**Severity:** P3

**Symptoms:** GitHub operations return `EXECUTION_FAILED` with 401 (no token) or connection error.

**Detection:**

```go
resp.Error.Code == "EXECUTION_FAILED"
resp.Error.Message contains "Bad credentials"
```

**Procedure:**

```bash
# 1. Check if token is set
echo $GITHUB_TOKEN

# 2. Verify token has required permissions (repo, read:org minimum)

# 3. Set token and restart
export GITHUB_TOKEN="your_token_here"
systemctl --user restart mcp-github.service
```

**Rollback:** Revert to previous token if new token is invalid. If no token was ever set, this is a known limitation (GitHub MCP cannot perform authenticated operations without `GITHUB_TOKEN`).

---

## Data Recovery

### Trace Recovery

Decision traces are in-memory only. To preserve traces for debugging:

**Option 1 — Log-based capture:**

```go
if resp.Meta.DecisionTrace != nil {
    traceJSON, _ := json.Marshal(resp.Meta.DecisionTrace)
    log.Printf("[trace] %s", string(traceJSON))
}
```

**Option 2 — Audit log:**

```go
LogAudit(AuditRecord{
    Timestamp:  time.Now().UTC().Format(time.RFC3339),
    RequestID:  req.ID,
    TraceID:    req.Meta.TraceID,
    Action:     string(req.Action.Type) + "." + req.Action.Operation,
    Server:     auditServer,
    Status:     resp.Status,
    DurationMs: time.Since(start).Milliseconds(),
})
```

Pipe stdout to a file or external log collector for persistence.

### Metrics Recovery

Metrics are in-memory only (`metrics.Global()`). Restart resets all counters.

**To snapshot before restart:**

```go
snap := metrics.Global().Snapshot()
snapJSON, _ := json.MarshalIndent(snap, "", "  ")
os.WriteFile("metrics_snapshot.json", snapJSON, 0644)
```

**Restore behavior:** Metrics start at zero on restart. This is by design — SLO window resets.

### Configuration Recovery

Configuration is stored in:

| Location | Purpose | Recovery |
|----------|---------|----------|
| `~/.config/systemd/user/mcp-*.service` | systemd unit files | Recreate from templates in `deployment/systemd/` or `RUNBOOKS.md` |
| `~/.config/opencode/opencode.jsonc` | OpenCode MCP server list | Restore from backup or re-run `opencode mcp add` |
| `runtime/mcp/v2/gateway.go` | Registered server list | Restore from version control (`git checkout main`) |
| Environment variables | GitHub token, API keys | Set from secrets manager |

**Systemd unit templates:** The canonical templates are documented in `RUNBOOKS.md` (section: Add MCP). Copy the template and adjust paths for the specific service.

**Recovery procedure for complete config loss:**

```bash
# 1. Recreate systemd units from documented templates
# See RUNBOOKS.md → MCP Operations → Add MCP for unit file templates

# 2. Reload systemd
systemctl --user daemon-reload

# 3. Recreate opencode config
opencode mcp add filesystem --transport http http://localhost:4110
opencode mcp add git --transport http http://localhost:4111
opencode mcp add fetch --transport http http://localhost:4112
opencode mcp add memory --transport http http://localhost:4113

# 4. Set environment secrets
export GITHUB_TOKEN="<from-secrets-manager>"
export CONTEXT7_API_KEY="<from-secrets-manager>"

# 5. Start services
systemctl --user start mcp-*.service
```

---

## Recovery Verification Checklist

After any recovery action, run this checklist:

```markdown
## Local MCP Services
[ ] All 4 local MCP processes active (systemctl --user is-active mcp-filesystem.service mcp-git.service mcp-fetch.service mcp-memory.service)
[ ] All 4 local ports respond (curl -s http://localhost:411{0,1,2,3}/health)
[ ] BindsTo cascade intact (all dependent services restarted with root)

## Remote MCP Connectivity
[ ] opencode mcp list shows 8 servers connected (4 local + 4 remote)
[ ] Remote providers reachable (GitHub, Context7, ChromaDB, Supabase)

## Functional Verification
[ ] Single test request succeeds (filesystem.read /tmp)
[ ] Metrics dashboard renders with non-zero uptime
[ ] Error rate is below 5%
[ ] Panic count is 0 (or stable if pre-existing)
[ ] Rate limit counter is not saturated
```
