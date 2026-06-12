# Recovery Procedures — MCP Runtime v3.1.1-stable

## Runtime Recovery

### Gateway Crash

**Symptoms:** Process exits unexpectedly; no response to requests; systemd shows `inactive` or `failed`.

**Procedure:**

```bash
# 1. Check service status
systemctl --user status mcp-git.service

# 2. View recent logs for crash reason
journalctl --user -u mcp-git.service -n 50 --no-pager

# 3. Check for panic or fatal error
journalctl --user -u mcp-git.service --no-pager | grep -E "panic|fatal|error|FATAL"

# 4. Restart the service
systemctl --user restart mcp-git.service

# 5. Verify health
systemctl --user is-active mcp-git.service
curl -s http://localhost:4111/health
```

**Expected recovery time:** < 5 seconds (systemd `RestartSec=5`).

**Prevention:**
- Panics are caught by `recover()` in `Gateway.Process()` — process should not crash from request handling
- `Listen()` uses graceful error handling (no `log.Fatalf`) — malformed input should not cause crash
- If crash persists, it indicates a code-level defect (e.g., nil dereference outside Process), not a runtime condition

### Panic Event

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
# 1. Extract panic details from logs
journalctl --user -u mcp-git.service --no-pager | grep "PANIC RECOVERED"
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
# 3. If panics persist, restart the affected service
systemctl --user restart mcp-git.service
```

**Escalation:** Multiple panics on different operations → **P1** — indicates systemic issue (e.g., memory corruption, data race).

### Deadlock

**Symptoms:** Requests hang indefinitely; no response; no error in logs; systemd shows `active` but not responding.

**Procedure:**

```bash
# 1. Check for stuck processes
ps aux | grep mcp

# 2. Force restart
systemctl --user kill -s SIGKILL mcp-git.service
systemctl --user restart mcp-git.service
```

**Root cause investigation:**
- Likely a goroutine blocked on channel, mutex, or network I/O
- Check for missing `context.WithTimeout` in custom server implementations
- The `TimeoutAdapter` wraps all MCP calls with configurable timeout — verify it is used

### High Memory Usage

**Threshold:** Memory > 512MB for a single MCP process.

**Detection:**

```bash
ps -o pid,rss,comm -p $(pgrep -f "mcp-")
```

**Procedure:**

```bash
# 1. Restart the leaking service
systemctl --user restart mcp-filesystem.service  # or the affected one

# 2. Monitor memory after restart
sleep 10 && ps -o pid,rss,comm -p $(pgrep -f "mcp-")
```

**If memory grows again:**
- Check for unbounded result sets in server responses
- Check for goroutine leak (open connections not closed)
- Review `TimeoutAdapter` configuration — long timeouts can accumulate goroutines

---

## MCP Recovery

### Filesystem Down

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

**If restart fails:**
- Check `journalctl --user -u mcp-filesystem.service -n 50` for permission errors
- Verify the working directory exists and is readable
- Filesystem path restrictions are enforced in `ValidateRequest` — check path is within allowed roots

### Git Down

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

**Common failure modes:**
- Repository not found at specified path → verify path in request parameters
- Git binary not in PATH → check `ExecStart` in unit file

### Fetch Down

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

**If timeouts persist:**
- Check network connectivity to target URLs
- The `FetchServer` has a 30-second internal timeout — very slow targets may need increased timeout
- DNS resolution failures are the most common cause (confirmed during testing)

### Memory Down

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

**Data persistence:** `NewMemoryServer("")` uses in-memory storage. Restart causes data loss. For persistent storage, provide a file path to the constructor.

### Context7 Down

**Symptoms:** Context7 operations return `EXECUTION_FAILED` with DNS or connection error.

**Detection:**

```go
// Request returns error code EXECUTION_FAILED
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
echo $CONTEXT7_API_KEY  # Should be set in the environment
```

**Escalation:** Context7 is a remote provider — outage may be at their end. Verify at their status page.

### GitHub Failure

**Symptoms:** GitHub operations return `EXECUTION_FAILED` with 401 (no token) or connection error.

**Detection:**

```go
resp.Error.Code == "EXECUTION_FAILED"
resp.Error.Message contains "Bad credentials"  // No GITHUB_TOKEN
```

**Procedure:**

```bash
# 1. Check if token is set
echo $GITHUB_TOKEN

# 2. Verify token has required permissions
# Token needs repo, read:org scopes minimum

# 3. Restart service with token
export GITHUB_TOKEN="your_token_here"
systemctl --user restart mcp-github.service  # if applicable
```

**Known limitation:** Authenticated operations require `GITHUB_TOKEN`. Without it, GitHub MCP returns 401 for any API call.

---

## Data Recovery

### Trace Recovery

Decision traces are in-memory only. To preserve traces for debugging:

**Option 1 — Log-based capture:**

```go
// In your application code
if resp.Meta.DecisionTrace != nil {
    traceJSON, _ := json.Marshal(resp.Meta.DecisionTrace)
    log.Printf("[trace] %s", string(traceJSON))
}
```

**Option 2 — Audit log:**

Every request produces an audit record via `LogAudit()`:
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

To persist audit records, pipe stdout to a file or external log collector.

### Metrics Recovery

Metrics are in-memory only (`metrics.Global()`). Restart resets all counters.

**To snapshot metrics before restart:**

```go
snap := metrics.Global().Snapshot()
snapJSON, _ := json.MarshalIndent(snap, "", "  ")
os.WriteFile("metrics_snapshot.json", snapJSON, 0644)
```

**To restore after restart:** Metrics start at zero. There is no restore mechanism — this is intended (SLO window resets).

For long-term metrics, pipe the dashboard output:

```go
// Periodic snapshot
dash := metrics.NewCLIDashboard(nil)
log.Printf("[metrics]\n%s", dash.Render())
```

### Configuration Recovery

Configuration is stored in:

| Location | Purpose | Recovery |
|----------|---------|----------|
| `~/.config/systemd/user/mcp-*.service` | systemd unit files | Recreate from template in RUNBOOKS.md |
| `~/.config/opencode/opencode.jsonc` | OpenCode MCP server list | Restore from backup or re-run `opencode mcp add` |
| `runtime/mcp/v2/gateway.go` | Registered server list | Restore from version control |
| `runtime/mcp/v2/gateway.go` (NewGateway) | Rate limiter defaults | Restore from version control |
| Environment variables | GitHub token, API keys | Set from secrets manager |

**Recovery procedure for complete config loss:**

```bash
# 1. Restore systemd units from version control
git checkout main -- ~/.config/systemd/user/

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
[ ] All 4 local systemd services are active
[ ] All 4 local ports respond (4110-4113)
[ ] opencode mcp list shows 8 servers connected
[ ] Single test request succeeds (filesystem.read /tmp)
[ ] Metrics dashboard renders with non-zero uptime
[ ] Error rate is below 5%
[ ] Panic count is 0 (or stable if pre-existing)
[ ] Rate limit counter is not saturated
```
