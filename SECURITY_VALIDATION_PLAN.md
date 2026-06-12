# Phase C1 — Security Validation Plan

## Objective
Prove the system fails safely (Fail-Close), not just that it works. Validate enforcement boundaries, rate limits, trace safety, memory safety, MCP failure resilience, and adaptive routing governance.

## Constraint
**No New Runtime Components. No New Engines. No New MCP Servers. No Architecture Changes.** Only: Test, Measure, Prove, Document.

---

## Test Group 1 — Enforcement

Test gateway's ability to reject unauthorized/ malformed requests at each validation layer.

| Test | Target | Method | Expected | Existing? |
|------|--------|--------|----------|-----------|
| 1.1 Unknown MCP | v2 gateway | `Action.Type = "unknown"` | `VALIDATION_ERROR` | `TestGateway_InvalidAction` |
| 1.2 Unknown Operation | v2 gateway | `Action.Operation = "nonexistent"` | `ROUTE_NOT_FOUND` | `TestGateway_RouteNotFound` |
| 1.3 Missing Parameters | v2 gateway | Omit required payload fields | Server-specific error, no panic | Partial |
| 1.4 Malformed Request Body | v2 gateway | Non-JSON payload in `Process()` | Validation error (or graceful fail at schema level), no panic | Missing |
| 1.5 Unknown MCP Server | v2 gateway | Remove server from registry, route to it | `SERVER_NOT_FOUND` | Missing |
| 1.6 Mixed Valid/Invalid Params | v2 gateway | Valid + invalid parameters | Server rejects invalid, no panic | Missing |

**Pass Criteria**: All blocked. No panic. Audit generated. Trace recorded.

---

## Test Group 2 — Rate Limiting

Test the system's behavior under load. Note: true time-window rate limiting is NOT implemented — `RecordRateLimit()` counter exists in metrics but is never called. The backpressure model provides concurrent-request limiting, not per-time-window limits.

| Test | Target | Method | Expected |
|------|--------|--------|----------|
| 2.1 Burst Traffic | v2 gateway | 100 concurrent requests | No crash, backpressure may reject some |
| 2.2 Sustained Traffic | v2 gateway | 500 sequential requests | No crash, convergence increases |
| 2.3 Mixed Traffic | v2 gateway | Random operations | No crash, all operations served |

**Pass Criteria**: System remains stable. No panic. All requests complete (some may error). Response times remain bounded.

**Gap Documented**: `RecordRateLimit()` is defined in `runtime/metrics/registry.go:107` but never called. No time-window rate limiting exists.

---

## Test Group 3 — Trace Safety

Test that the DecisionTrace and audit system handle large inputs within safe bounds.

| Test | Target | Method | Expected |
|------|--------|--------|----------|
| 3.1 1KB Input | v2 gateway | Parameter with 1KB value | Trace records, no truncation needed |
| 3.2 10KB Input | v2 gateway | Parameter with 10KB value | Trace records, no explosion |
| 3.3 100KB Input | v2 gateway | Parameter with 100KB value | Trace records, field may exceed 512B |
| 3.4 1MB Input | v2 gateway | Parameter with 1MB value | System should not OOM, should fail gracefully |

**Gap Documented**: No explicit 512B field truncation in DecisionTrace. No explicit 64KB trace limit. The trace grows linearly with input size.

**Pass Criteria**: No crash. No OOM. Request either succeeds with truncated response or fails gracefully with error code.

---

## Test Group 4 — Memory Safety

Test system resilience against extreme payloads.

| Test | Target | Method | Expected |
|------|--------|--------|----------|
| 4.1 Huge Payload | v2 gateway | 10MB JSON payload as parameters | Graceful failure or timeout |
| 4.2 Invalid Payload | v2 gateway | Wrong types in parameter values | Server rejects, no panic |
| 4.3 Corrupted Payload | v2 gateway | Binary data in JSON string field | Server sanitizes or rejects, no panic |

**Pass Criteria**: Graceful failure. No panic. Error response returned.

---

## Test Group 5 — MCP Failure Simulation

Test that individual MCP server failures are isolated and do not cascade.

| Test | Target | Method | Expected |
|------|--------|--------|----------|
| 5.1 Filesystem Down | v2 gateway | Remove filesystem from registry, request filesystem op | `SERVER_NOT_FOUND`, no panic |
| 5.2 Git Failure | v2 gateway | Git command fails (invalid repo) | Error returned, no panic, trace recorded |
| 5.3 Fetch Timeout | v2 gateway | Request to unreachable URL | Timeout error, circuit breaker may open |
| 5.4 Memory Failure | v2 gateway | Invalid memory params | Error returned, no panic |
| 5.5 Context7 Failure | v2 gateway | Invalid context7 key | Error returned, no panic |
| 5.6 GitHub Failure | v2 gateway | Missing GITHUB_TOKEN | Error returned, no panic |

**Each test verifies**:
- `resp.Status == "error"` (no panic)
- `resp.Meta.DecisionTrace` non-nil (trace recorded)
- `LogAudit` output contains server+operation (audit recorded)
- Gateway continues serving other servers after failure (no cascade)

**Pass Criteria**: All servers fail independently. No panic. Metrics recorded. Trace recorded. Decision logged.

---

## Test Group 6 — Adaptive Routing Safety

**Critical test.** Prove that Knowledge Scoring's route override cannot escape Governance enforcement.

| Test | Target | Method | Expected |
|------|--------|--------|----------|
| 6.1 Override Catches Enforcement | v2 gateway | Knowledge biases toward `github`; enforcement blocks `github` operation | Request blocked (`ENFORCEMENT_BLOCKED`), not executed |
| 6.2 Override Caught by Policy Engine | v2 gateway | Knowledge biases toward server not in allow list | Request blocked (`POLICY_DENIED`) |
| 6.3 Enforcement vs Policy Priority | v2 gateway | Policy allows operation, enforcement blocks server | Enforcement blocks (Stage 5.5 > Stage 2) |
| 6.4 Trace Shows Both Override and Enforcement | v2 gateway | Knowledge override + enforcement block | Trace contains `override` step AND `enforcement:blocked` step |

**Proof**: Route Override ≠ Policy Bypass. Adaptive routing CANNOT escape governance because:
1. Policy Engine (Stage 2) checks before routing — catches disallowed action types
2. Enforcement Gate (Stage 5.5) checks after routing — catches blocked server+operation pairs
3. Both are in the critical path; neither can be bypassed by adaptive routing

**Pass Criteria**: All enforcement-blocked requests return `ENFORCEMENT_BLOCKED`. Trace shows both routing override and enforcement block. Execution never reaches the blocked server.

---

## Summary Table

| Group | Tests | Status | Gaps Found |
|-------|-------|--------|------------|
| G1 Enforcement | 6 tests | Existing coverage partial | Missing malformed body + unknown server tests |
| G2 Rate Limiting | 3 tests | Only backpressure exists | No time-window rate limiting; `RecordRateLimit()` defined but unused |
| G3 Trace Safety | 4 tests | No current truncation | No 512B field truncation; no 64KB trace limit; linear trace growth |
| G4 Memory Safety | 3 tests | Server-specific validation | No centralized payload size enforcement |
| G5 MCP Failure Simulation | 6 tests | Circuit breaker + retry exist | Need to verify isolation + audit per server |
| G6 Adaptive Routing Safety | 4 tests | Enforcement gate exists | Must prove Route Override ≠ Policy Bypass |

---

## Implementation

All tests in `runtime/mcp/v2/security_test.go` using existing `Gateway`, `EnforcementEngine`, `LearningEngine`, and server types. No new production code. No architectural changes.
