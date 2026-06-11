# MCP Runtime — C4 Architecture Model

**Version:** v3.1.0-stable  
**Model:** C4 (Context → Container → Component → Execution Flow)

---

## Level 1: System Context Diagram

```mermaid
C4Context
  Person(user, "MCP Client", "Sends tool execution requests")
  System(runtime, "MCP Runtime v3.1", "Adaptive decision-driven execution system with enforcement and observability")
  System_Ext(mcpServers, "MCP Tool Servers", "Git, Filesystem, Memory, GitHub, Fetch, Context7, Postgres, ChromaDB")
  System_Ext(chromaDB, "ChromaDB Cloud", "Knowledge retrieval for routing context")

  Rel(user, runtime, "Sends MCP request")
  Rel(runtime, mcpServers, "Executes tool operation")
  Rel(runtime, chromaDB, "Queries execution knowledge")
```

---

## Level 2: Container Diagram

```mermaid
C4Container
  Boundary(gateway, "MCP Gateway", "core routing + enforcement container") {
    Container(processor, "Request Processor", "Go", "Validates, resolves, scores, enforces, executes")
    Container(router, "Router", "Go", "Maps action types to MCP server capabilities")
    Container(enforcer, "Enforcement Engine", "Go", "Allow/Block control gate before execution")
  }

  Boundary(intelligence, "Intelligence Layer", "adaptive decisioning") {
    Container(scoring, "Scoring Engine", "Go", "Scores candidates by capability, knowledge, history")
    Container(stability, "Stability Engine", "Go", "Oscillation detection, exploration decay, convergence")
    Container(learning, "Learning Engine", "Go", "Weight updates per outcome")
  }

  Boundary(observability, "Observability Layer", "passive telemetry") {
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

---

## Level 3: Component Diagram (Gateway)

```mermaid
C4Component
  Boundary(gw, "Gateway", "runtime/mcp/v2/gateway.go") {
    Component(process, "Process()", "Go func", "Orchestrates all 12 stages")
    Component(selectBest, "selectBestServer()", "Go func", "Scores + sorts all candidates")
    Component(errorResp, "errorResponse()", "Go func", "Standardised error envelope")
  }

  Boundary(engines, "Engines", "supporting components") {
    Component(ee, "EnforcementEngine", "enforcement.go", "PolicyRule map + Check()")
    Component(se, "StabilityEngine", "feedback.go", "EffectiveRate + AdjustScore + RecordSelection")
    Component(le, "LearningEngine", "feedback.go", "WeightsFor + Update")
    Component(es, "ExplorationState", "feedback.go", "AdjustScoreWithRate")
    Component(pie, "PolicyIntelligenceEngine", "policy_intelligence.go", "Record + DetectDrift + GenerateSuggestions")
  }

  Rel(process, selectBest, "Delegates scoring")
  Rel(process, ee, "Stage 5.5 check")
  Rel(process, se, "Stability adjustment")
  Rel(process, le, "Outcome feedback")
  Rel(process, es, "Selection recording")
  Rel(ee, pie, "Post-enforcement event")
```

---

## Level 4: Execution Flow Diagram

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
    H -->|blocked| ERR4["Enforcement Blocked"]
    H -->|allowed| I["Stage 6: Execute"]
    I -->|fail| ERR5["Execution Failed"]
    I -->|ok| J["Stage 7: Learn + Governance"]
    J --> K["Stage 8: Normalize"]
    K --> L["Response + Trace"]
```

---

## Plane Ownership Map

| Plane | Components | Authority |
|-------|-----------|-----------|
| **Execution** | Gateway, MCP Adapters, Router | Deterministic execution |
| **Intelligence** | Scoring, Stability, Learning, Exploration | Decision influence only |
| **Control** | Enforcement Engine | Allow/Block authority |
| **Observability** | Decision Trace, Governance Audit, Policy Intelligence | Recording only |
