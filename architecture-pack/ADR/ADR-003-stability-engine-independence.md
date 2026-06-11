# ADR-003: Stability Engine Independence

**Status:** Accepted (v2.8)  
**Decided:** 2026-06-11  
**Scope:** Intelligence Plane

---

## Context

By v2.7, the system had adaptive routing (scoring + exploration) that could produce **oscillation** — rapid switching between two servers with similar scores. For example, `git` and `fetch` both support `status`, and exploration could cause the system to alternate on every request (`git → fetch → git → fetch`).

This oscillation had negative effects:
- Unpredictable execution latency
- Confusing audit logs
- Reduced user trust in the system

Two approaches were considered:

1. **Integrate stability into the scoring function** — add oscillation penalty directly to `scoreCapability()`
2. **Independent Stability Engine** — a separate component that adjusts scores after exploration but before selection

Approach 1 was rejected because:
- It couples stability concerns with capability scoring, violating separation of concerns
- It makes scoring non-deterministic (oscillation state would affect raw capability scores)
- It complicates unit testing of scoring logic

## Decision

Create an independent **Stability Engine** that operates as a post-exploration adjustment layer:

- **Exploration decay**: per-server `baseRate * exp(-decayRate * usageCount)` with a 1% floor
- **Oscillation detection**: tracks alternating patterns in a 20-entry sliding window per operation
- **Convergence scoring**: measures how dominant a single server is in the window
- **Stability bias**: slowly accumulates (+0.01 per event) for consistently selected servers when convergence > 0.5

The final score becomes: `baseScore + explorationAdjustment - oscillationPenalty + stabilityBias`

## Consequences

### Positive
- Stability concerns are isolated in a single component
- Scoring remains pure (same knowledge + same weights → same score)
- Oscillation is penalised without modifying the routing algorithm
- Exploration naturally decays as servers prove reliable

### Negative
- Adds a processing step (sub-millisecond, non-blocking)
- Stability bias can make the system slow to adapt to genuinely better alternatives

### Neutral
- The Stability Engine influences **selection** but never **enforcement**
- It operates in the Intelligence Plane, not the Control Plane

## Architectural Principle

> The Stability Engine influences decisions but never enforces them. It prevents oscillation without blocking execution.
