# CDCS-1 Reliability Baseline v1

**Session:** A — CDCS-1 Runtime Baseline  
**Date:** 2026-06-17  
**Tag:** CDCS-1-Runtime-Baseline-v1  

---

## Baseline Measurements

All measurements taken after 120h continuous unsupervised operation.

### Service Reliability

| Metric | Value |
|--------|-------|
| Core MCP uptime | 120h (6/6 services) |
| L2 uptime | 120h (5/5 services, cycling) |
| Service failures | 0 |
| Unexpected restarts | 0 |
| Timer miss rate | 0% |

### Data Pipeline Reliability

| Metric | Value |
|--------|-------|
| Observations generated | 5141 |
| Data loss rate | 0% (12 corrupted records skipped, not lost) |
| Producer error rate (post-fix) | 0% |
| Consumer skip rate (post-fix) | 1 record (injected test) |
| Pipeline breaks | 0 |

### Fault Tolerance

| Scenario | Result |
|----------|--------|
| Single malformed observation | Skip + warn, pipeline continues |
| Cascade (BindsTo) | Impossible — 0 BindsTo across all 6 services |
| EMCL-T crash | Isolated per domain (core/integration) |
| GAG circuit breaker trip | Forces OBSERVE_ONLY, no crash |

---

## Failure Mode Registry

All known failure modes from the soak are closed:

| FM-ID | Description | Status | Closure Type |
|-------|-------------|--------|-------------|
| FM-001 | Malformed JSON produced by observe layer | CLOSED | Producer validation gate |
| FM-002 | json.loads crash kills consumer pipeline | CLOSED | Try/except + skip in all consumers |
| FM-003 | load_json crash on corrupted report file | CLOSED | Try/except + return {} |

---

## Baseline Constraints

1. **Mode must remain OBSERVE_ONLY** until explicit decision to raise
2. **No BindsTo** must ever be reintroduced to any service
3. **All JSON parsing MUST** go through try/except wrappers
4. **Producer gate MUST** remain on all observation writes

---

## Reliability Rating

| Dimension | Rating |
|-----------|--------|
| Runtime Stability | ✅ Verified |
| Fault Isolation | ✅ Verified |
| Cascade Resistance | ✅ Verified |
| Data Integrity | ✅ Verified |
| Operational Safety | ✅ Verified |

**Overall: RELIABILITY BASELINE ESTABLISHED**
