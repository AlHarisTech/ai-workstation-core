# ADR-001: Enforcement Gate Isolation

**Status:** Accepted (v3.0)  
**Decided:** 2026-06-11  
**Scope:** Control Plane

---

## Context

Before v3.0, the system had a single `PolicyEngine` that ran at Stage 2 (pre-resolve). This policy checked access control lists (allow/deny by action type and operation) but had no awareness of the **selected server**. Once the system gained adaptive routing (v2.5–v2.7), it became possible for the scoring engine to select a server that should not be allowed for a specific operation, even though the action type was permitted.

Key observations:

- The existing policy layer ran **before** server selection and could not block based on the final `(server, operation)` pair
- Adaptive routing could override the default server (v2.5), bypassing the original policy intent
- There was no **fail-safe** — if all intelligence layers agreed on a server, execution proceeded without a final authority check

## Decision

Introduce a dedicated **Enforcement Gate** as Stage 5.5, positioned after routing selection but before execution. This gate:

- Is the **only** component allowed to block execution
- Operates on the final `(server, operation)` pair
- Is completely isolated from scoring, stability, and learning
- Defaults to **allow-all** (backward compatible)

## Consequences

### Positive
- Clear separation of **decision** (what is best) from **control** (what is allowed)
- Enforcement can be audited independently of routing
- Fail-safe: even if scoring produces a dangerous selection, enforcement can block it
- Default allow-all ensures zero behavioral change for existing deployments

### Negative
- Additional stage in the execution pipeline (minimal, sub-millisecond)
- Requires explicit rule configuration to be useful

### Neutral
- Enforcement rules are static (v3.0 frozen); future policy intelligence (v3.1) observes but does not modify them

## Architectural Principle

> Enforcement is the only control authority. No other layer may block execution.
