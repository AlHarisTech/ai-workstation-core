# MCP Runtime — C4 Architecture Model

**Version:** v3.1.0-stable  
**Model:** C4 (Context → Container → Component → Execution Flow + Data Contracts + Boundaries)

---

## Section 1: System Boundary Map

```
┌─────────────────────────────────────────────────┐
│                 EXTERNAL WORLD                   │
│  ┌──────────┐     ┌──────────────────────┐       │
│  │   MCP    │     │   MCP Tool Servers   │       │
│  │  Client  │     │  (Git, FS, Memory,   │       │
│  │ (Public) │     │   GitHub, Fetch,     │       │
│  └────┬─────┘     │   Context7, Postgres,│       │
│       │           │   ChromaDB HTTP)     │       │
│       │ MCP Req   └──────────┬───────────┘       │
│       ▼                      │                    │
│  ┌─────────────────────────────────────────┐      │
│  │         MCP RUNTIME v3.1                │      │
│  │  (Decision + Control + Execution +      │      │
│  │         Observability)                  │      │
│  └──────────────────────┬──────────────────┘      │
│                         │                         │
│                  ┌──────▼──────┐                  │
│                  │  ChromaDB   │                  │
│                  │   Cloud     │                  │
│                  │ (Knowledge) │                  │
│                  └─────────────┘                  │
└─────────────────────────────────────────────────┘
```

### System Boundary Rules

- **Inside MCP Runtime**: All decision logic, enforcement, execution orchestration, observability
- **Outside (controlled)**: MCP Tool Servers (pre-registered), ChromaDB (query-only)
- **Outside (untrusted)**: MCP Client (public, unauthenticated)
- **Rule**: No external system can influence enforcement logic directly

---

## Section 2: C4 Context Diagram

```mermaid
C4Context
  Person(user, "MCP Client", "Sends tool execution requests")
  System(runtime, "MCP Runtime v3.1", "Adaptive decision-driven execution system with enforcement and observability")
  System_Ext(mcpServers, "MCP Tool Servers", "Git, Filesystem, Memory, GitHub, Fetch, Context7, Postgres, ChromaDB")
  System_Ext(chromaDB, "ChromaDB Cloud", "Knowledge retrieval for routing context")

  Rel(user, runtime, "Sends MCP request", "MCP/JSON-RPC")
  Rel(runtime, mcpServers, "Executes tool operation", "HTTP/stdio")
  Rel(runtime, chromaDB, "Queries execution knowledge", "HTTP")
```

---

## Section 3: C4 Container Diagram

```mermaid
C4Container
  Boundary(gateway, "MCP Gateway Container", "runtime/mcp/v2/gateway.go") {
    Container(processor, "Request Processor", "Go", "Validates, resolves, scores, enforces, executes")
    Container(router, "Router", "Go", "Maps action types to MCP server capabilities")
    Container(enforcer, "Enforcement Engine", "Go", "Allow/Block control gate before execution")
  }

  Boundary(intelligence, "Intelligence Layer Container", "runtime/mcp/v2/feedback.go") {
    Container(scoring, "Scoring Engine", "Go", "Scores candidates by capability, knowledge, history")
    Container(stability, "Stability Engine", "Go", "Oscillation detection, exploration decay, convergence")
    Container(learning, "Learning Engine", "Go", "Weight updates per outcome")
  }

  Boundary(observability, "Observability Layer Container", "runtime/mcp/v2/") {
    Container(trace, "Decision Trace", "Go", "Per-request step capture")
    Container(policyIntel, "Policy Intelligence", "Go", "Event recording, drift detection, suggestions")
    Container(audit, "Governance Audit", "Go", "Immutable structured logging")
  }

  Rel(processor, router, "Resolves capability")
  Rel(processor, enforcer, "Checks enforcement")
  Rel(processor, scoring, "Gets scores")
  Rel(processor, stability, "Applies stability adjustment")
  Rel(processor, learning, "Sends outcome")
  Rel(processor, trace, "Appends trace steps")
  Rel(processor, audit, "Logs governance record")
  Rel(enforcer, policyIntel, "Records enforcement event")
```

### Container Ownership Map

| Container | File(s) | Plane | Lifecycle |
|-----------|---------|-------|-----------|
| Request Processor | `gateway.go` | Execution | Per-request |
| Router | `gateway.go` | Execution | Per-request |
| Scoring Engine | `feedback.go`, `schema.go` | Intelligence | Per-request + persistent weights |
| Stability Engine | `feedback.go` | Intelligence | Persistent (accumulates state) |
| Learning Engine | `feedback.go` | Intelligence | Per-request (appends) |
| Enforcement Engine | `enforcement.go` | Control | Persistent (rule map) |
| Policy Intelligence | `policy_intelligence.go` | Observability | Persistent (event store) |
| Decision Trace | `schema.go` | Observability | Per-request (attached to response) |
| Governance Audit | `governance.go` | Observability | Per-request (appends to log) |

