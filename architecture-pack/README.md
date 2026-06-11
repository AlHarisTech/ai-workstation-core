# MCP Runtime — Architecture Pack

**Version:** v3.1.1-stable  
**Status:** Documentation Layer (Read-only, Non-invasive)  
**Relationship:** Sidecar to runtime system — zero modification of any source file

---

## Purpose

This pack is a **formal documentation layer** that exists independently of the runtime implementation. It captures:

- **Visual architecture** (C4 model: context → container → component → execution flow)
- **Design decisions** (ADRs: why every architectural choice was made)
- **Performance envelope** (SLA bounds, latency budgets, stress thresholds)
- **Threat model** (MCP-specific attack surface, trust boundaries, abuse scenarios)

## Governance Rules

- NO file under `runtime/` is touched
- NO behavioral specification overrides implementation
- This layer is **read-only knowledge**, not executable configuration

## Contents

| File | Purpose |
|------|---------|
| `ARCHITECTURE.md` | C4 Model — all four levels with Mermaid diagrams |
| `ADR/ADR-001-enforcement-gate-isolation.md` | Why Enforcement is the sole control authority |
| `ADR/ADR-002-passive-policy-intelligence.md` | Why Policy Intelligence is observer-only |
| `ADR/ADR-003-stability-engine-independence.md` | Why Stability Engine operates independently of scoring |
| `BENCHMARK_SPEC.md` | Performance envelope — latency, throughput, stress thresholds |
| `THREAT_MODEL.md` | MCP-specific threat model with OWASP mapping |

## Relationship to SYSTEM_DESIGN.md

SYSTEM_DESIGN.md is the **contract-level specification**.  
This pack is the **visualisation + rationale + measurement + security companion**.
