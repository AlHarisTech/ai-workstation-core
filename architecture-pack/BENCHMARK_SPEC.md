# MCP Runtime — Benchmark Specification

**Version:** v3.1.1-stable  
**Status:** Reference Specification (not implemented)

---

## 1. Latency Budgets (per-request, p99)

| Stage | Budget | Measured (v3.1) | Notes |
|-------|--------|-----------------|-------|
| Stage 0: Rate Limit | ≤ 20µs | — | Token bucket check (pre-gateway) |
| Stage 1: Validate | ≤ 50µs | — | Request parsing + schema check |
| Stage 2: Policy | ≤ 50µs | — | ACL lookup (Stage 2, pre-v3.0) |
| Stage 3: Resolve | ≤ 100µs | — | Server capability matching |
| Stage 4: Knowledge | ≤ 500ms | — | ChromaDB query (network-bound) |
| Stage 4.5: Score + Select | ≤ 200µs | — | Scoring + exploration + stability |
| Stage 5: Route | ≤ 50µs | — | Candidate-to-server binding |
| Stage 5.5: Enforcement | ≤ 50µs | — | Rule lookup + check |
| Stage 6: Execute | ≤ 5s | — | MCP server execution (external) |
| Stage 7: Learn + Governance | ≤ 100µs | — | Weight update + audit write |
| Stage 8: Normalize | ≤ 50µs | — | Response formatting + trace attach |

**Total system overhead (Stages 1–5 + 7–8, excluding execute):** ≤ 1ms p99

---

## 2. Decision Throughput SLA

| Metric | Target | Degradation Threshold | Critical |
|--------|--------|----------------------|----------|
| Decisions/sec (single instance) | ≥ 500 | < 200 | < 50 |
| Concurrent requests | ≥ 50 | < 20 | < 5 |
| Decision latency p50 | ≤ 500µs | > 1ms | > 5ms |
| Decision latency p99 | ≤ 1ms | > 5ms | > 10ms |

---

## 3. Stress Thresholds

| Parameter | Value | Behaviour at Threshold |
|-----------|-------|----------------------|
| Max in-flight requests | 200 | New requests queued with backpressure |
| Max exploration rate | 50% (cap) | ExplorationRate capped at 50% regardless of configuration |
| Knowledge timeout | 3s | Fallback to empty `{}` context |
| Execution timeout | 30s | Stage 6 forced abort |
| Concurrent ChromaDB queries | 10 | Round-robin throttling |

---

## 4. Enforcement Under Load

- **Fail-close during uncertainty**: if enforcement check exceeds 100ms or returns ambiguous, treat as `Block`
- **Policy Intelligence recording**: non-blocking, dropped if write queue exceeds 1000 events
- **DecisionTrace size cap**: 128 steps per trace; overflow truncates oldest
- **TraceStep size cap**: 512 bytes per Input/Output (soft limit); total trace ≤ 64 KB (soft limit)

---

## 5. Convergence Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Oscillation frequency | < 5% of requests | Alternating pattern in convergence window |
| Stability bias accumulation | 0.01 per request at convergence > 0.5 | `StabilityMetrics` snapshot |
| Exploration floor | 1% of base rate | `EffectiveRate()` calculation |

---

## 6. Test Requirements

- **Functional**: 60 tests (v2–v3.1), all pass
- **Stress minimum**: 1000 sequential valid requests with no enforcement blocks
- **Stability minimum**: 3 consecutive oscillation-free convergence windows under randomised load
- **Enforcement coverage**: every rule type (Allow, Deny, Audit) exercised at least once
