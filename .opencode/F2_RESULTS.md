# F2 — Failure Mode Simulation Results

**Date:** 2026-06-12T14:39:00Z
**Phase:** F (Runtime Stabilization & Governance)

## Compliance Score: 85/100 — "Stable with Minor Coupling Risk"

## Phase 1: Single-Domain Injection

### 1.1 GitHub Token Failure — PASS
- Server stays active with degraded auth (no crash on invalid token)
- No restart loop
- Core services unaffected
- Injection method: replace mcp-tokens.env with invalid values

### 1.3 Core Crash Loop — PARTIAL PASS
- 12 SIGKILLs injected into mcp-filesystem
- Systemd debounces: only 3 NRestarts for 12 kills
- Burst cap holds (≤10/60s)
- **Gap:** BindsTo cascade stops git → fetch → memory
- Recovery: manual restart required

## Phase 2: Cross-Domain Stress — PASS with known gap
- Token failure (integration) + filesystem crash (core) simultaneously
- Integration → Core isolation: STRONG (chromadb/github unaffected by core chaos, core unaffected by integration degradation)
- Core → Core cascade: same BindsTo gap
- Recovery: all services restored

## Phase 3: Adversarial Cascade Attempt — PASS with known gap
- Triple injection: invalid tokens + SIGKILL burst + all domains stressed
- git entered FAILED state (BindsTo prevented restart while filesystem unstable)
- No cross-domain propagation
- Recovery: `reset-failed` + `restart` restores all

## Architectural Findings

### Isolated (PASS)
| Boundary | Result |
|---|---|
| Integration → Core | ✅ No propagation |
| Token failure → system crash | ✅ No cascade |
| External degradation → runtime | ✅ Contained |

### Not Isolated (GAP)
| Path | Why | Impact |
|---|---|---|
| filesystem crash → git stop | BindsTo=mcp-filesystem.service | Manual recovery needed for entire chain |

## Recovery Characteristics
- Post-stress latency: 3-15ms (back to baseline)
- Token restore + restart: full recovery in <5s
- Core throughput unaffected after stabilization

## Recommendation
Accept BindsTo cascade as known behavior (not a bug — intentional dependency). Document as manual recovery step in runbooks. The gap is within Core domain only; Integration→Core containment is architecturally sound.
