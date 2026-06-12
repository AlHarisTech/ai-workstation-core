# Phase C2 — Hardening Closure Plan

## Objective
Close 3 implementation gaps identified in Phase C1 Security Validation. No architecture changes. Minimal diff.

## Gap Summary

| ID | Gap | Severity | Type | 
|----|-----|----------|------|
| H1 | Panic Recovery Missing | P0 | Implementation |
| H2 | `log.Fatalf` in `Listen()` | P1 | Implementation |
| H3 | Rate Limiter Reality Audit | P2 | Documentation |

---

## H1 — Panic Containment

### Current State
- `RecordPanic()` exists in `runtime/metrics/registry.go:109` — never called
- `recover()` absent from all `Process()`, `Execute()`, and goroutine paths

### Changes

**1. `runtime/mcp/v2/gateway.go` — `Process()` line 98**

Add `recover()` to the existing deferred function. Format:
```go
defer func() {
    if r := recover(); r != nil {
        resp.Status = "error"
        resp.Error = ErrorInfo{Code: "INTERNAL_PANIC", Message: fmt.Sprintf("panic: %v", r)}
        trace.Steps = append(trace.Steps, TraceStep{Stage: "panic", Output: "recovered", Meta: map[string]any{"panic": fmt.Sprintf("%v", r)}})
        log.Printf("[gateway] PANIC RECOVERED: %v", r)
    }
    // ... existing audit, learning, trace code
}()
```

**2. `runtime/mcp/contracts/adapter.go` — `TimeoutAdapter.Execute()` line 102**

Add `recover()` to the goroutine:
```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            ch <- result{
                resp: types.MCPResponse{ID: req.ID, Success: false, Error: fmt.Sprintf("panic: %v", r)},
                err:  fmt.Errorf("panic in %s: %v", ta.inner.Name(), r),
            }
        }
    }()
    r, e := ta.inner.Execute(ctx, req)
    ch <- result{r, e}
}()
```

### Scope
- `Gateway.Process()` — catches panics from all server Execute() calls + gateway logic
- `TimeoutAdapter` goroutine — catches panics in the separate goroutine (critical: would otherwise crash process)

### Not Needed
- Individual server `Execute()` methods — all called directly from `Process()` in the same goroutine, covered by `Process()`'s recover
- `RetryAdapter` — calls `inner.Execute()` synchronously, covered by TimeoutAdapter or Process()

### Test
Verify `INTERNAL_PANIC` error code returned on panic. Verify `[gateway] PANIC RECOVERED` in logs.

---

## H2 — Fatal Exit Removal

### Current State
`Listen()` at `v2/gateway.go:419` calls `log.Fatalf` on malformed stdin, exiting the process.

### Change
Replace `log.Fatalf` with graceful error return:
```go
func (g *Gateway) Listen() (*MCPRequest, error) {
    var req MCPRequest
    err := json.NewDecoder(os.Stdin).Decode(&req)
    if err == io.EOF {
        return nil, nil
    }
    if err != nil {
        return nil, fmt.Errorf("failed to read request: %w", err)
    }
    return &req, nil
}
```

Callers must handle the error instead of expecting process exit.

---

## H3 — Rate Limiter Reality Audit

### Question
Does Stage 0 Rate Limiter exist in the codebase, or is it documented only?

### Investigation
- Search for rate limiting enforcement logic (time-window, per-user quotas, burst control)
- Verify `RecordRateLimit()` caller chain
- Check if rate limiting exists in any form (backpressure is NOT rate limiting)

### Outcome
Either:
- **FOUND**: Document the proof with file paths and test results
- **NOT FOUND**: Document the gap for Phase D

---

## Success Criteria

| Criterion | Verification |
|-----------|-------------|
| H1: Process recovers from panic | Test injects panic → returns `INTERNAL_PANIC`, trace steps, log output |
| H1: Goroutine recovers from panic | TimeoutAdapter goroutine returns error instead of crashing |
| H2: No `log.Fatalf` in `Listen()` | `Listen()` returns `(*MCPRequest, error)` — caller handles error |
| H3: Rate limiter status known | Documented finding: exists or gap acknowledged |
| All existing tests pass | `go test ./runtime/mcp/v2/ -count=1` |
