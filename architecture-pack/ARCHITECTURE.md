# MCP Runtime — C4 Architecture Model

**Version:** v3.1.1-stable
**Model:** C4 + Sequence Diagrams + Component Deep Specs

---

## 1. System Boundary Map

```
INSIDE MCP RUNTIME:
├── Request Processor       (gateway.go)
├── Router                  (gateway.go)
├── Scoring Engine          (feedback.go)
├── Stability Engine        (feedback.go)
├── Learning Engine         (feedback.go)
├── Enforcement Engine      (enforcement.go)       ← SOLE AUTHORITY
├── Policy Intelligence     (policy_intelligence.go)
├── Governance Audit        (governance.go)
└── Decision Trace          (schema.go)

OUTSIDE (registered): MCP Tool Servers, ChromaDB Cloud
ALWAYS EXTERNAL (untrusted): MCP Client
```

---

## 2. C4 Context Diagram

```mermaid
C4Context
  Person(user, "MCP Client", "Sends tool execution requests")
  Enterprise_Boundary(runtimeBoundary, "MCP Runtime System Boundary") {
    System(runtime, "MCP Runtime", "Decision-driven execution with enforcement and observability")
    System_Ext(auditLog, "Governance Audit Log", "Immutable structured audit trail")
  }
  System_Ext(mcpServers, "MCP Tool Servers", "Git, FS, Memory, GitHub, Fetch, Context7, Postgres, ChromaDB")
  System_Ext(chromaDB, "ChromaDB Cloud", "Knowledge retrieval")
  System_Ext(monitoring, "External Monitoring", "Log aggregation, metrics")

  Rel(user, runtime, "MCP request", "MCP/JSON-RPC")
  Rel(runtime, mcpServers, "Execute operation", "HTTP/stdio")
  Rel(runtime, chromaDB, "Query knowledge", "HTTP")
  Rel(runtime, auditLog, "Write governance", "append-only")
```

---

## 3. C4 Container Diagram

```mermaid
C4Container
  Boundary(run, "MCP Runtime", "") {
    Boundary(gw, "Gateway Container", "gateway.go") {
      Container(processor, "Request Processor", "Go", "Orchestrates stages 1–8")
      Container(router, "Router", "Go", "Maps action types to servers")
    }
    Boundary(intel, "Intelligence Container", "feedback.go") {
      Container(scoring, "Scoring Engine", "Go", "Candidate scoring")
      Container(stability, "Stability Engine", "Go", "Oscillation control, exploration decay")
      Container(learning, "Learning Engine", "Go", "Weight updates")
    }
    Boundary(ctrl, "Control Container", "enforcement.go") {
      Container(enforcer, "Enforcement Engine", "Go", "Allow/Block gate")
    }
    Boundary(obs, "Observability Container", "") {
      Container(trace, "Decision Trace", "Go", "Per-request step capture")
      Container(policyIntel, "Policy Intelligence", "Go", "Event recording, drift detection")
      Container(audit, "Governance Audit", "Go", "Structured logging")
    }
  }

  Rel(processor, enforcer, "Stage 5.5 check")
  Rel(processor, scoring, "Stage 4.5 scores")
  Rel(processor, stability, "Stability adjustment")
  Rel(processor, learning, "Stage 7 outcome")
  Rel(enforcer, policyIntel, "Post-enforcement event")
```

---

## 4. C4 Component Diagram

| Component | File | Input | Output | Invariant |
|-----------|------|-------|--------|-----------|
| `Process()` | `gateway.go` | `MCPRequest` | `MCPResponse` | Always produces response |
| `selectBestServer()` | `gateway.go` | `[]ServerCandidate` | `ServerCandidate` | Deterministic scores |
| `EnforcementEngine.Check()` | `enforcement.go` | `(server, op)` | `EnforcementResult` | Sole blocking authority |
| `StabilityEngine.AdjustScore()` | `feedback.go` | `(server, score, op)` | `adjustedScore` | Never blocks execution |
| `LearningEngine.Update()` | `feedback.go` | `(server, op, success)` | `void` | No live effect on request; delta: +0.01/-0.02 (safety-biased) |
| `RateLimiter.Check()` | Edge | `(clientID, op)` | `allow/429` | Pre-gateway only; never enters pipeline |
| `PolicyIntelligence.Record()` | `policy_intelligence.go` | `PolicyEvent` | `void` | Never feeds back |

