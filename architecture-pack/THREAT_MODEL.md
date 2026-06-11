# MCP Runtime — Threat Model

**Version:** v3.1.1-stable  
**Methodology:** OWASP Application Threat Modeling  
**Scope:** MCP-specific attack surface

---

## 1. Trust Boundaries

```
[MCP Client] ──── [MCP Gateway] ──── [MCP Tool Servers]
    (public)          (internal)          (trusted/untrusted)
```

| Boundary | Trust Level | Notes |
|----------|-------------|-------|
| Client → Gateway | Low | Requests may be malformed, malicious, or unauthorised |
| Gateway → MCP Servers | Medium-High | Servers are pre-registered; assumes non-malicious but may have bugs |
| Gateway → ChromaDB | High | Service account with query-only scope |

---

## 2. Threat Enumeration (OWASP TOP 10 Mapping)

### T1: MCP Request Injection (A03:2021 — Injection)

**Description:** An attacker crafts a malformed MCP request that bypasses the schema validation (Stage 1) to execute an unregistered operation or reach an unintended server.

**Mitigation:**
- Stage 1 Validate rejects all requests with unknown `action_type` or `operation`
- Stage 3 Resolve only returns servers registered in the capability map
- Stage 5.5 Enforcement provides a final gate against `(server, operation)` pairs not explicitly allowed

**Residual risk:** Low — three independent barriers must be bypassed.

---

### T2: Enforcement Bypass (A01:2021 — Broken Access Control)

**Description:** An attacker exploits a race condition or logic gap to execute a blocked operation by bypassing Stage 5.5.

**Attack vectors:**
- Direct access to MCP server port (bypassing Gateway entirely)
- Exploitation of a routing bug that maps a blocked operation to an allowed `(server, operation)` pair

**Mitigation:**
- MCP servers listen only on localhost or internal network
- Enforcement rules match on exact `(server, operation)` pairs, not wildcards
- Fail-close mode blocks when enforcement is uncertain

**Residual risk:** Low — requires network access to internal ports.

---

### T3: Scoring Manipulation (A08:2021 — Software and Data Integrity Failures)

**Description:** An attacker influences routing by polluting the knowledge base or feedback history to favour a compromised server.

**Attack vectors:**
- Repeated successful executions on a compromised server to inflate its weight
- ChromaDB injection to return misleading context

**Mitigation:**
- Enforcement gate is independent of scoring — even if a compromised server is selected, enforcement can block it
- Exploration decay limits how quickly a new server can gain influence
- ChromaDB fallback returns empty `{}` to prevent self-bias

**Residual risk:** Medium — feedback pollution is possible; enforcement is the compensating control.

---

### T4: Denial of Service via Excessive Execution (A04:2021 — Uncontrolled Resource Consumption)

**Description:** An attacker sends a high volume of requests to exhaust MCP server resources (e.g., excessive git status calls).

**Attack vectors:**
- Many concurrent requests for the same operation
- Operations with slow execution paths (e.g., large file reads)

**Mitigation:**
- Rate Limiter (Stage 0 pre-gateway): token bucket per `(clientID, operation)` — 100 req/min soft limit, 500 req/min global limit
- On overflow: HTTP 429 + `PolicyEvent{decision: rate_limited}`
- Execution timeout (30s) per operation
- Backpressure at 200 in-flight requests

**Residual risk:** Low — rate limiting prevents resource exhaustion; operates entirely outside the internal decision pipeline.

---

### T5: Knowledge Base Poisoning (A08:2021 — Software and Data Integrity Failures)

**Description:** An attacker inserts malicious context into the ChromaDB knowledge base, causing the system to select a dangerous server for a given operation.

**Attack vectors:**
- Direct ChromaDB write access (requires API key)
- Exploitation of a ChromaDB vulnerability

**Mitigation:**
- ChromaDB is accessed with a query-only account
- Knowledge base is read-only at runtime
- Knowledge scores are advisory only — enforcement is the final gate

**Residual risk:** Low — assumes proper ChromaDB credential management.

---

### T6: Policy Intelligence Data Leakage (A05:2021 — Security Misconfiguration)

**Description:** The `PolicyEvent` stream reveals internal enforcement patterns (which operations are blocked, which servers are mistrusted) to an external observer.

**Attack vectors:**
- Governance audit logs exposed via unprotected endpoint
- DecisionTrace attached to every response reveals available servers and enforcement outcomes

**Mitigation:**
- `DecisionTrace` is included in response metadata by default — servers and enforcement outcomes are visible to the client
- This is a design choice for transparency, not a leak

**Residual risk:** None by design — trace visibility is intentional.

---

### T7: Malicious MCP Server (A06:2021 — Vulnerable and Outdated Components)

**Description:** A compromised or malicious MCP server returns arbitrary responses that trigger unintended behaviour in downstream systems.

**Attack vectors:**
- Server returns malicious filesystem paths, URLs, or SQL queries
- Server responds with malformed data that crashes the Gateway

**Mitigation:**
- Gateway treats server responses as opaque data — no deserialisation into sensitive structures
- Execution timeout prevents indefinite hangs
- No server-initiated communication — all requests are Gateway → Server

**Residual risk:** Medium — depends on how the client processes execution results.

---

## 3. Risk Summary

| ID | Threat | Likelihood | Impact | Risk | Mitigated By |
|----|--------|-----------|--------|------|-------------|
| T1 | Request injection | Low | High | Medium | Validate + Resolve + Enforcement |
| T2 | Enforcement bypass | Low | Critical | Medium | Network isolation + exact matching |
| T3 | Scoring manipulation | Medium | Medium | Medium | Enforcement override |
| T4 | Resource exhaustion | Low | Medium | Medium | Rate Limiter (token bucket, Stage 0) |
| T5 | Knowledge poisoning | Low | High | Medium | Query-only account |
| T6 | Data leakage | Low | Low | Low | Intentional transparency |
| T7 | Malicious server | Medium | High | Medium | Opaque response handling |

---

## 4. Recommended Hardening (Post-Freeze)

1. ~~Rate limiting layer~~ — **IMPLEMENTED in v3.1.1 as RateLimiter (Stage 0)** — token bucket per (clientID, operation) pre-gateway
2. **Policy rule versioning** — add timestamps to `PolicyRule` for audit trail lineage
3. **DecisionTrace encryption** — optional HMAC signature to prevent client tampering
4. **Server health probes** — detect compromised or slow servers before routing to them