---

## Section 4: C4 Component Diagram

```mermaid
C4Component
  Boundary(gw, "Gateway", "runtime/mcp/v2/gateway.go") {
    Component(process, "Process()", "Go func", "Orchestrates all 12 stages")
    Component(selectBest, "selectBestServer()", "Go func", "Scores + sorts all candidates")
    Component(errorResp, "errorResponse()", "Go func", "Standardised error envelope")
  }

  Boundary(engines, "Engine Components", "runtime/mcp/v2/") {
    Component(ee, "EnforcementEngine", "enforcement.go", "PolicyRule map + Check()")
    Component(se, "StabilityEngine", "feedback.go", "EffectiveRate + AdjustScore + RecordSelection")
    Component(le, "LearningEngine", "feedback.go", "WeightsFor + Update")
    Component(es, "ExplorationState", "feedback.go", "AdjustScoreWithRate")
    Component(pie, "PolicyIntelligenceEngine", "policy_intelligence.go", "Record + DetectDrift + GenerateSuggestions")
    Component(adapter, "MCP Adapters", "adapters.go", "HTTP/stdio server communication")
  }

  Rel(process, selectBest, "Delegates scoring")
  Rel(process, ee, "Stage 5.5 check")
  Rel(process, se, "Stability adjustment")
  Rel(process, le, "Outcome feedback")
  Rel(process, es, "Selection recording")
  Rel(ee, pie, "Post-enforcement event")
```

### Component Responsibilities

| Component | Responsibility | Input | Output |
|-----------|---------------|-------|--------|
| `Process()` | Stage orchestration | `MCPRequest` | `MCPResponse` |
| `selectBestServer()` | Candidate ranking | `[]ServerCandidate` | `ServerCandidate` |
| `EnforcementEngine.Check()` | Gate check | `(server, operation)` | `EnforcementResult` |
| `StabilityEngine.AdjustScore()` | Stability adjustment | `(server, score, operation)` | `adjustedScore` |
| `LearningEngine.Update()` | Weight learning | `(server, operation, success)` | `void` |
| `PolicyIntelligence.Record()` | Event recording | `PolicyEvent` | `void` |

---

## Section 5: Execution Flow

```mermaid
flowchart TD
    A["MCP Request"] --> B["Stage 1: Validate"]
    B -->|fail| ERR1["Validation Error"]
    B -->|pass| C["Stage 2: Policy (ACL)"]
    C -->|deny| ERR2["Policy Denied"]
    C -->|allow| D["Stage 3: Resolve Server"]
    D -->|not found| ERR3["Route Not Found"]
    D -->|found| E["Stage 4: Knowledge (ChromaDB)"]
    E --> F["Stage 4.5: Score + Select"]
    F --> G["Stage 5: Route"]
    G --> H["Stage 5.5: Enforcement Gate"]
    H -->|blocked| ERR4["Enforcement Blocked"]
    H -->|allowed| I["Stage 6: Execute on MCP Server"]
    I -->|fail| ERR5["Execution Failed"]
    I -->|ok| J["Stage 7: Learn + Governance"]
    J --> K["Stage 8: Normalize"]
    K --> L["Response + DecisionTrace"]
```

### Data Handoff Per Stage

| Stage | Input | Processing | Output | Owner Plane |
|-------|-------|-----------|--------|-------------|
| 1 | Raw request | Schema validation | `MCPRequest` | Execution |
| 2 | `MCPRequest` | ACL check | `policyResult` | Execution |
| 3 | `MCPRequest` | Server capability match | `[]ServerCandidate` | Execution |
| 4 | `MCPRequest` | ChromaDB query | `KnowledgeContext` | Execution |
| 4.5 | `[]ServerCandidate` | Score + stability + select | `selected Server` | Intelligence |
| 5 | `selected Server` | Route binding | `(server, operation)` | Execution |
| 5.5 | `(server, operation)` | Enforcement rule check | `EnforcementResult` | Control |
| 6 | `(server, operation, args)` | MCP call | `ExecutionResult` | Execution |
| 7 | `ExecutionResult` | Weight update + audit | `void` | Intelligence + Observability |
| 8 | `ExecutionResult` | Response format + trace | `MCPResponse` | Execution |

---

## Section 6: Plane Ownership Map

| Plane | Components | Authority |
|-------|-----------|-----------|
| **Execution** | Gateway, MCP Adapters, Router | Deterministic execution |
| **Intelligence** | Scoring, Stability, Learning, Exploration | Decision influence only |
| **Control** | Enforcement Engine | Allow/Block authority (sole) |
| **Observability** | Decision Trace, Governance Audit, Policy Intelligence | Recording only |