---

## 5. Execution Flow

```mermaid
flowchart TD
    A["Request"] --> B["Stage 1: Validate"]
    B -->|fail| ERR1["Validation Error"]
    B -->|pass| C["Stage 2: Policy"]
    C -->|deny| ERR2["Policy Denied"]
    C -->|allow| D["Stage 3: Resolve"]
    D -->|not found| ERR3["Route Not Found"]
    D -->|found| E["Stage 4: Knowledge"]
    E --> F["Stage 4.5: Score + Select"]
    F --> G["Stage 5: Route"]
    G --> H["Stage 5.5: Enforcement Gate"]
    H -->|blocked| ERR4["Blocked"]
    H -->|allowed| I["Stage 6: Execute"]
    I -->|fail| ERR5["Execution Failed"]
    I -->|ok| J["Stage 7: Learn + Governance"]
    J --> K["Stage 8: Normalize"]
    K --> L["Response + Trace"]
```

---

## 6. Sequence Diagrams

### 6.0 Rate Limit Block
```mermaid
sequenceDiagram
    participant Client as MCP Client
    participant RL as Rate Limiter
    participant PI as Policy Intelligence

    Client->>RL: MCP Request
    RL->>RL: Check token bucket
    RL-->>RL: Overflow
    RL->>PI: Record PolicyEvent (rate_limited)
    RL-->>Client: HTTP 429
```

### 6.1 Normal Request

```mermaid
sequenceDiagram
    participant Client
    participant GW as Gateway
    participant EE as Enforcement
    participant Server as MCP Server

    Client->>GW: MCP Request
    GW->>GW: Validate + Policy + Resolve + Score
    GW->>EE: Stage 5.5: Check(server, op)
    EE-->>GW: Allowed
    GW->>Server: Execute
    Server-->>GW: Result
    GW-->>Client: Response + Trace
```

### 6.2 Enforcement Block

```mermaid
sequenceDiagram
    participant Client
    participant GW as Gateway
    participant EE as Enforcement

    Client->>GW: Request
    GW->>GW: Resolve + Score + Route
    GW->>EE: Check(server, op)
    EE-->>GW: Blocked
    GW-->>Client: Error + Trace (blocked)
```

---

## 7. Plane Ownership Map

| Plane | Components | Authority |
|-------|-----------|-----------|
| **Execution** | Gateway, Router, Adapters | Deterministic execution |
| **Intelligence** | Scoring, Stability, Learning, Exploration | Decision influence only |
| **Control** | Enforcement Engine | Allow/Block (sole) |
| **Observability** | Decision Trace, Policy Intelligence, Audit | Recording only |

---

## 8. Data Contracts

### EnforcementResult
```
{ Allowed: bool, RuleMatched: string, BlockReason: string }
Invariant: Allowed=false → pipeline terminates; no override possible
```

### PolicyEvent
```
{ TraceID, Server, Operation, decision: "allowed"|"blocked"|"audit"|"rate_limited", RuleMatched, Timestamp }
Invariant: Single mutually exclusive enum — exactly one value per event
```

### TraceStep
```
{ Stage: int, Component: string, Input, Output, Duration, Error }
Invariant: All stages captured; cap at 128 steps
Size: Input/Output ≤ 512 bytes each; total trace ≤ 64 KB
```

---

## 9. System Invariants (with Binding Table)

| # | Invariant | Enforced By |
|---|-----------|-------------|
| I1 | Enforcement is sole control authority | `EnforcementEngine.Check()` |
| I2 | Intelligence cannot influence enforcement | `StabilityEngine`, `ScoringEngine`, `LearningEngine` |
| I3 | Deterministic execution | `selectBestServer()`, `Process()` |
| I4 | System works without intelligence | `Process()` stage orchestration |
| I5 | Traceability always on | `Process()` defer trace population |
| I6 | Policy Intelligence is passive | `PolicyIntelligenceEngine` design constraint |
| I7 | Fail-close on uncertainty | `EnforcementEngine.Check()` timeout handler |
| I8 | Rate Limiter is edge-only | `RateLimiter` Stage 0 placement |
