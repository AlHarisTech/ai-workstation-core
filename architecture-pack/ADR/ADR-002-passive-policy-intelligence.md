# ADR-002: Passive Policy Intelligence

**Status:** Accepted (v3.1)  
**Decided:** 2026-06-11  
**Scope:** Observability Plane

---

## Context

After introducing the Enforcement Gate (v3.0), the system had a control point but no feedback about how it was being used. Questions arose:

- Which `(server, operation)` pairs are being blocked most frequently?
- Is enforcement behaviour drifting over time?
- Should policy rules be adjusted based on observed patterns?

Two approaches were considered:

1. **Active feedback loop** — enforcement outcomes modify policy rules automatically
2. **Passive observation** — enforcement outcomes are recorded and analysed, but never fed back into the decision or enforcement pipeline

Approach 1 was rejected because:
- It creates a feedback loop that can destabilise enforcement
- It violates the separation between control and observation
- It makes enforcement behaviour non-deterministic over time

## Decision

Implement **Policy Intelligence** as a strictly passive observability layer:

- Records every `PolicyEvent` (TraceID, Server, Operation, Allowed, Blocked, Reason)
- Maintains per-server+operation weights (+0.01 per allow, -0.02 per block)
- Detects drift patterns (≥3 blocks in last 10 events for the same key)
- Generates non-binding suggestions (e.g., `review_policy` with confidence score)

All data structures are **internal only** — no exposed API can trigger enforcement changes.

## Consequences

### Positive
- Enforcement history is fully recorded and analysable
- Drift detection provides early warning without automated action
- Suggestions can inform human administrators without risk of cascading failures

### Negative
- No automatic policy adjustment — requires manual intervention for rule changes
- Storage grows linearly with request volume (in-memory only; no persistence in v3.1)

### Neutral
- Suggestions are informational only; they have zero influence on routing, scoring, or enforcement

## Architectural Principle

> Policy Intelligence never influences decisions. It is a read-only observer of enforcement outcomes.
