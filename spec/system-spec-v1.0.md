# AI Workstation Core — System Specification v1.0

**Status:** CANONICAL REFERENCE (IMMUTABLE)
**Source:** Collapsed from ADR-0001 through ADR-0008
**Principle:** No redesign — recomposition of 14 ADRs into a single authoritative layered specification
**Entry Point:** This document is the single source of truth for the AI Workstation Core system

---

## Document Structure

This specification is organized into 5 books, each collapsing multiple ADRs into a unified layer.

```
Book I    — Execution Substrate (ADR-0001, ADR-0002, ADR-0003-A/B/C/D/E)
Book II   — Truth & Projection (ADR-0004, ADR-0005, ADR-0006)
Book III  — Formal Abstraction (ADR-0007)
Book IV   — Compositional Calculus (ADR-0008)
Book V    — Unified Invariant System (cross-ADR)
```

---

# BOOK I — EXECUTION SUBSTRATE

## I.1 Canonical Architecture

### I.1.1 System Type

```
AI Workstation Core is an Engineering Operating System with a
Specification Runtime Kernel, Multi-Agent Execution Layer,
Governance & Validation Core, and Plugin-based Ecosystem.

Source: ADR-0001
```

### I.1.2 Layer Architecture

```
Kernel Layer (lifecycle, session, policy, capability, event bus)
    │
Specification Runtime (.spec → semantic graph → executable plan)
    │
Domain Engine (bounded contexts, entities, services, aggregates, contracts)
    │
Runtime Execution Layer (agents, orchestration, tool adapters, execution engine)
    │
Governance Engine (policy registry, approval flows, risk engine, change control)
    │
Validation Engine (static analysis, runtime verification, test orchestrator, evidence)
    │
Observability Platform (telemetry, tracing, logging, metrics)
    │
Plugin & Adapter System (agents, tools, providers, integrations, external engines)
```

### I.1.3 System Boundaries (BR-001 through BR-005)

| Boundary | Rule |
|----------|------|
| BR-001 | Spec > Code > Runtime (specification supremacy) |
| BR-002 | No hardcoded environment assumptions |
| BR-003 | Agent execution ≠ System logic |
| BR-004 | Plugin isolation — every external system via adapter only |
| BR-005 | CDCS-1 is immutable reference (READ ONLY, NO EXECUTION DEPENDENCY) |

### I.1.4 Execution Modes (ADR-0002-F)

| Mode | Path | Use Case |
|------|------|----------|
| Mode 0 | Direct execution (no spec, no compiler, no RIR) | Single-turn Q&A, trivial fixes |
| Mode 1 | Lightweight workflow (ephemeral graph, no persistence) | Minor features, small bugs |
| Mode 2 | Compiled workflow (Spec → Compiler → RIR → Runtime) | Deterministic, replayable, governed |
| Mode 3 | Full autonomous system (Mode 2 + plugins + persistence + recovery) | Long-running, multi-agent, checkpoints |

---

## I.2 Compiler System

### I.2.1 Compiler Architecture (ADR-0002, ADR-0002-C)

The Compiler is a closed, staged transformation function:

```
f: Spec → RIR
f = Pass₆ ∘ Pass₅ ∘ Pass₄ ∘ Pass₃ ∘ Pass₂ ∘ Pass₁ ∘ Pass₀
```

| Pass | Name | Input | Output |
|------|------|-------|--------|
| Pass -1 | Spec Discovery | Discovery root | Raw spec content |
| Pass 0 | Schema Gate | Raw spec | Validated spec nodes |
| Pass 1 | Normalization | Validated spec | Canonically ordered spec |
| Pass 2 | Semantic Resolution | Normalized spec | Semantic AST |
| Pass 3 | Graph Construction | Semantic AST | CEG (pre-validation) |
| Pass 4 | Execution Planning | Validated CEG | CEG + execution plan |
| Pass 5 | Validation Binding | CEG + plan | CEG + plan + validations |
| Pass 6 | IR Emission | Complete artifact | RIR (JSON) |

**Compiler Semantics Rules (CR-001 through CR-004):**

| Rule | Description |
|------|-------------|
| CR-001 | No execution awareness — compiler does not know "how" execution happens |
| CR-002 | Deterministic output — same input → same RIR |
| CR-003 | Fail-closed — ambiguity, missing dependency, undefined node → compile failure |
| CR-004 | No runtime feedback loop — no adaptive compilation, no dynamic injection |

### I.2.2 RIR Schema (ADR-0002-A)

The Runtime Intermediate Representation is the ONLY artifact crossing the Compiler → Runtime boundary.

```yaml
rir:
  meta:
    schema_version: "1.0"
    spec_hash: "<sha256>"
    compilation_id: "<uuid>"
    compiled_at: "<iso8601>"
    source_spec: "<ref>"
    compiler_version: "<semver>"

  execution_plan:
    scheduling_model: "static_dag" | "layered" | "streaming"
    execution_strategy: "parallel_first" | "dependency_first"
    max_parallelism: <int>
    fail_strategy: "fast" | "continue" | "isolated"

  units:
    - id: "<uuid>"
      type: "agent" | "capability" | "task" | "gate" | "checkpoint"
      agent_ref: "<spec-ref>"
      binding:
        contract: "<contract-ref>"
        isolation: "process" | "container" | "virtual"
      dependencies: ["<unit-id>"]
      data_flow:
        inputs:
          - key: "<input-key>"
            from:
              unit: "<unit-id>"
              output: "<output-key>"
            contract: "<data-contract-ref>"
        outputs: ["<key>"]
      validation:
        preconditions:  ["<assertion-ref>"]
        postconditions: ["<assertion-ref>"]
        invariants:     ["<assertion-ref>"]
        failure_modes:  ["<failure-mode-ref>"]
      scheduling:
        priority: <int>
        deadline: "<iso8601>" | null
        retry:
          max_attempts: <int>
          backoff: "constant" | "linear" | "exponential"
      context:
        mode: "fresh" | "inherit" | "checkpoint:<id>"
        max_tokens: <int> | null
        tools: ["<tool-ref>"]
      governance:
        required_approvals: ["<role>"]
        change_scope: "read_only" | "scoped_write" | "full"

  graph:
    dag:
      nodes: ["<unit-id>"]
      edges:
        - from: "<unit-id>"
          to: "<unit-id>"
          type: "dependency" | "data_flow" | "control_flow"
    cycles: []
    isolated_regions:
      - units: ["<unit-id>"]
        reason: "<label>"

  assertions:
    - id: "<uuid>"
      type: "precondition" | "postcondition" | "invariant"
      predicate: "<declarative-expression>"
      on_failure: "abort" | "retry" | "skip" | "escalate"

  failure_modes:
    - id: "<uuid>"
      condition: "<predicate>"
      action: "retry" | "skip" | "abort" | "escalate"
      max_retries: <int>

  isolation_boundaries:
    - id: "<uuid>"
      units: ["<unit-id>"]
      type:
        model: "strict"
        constraints:
          - "no_shared_state"
          - "no_network"
          - "no_filesystem"
          - "cpu_bound"
          - "memory_bound"
      reason: "<label>"

  checkpoints:
    - after_unit: "<unit-id>"
      save: ["context", "artifacts", "metrics"]
      label: "<string>"

  handoff:
    output_artifacts: ["<path-ref>"]
    evidence: ["<evidence-ref>"]
    next_spec: "<spec-ref>" | null
```

**RIR Invariants (I-001 through I-006):**

| ID | Invariant |
|----|-----------|
| I-001 | `graph.dag.cycles` MUST be `[]` |
| I-002 | Every `unit.dependencies` entry MUST resolve to a known `unit.id` |
| I-003 | Every `validation.preconditions` ref MUST exist in `assertions` |
| I-004 | `execution_plan.max_parallelism` > 0 |
| I-005 | No `unit.id` duplication |
| I-006 | `spec_hash` MUST be non-empty |

### I.2.3 CEG Semantics (ADR-0002-B)

The Canonical Execution Graph is the single structural representation of execution order — a pure DAG.

```
CEG = (V, E, λ_v, λ_e)

V   = finite set of vertices (execution units)
E   = finite set of directed edges ⊆ V × V
λ_v = vertex labeling: V → UnitType × Properties
λ_e = edge labeling: E → EdgeType × Contract

Constraints:
  C1: Every v has exactly one type ∈ {agent, capability, task, gate, checkpoint}
  C2: E is acyclic (NOT transitive); reachability R is the transitive closure of E
  C3: CEG is weakly connected (no unreachable nodes)
  C4: max_depth(CEG) ≤ 128
  C5: indegree(v) ≤ 64
```

**Edge types:**

| Type | Semantics |
|------|-----------|
| DEPENDENCY | v CANNOT start until u reaches SUCCESS |
| DATA_FLOW | v consumes output of u; carries DataContract |
| CONTROL_FLOW | u (gate) determines WHETHER v executes |
| BARRIER | Compile-time expansion to explicit DEPENDENCY edges |

**CEG Validation Rules (V-001 through V-014):**

| Rule | Check | Pass | On Failure |
|------|-------|------|------------|
| V-001 | No cycles | Pass 0, Pass 3 | COMPILE_ERROR |
| V-002 | All nodes reachable | Pass 2, Pass 4 | COMPILE_ERROR |
| V-003 | indegree ≤ 64 | Pass 0, Pass 2, Pass 3, Pass 5 | COMPILE_ERROR |
| V-004 | max_depth ≤ 128 | Pass 3, Pass 4 | COMPILE_ERROR |
| V-005 | Gate nodes have branches + default | Pass 3 | COMPILE_ERROR |
| V-006 | Gate branches are mutually exclusive | GLOBAL | WARNING |
| V-007 | Data flow edges carry contract ref | Pass 0, Pass 3 | COMPILE_ERROR |
| V-008 | Cross-region edges are DEPENDENCY only | Pass 3 | COMPILE_ERROR |
| V-009 | No orphan nodes | Pass 3 | COMPILE_ERROR |
| V-010 | Checkpoint nodes have successors | Pass 5 | WARNING |
| V-011 | Sub-graph expansion depth ≤ 8 | Pass 3 | COMPILE_ERROR |
| V-012 | Mode 2 + checkpoint node | Pass 3 | COMPILE_ERROR |
| V-013 | ESCALATE branch when mode ≠ 2 | Pass 3 | COMPILE_ERROR |
| V-014 | At most one ESCALATE branch per gate | Pass 3 | COMPILE_ERROR |

---

## I.3 Runtime Engine

### I.3.1 Execution Semantics (ADR-0003-A)

**Node Status Lattice:**

```
PENDING | BLOCKED | READY | RUNNING | SUCCESS | FAILURE | SKIPPED | TERMINATED
```

Terminal states: SUCCESS, FAILURE, SKIPPED, TERMINATED
Non-terminal: PENDING, BLOCKED, READY, RUNNING

**Runtime State Machine (orthogonal to node states):**

```
RUNNING → TERMINATING → TERMINATED
RUNNING → ABORTED
```

**Node Activation Rule:**

```
A node v becomes READY iff:
  ALL v.activation.requires ⊆ {SUCCESS}
  AND NO v.activation.requires ∈ {FAILURE that invalidates activation}
  AND v.activation.condition evaluates TRUE
```

**Runtime Execution Loop:**

```
RUN(execution_plan):
  for each lane ℓ in topological layer order:
    for each wave w in lane[ℓ].waves:
      dispatch ALL nodes in w concurrently
      v.status: READY → RUNNING (via Commit Engine)
      result ← adapter(v.type).execute(v, context)
      v.status ← result SUCCESS/FAILURE (via Commit Engine)
      on MODE_INSUFFICIENT:
        ∀ node n in {BLOCKED, READY, RUNNING}: n.status ← TERMINATED
        emit context_snapshot
        return to Router for reclassify (F-004)
    recompute BLOCKED→READY / BLOCKED→SKIPPED
  return final status map
```

**TERMINATED semantics:** TERMINATED is absorbing and non-propagating (RT-008). No dependency propagation originates from TERMINATED.

### I.3.2 Wave & Concurrency Model (ADR-0003-A §5)

**Lane:** L_ℓ = {v ∈ V : layer(v) = ℓ, no edge between any pair}

**Conflict:** Two same-layer nodes conflict if they share a data-contract reference or side_effect_surface.

**Wave partitioning:** Uses deterministic graph coloring (node-id-ordered) to partition each layer's candidates into conflict-free waves. Waves execute in ascending index order.

**Determinism guarantee:** Same CEG + same status_map → identical wave partitioning → identical final state.

### I.3.3 Runtime Components (ADR-0003-B)

| Component | Responsibility | State Mutation |
|-----------|---------------|----------------|
| **Runtime Coordinator** | Top-level state machine; lifecycle orchestration (RUN, TERMINATE, ABORT, RESUME, ESCALATE) | Never writes node state |
| **Scheduler** | Computes READY set; enforces wave ordering; claims units | Issues DispatchClaim (not READY→RUNNING) |
| **Adapter Dispatcher** | Plugin resolution, execution dispatch, isolation enforcement, result normalization | Never |
| **Commit Engine** | Atomic status commit; publishes events; prevents double-commit | **Sole writer of ALL node state transitions** |
| **Event Bus** | Transient ordered delivery of internal events | Never |
| **Execution Journal** | Durable append-only audit record of visible transitions | Never |
| **Snapshot Engine** (Mode 3) | Checkpoint persistence and restore | Never |

**Component interaction — Normal Node Lifecycle:**

```
Scheduler → DispatchClaim → Commit Engine (READY → RUNNING)
Commit Engine → append(Journal) → publish(NodeStarted)
Scheduler → dispatch → Dispatcher → execute → ExecutionResult
Scheduler → commit → Commit Engine (RUNNING → SUCCESS/FAILURE)
Commit Engine → append(Journal) → publish(NodeCommitted)
Scheduler ← subscribe(NodeCommitted) ← Event Bus
Scheduler → recompute → Commit Engine (BLOCKED → READY/SKIPPED)
```

**All state transitions flow through Commit Engine:**

```
READY    → RUNNING     (Commit Engine, on DispatchClaim)
RUNNING  → SUCCESS     (Commit Engine, on ExecutionResult)
RUNNING  → FAILURE     (Commit Engine, on ExecutionResult)
READY    → TERMINATED  (Commit Engine, on TerminateRequest)
BLOCKED  → TERMINATED  (Commit Engine, on TerminateRequest)
RUNNING  → TERMINATED  (Commit Engine, on TerminateRequest)
PENDING  → TERMINATED  (Commit Engine, on TerminateRequest)
BLOCKED  → READY       (Commit Engine, on recompute)
BLOCKED  → SKIPPED     (Commit Engine, on recompute)
```

### I.3.4 Runtime Invariants (RI-001 through RI-020)

| ID | Invariant |
|----|-----------|
| RI-001 | Commit events are immutable once published |
| RI-002 | Scheduler reacts only to committed events (never invoked directly by Commit Engine) |
| RI-003 | Every node has exactly one terminal transition |
| RI-004 | Status transitions are monotonic (no reversion from terminal) |
| RI-005 | TERMINATE never propagates — it is absorbing |
| RI-006 | ABORT terminates the entire execution immediately |
| RI-007 | Runtime derives no new graph structure (CEG is immutable) |
| RI-008 | Dispatch never bypasses Commit Engine |
| RI-009 | Commit Engine is the only component allowed to mutate node state |
| RI-010 | Scheduler decisions are deterministic (same CEG + same status_map → same READY set) |
| RI-011 | Snapshots are Mode-3 only |
| RI-012 | Snapshot is fully self-contained (no compiler/graph reconstruction needed for restore) |
| RI-013 | Restore requires hash + fingerprint compatibility |
| RI-014 | Restore never re-derives state — it loads exact scheduler_state from snapshot |
| RI-015 | Escalation causes clean restart only |
| RI-016 | Every visible transition is journaled before publication on Event Bus |
| RI-017 | Event ordering is deterministic within a single execution; undefined across executions |
| RI-018 | Restore reconstructs runtime state only — it never replays side effects |
| RI-019 | Scheduler runnable_set remains immutable between committed events |
| RI-020 | Event publication failure after durable journal write → ABORT with pending_publication |

---

## I.4 Adapter Layer

### I.4.1 Tool Adapters (ADR-0003-D)

Tools are the primitive execution unit — stateless, atomic, sandbox-isolated.

**Tool Contract:**

```yaml
tool_contract:
  tool_id: "<uuid>"
  capability_ref: "<capability-ref>"
  execution:
    model: "synchronous" | "async_poll"
    timeout_ms: <int>
    idempotent: true | false
    retry:
      max_attempts: <int>
      backoff: "constant" | "linear" | "exponential"
  inputs: [{ name, type, required, default }]
  outputs: [{ name, type }]
  side_effect_surface: ["fs:<path>", "network:<domain>"]
  provider:
    type: "llm" | "mcp_server" | "cli" | "api" | "internal"
    config_ref: "<provider-config-ref>"
  isolation:
    model: "process" | "container" | "virtual" | "inline"
    filesystem_access: "none" | "read_only:<path>" | "scoped:<path>"
    network_access: "none" | "restricted:<domain-list>" | "full"
```

**Provider types:** llm (OpenAI, Anthropic, Gemini, local), mcp_server, cli, api, internal

**Invocation path:** Dispatcher → Isolation Binder (sandbox) → Provider Resolver → Execution → Result Normalizer → Commit Engine

**Tool Invariants (TI-001 through TI-012):**

| ID | Invariant |
|----|-----------|
| TI-001 | Every tool invocation is isolated per declared sandbox model |
| TI-002 | CapabilityContract constraints are enforced locally — no governance call |
| TI-003 | Tool result flows through Commit Engine |
| TI-004 | Side effects are declared in tool_contract (wave conflict detection) |
| TI-005 | Provider credentials are referenced, never inlined |
| TI-006 | Sandbox violations produce FAILED node state |
| TI-007 | Tool adapters are stateless between invocations |
| TI-008 | Retry is bounded by max_attempts and backoff |
| TI-012 | Tool lifecycle transitions are monotonic |

### I.4.2 Agent Adapters (ADR-0003-C)

Agents are orchestrators that compose tool invocations into coherent execution sequences within a single CEG agent node.

**Agent Contract:**

```yaml
agent_contract:
  agent_id: "<uuid>"
  capability_ref: "<capability-ref>"
  tool_allowlist:
    - tool_ref: "<tool-id>"
      max_invocations: <int> | null
      scope: "full" | "restricted"
  orchestration:
    model: "reactive" | "planned" | "autonomous"
    max_tool_invocations: <int>
    max_duration_ms: <int>
  plan:
    max_depth: <int>
    replan_enabled: true | false
    replan_max_iterations: <int>
  state:
    model: "stateless" | "session" | "checkpoint"
  isolation:
    model: "process" | "container" | "virtual"
    filesystem_access: "none" | "read_only:<path>" | "scoped:<path>"
    network_access: "none" | "restricted:<domain-list>"
  side_effect_surface: ["fs:<path>", "network:<domain>"]
  handoff:
    artifacts: ["<key>"]
    evidence: ["<key>"]
    next_agent: "<agent-ref>" | null
```

**Orchestration models:**

| Model | Behavior | Replan |
|-------|----------|--------|
| reactive | Input → LLM → Output | No |
| planned | Input → Plan → [tools] → Output | No |
| autonomous | Input → Plan → [tools] ⇄ replan → Output | Bounded (replan_max_iterations) |

**Key rules:**
- Agent isolation ≥ tool isolation for all tools in allowlist
- Every tool invocation within an agent is validated against the agent's allowlist
- Agents never bypass the tool pipeline — all tool calls go through the standard Tool Adapter
- Tool failures inside an agent do NOT cascade to other CEG nodes
- Agent lifecycle is monotonic (same as tools)

**Agent Invariants (AI-001 through AI-012):**

| ID | Invariant |
|----|-----------|
| AI-001 | Agents compose tools — never replace or bypass the tool pipeline |
| AI-002 | Every tool invocation validated against agent allowlist |
| AI-003 | Replan bounded by replan_max_iterations |
| AI-004 | Agent isolation ≥ tool isolation for all tools in allowlist |
| AI-005 | Tool invocations flow through same Commit Engine pipeline |
| AI-006 | Agent state scoped to single CEG node execution |
| AI-007 | Agent handoff passes context but never mutates receiver's CEG node |
| AI-008 | Max tool invocations and max duration are hard limits |
| AI-009 | Agent side effects = union of all tool side effects + agent-level declarations |
| AI-010 | Tool failures inside agent do NOT cascade to other CEG nodes |
| AI-012 | Agent lifecycle is monotonic |

---

## I.5 Governance Engine (ADR-0003-E)

### I.5.1 Architectural Position

```
Governance is a Control Plane.
Runtime is a Data Plane.

They share no state.
They share no execution path.
They communicate exclusively through signals.
```

### I.5.2 Governance Components

| Component | Responsibility |
|-----------|---------------|
| **Admission Controller** | Controls entry into execution (PERMIT, DENY, DEFER, ESCALATE_REQUIRED) |
| **Policy Evaluator** | Pure function — evaluates policies against context |
| **Constraint Engine** | Validates hard/soft constraints |
| **Capability Governor** | Produces immutable CapabilityContract; never called at execution time |
| **Risk Engine** | Stateful but deterministic within execution; advisory only |
| **Escalation Manager** | Emits signals (ESCALATE, TERMINATE); never executes |
| **Compliance & Audit Engine** | Immutable audit records; sole allocator of sequence numbers |
| **Governance Event Stream** | Separate from Runtime Event Bus |

### I.5.3 Signal Model

**Mandatory signals (MUST be honored):**

| Signal | Effect |
|--------|--------|
| TERMINATE | Coordinator MUST issue TerminateRequest |
| DENY | Router MUST reject execution |
| RESTRICT | Dispatcher MUST narrow scope |

**Advisory signals (evaluated by target):**

| Signal | Effect |
|--------|--------|
| ESCALATE | Router evaluates reclassification |
| DEFER | Router may queue |
| ANNOTATE | Appended to audit trail |

### I.5.4 Runtime Signal Adapter

The ONLY bridge between Runtime Event Bus and Governance Event Stream — strictly unidirectional (GI-022). Governance components never subscribe to Runtime Event Bus directly (GI-013).

### I.5.5 Governance Invariants (GI-001 through GI-022)

| ID | Invariant |
|----|-----------|
| GI-001 | Governance never executes work |
| GI-002 | Governance never mutates CEG |
| GI-003 | Governance never commits node state |
| GI-004 | All governance decisions are auditable |
| GI-005 | Policy evaluation is deterministic (pure function) |
| GI-006 | Governance signals are immutable once emitted |
| GI-007 | Governance influences execution only through policies, signals, and contracts |
| GI-008 | Governance event ordering is deterministic within one execution |
| GI-009 | Governance and Runtime event streams are strictly separated |
| GI-010 | Escalation Manager may recommend TERMINATE but never performs termination |
| GI-011 | Admission decisions are final; revision requires new decision record |
| GI-012 | Capability restrictions are monotonic (never widen scope) |
| GI-013 | Runtime events observed only through read-only Runtime Signal Adapter |
| GI-014 | Data-plane never synchronously calls Governance during execution |
| GI-015 | Mandatory signals MUST be honored; failure → ABORT |
| GI-016 | Governance decisions are never memory-only; durable before acknowledgement |
| GI-017 | Compliance & Audit Engine is sole allocator of governance sequence numbers |
| GI-018 | Single policy manifest per execution; policies immutable during execution |
| GI-019 | Admission decisions immutable but may expire; expired → re-admission |
| GI-020 | Escalation priority: TERMINATE > DENY > ESCALATE > DEFER > ANNOTATE |
| GI-021 | Mandatory signal post-condition verification with bounded timeout |
| GI-022 | Runtime Signal Adapter is strictly unidirectional |

---

# BOOK II — TRUTH & PROJECTION

## II.1 Observability Layer (ADR-0004)

### II.1.1 Architectural Position

The Observability Layer is a passive, cross-cutting infrastructure. It captures complete execution lineage without affecting determinism or execution ordering.

```
Observability MUST NOT perform causal inference.
It records WHAT happened.
It never determines WHY it happened.
All interpretation is delegated to external layers.
```

### II.1.2 Components

| Component | Responsibility |
|-----------|---------------|
| **Trace Collector** | Captures per-node execution traces (inputs, outputs, state transitions, timing) |
| **Replay Engine** | Reconstructs prior execution deterministically from stored traces |
| **Audit Engine** | Unifies Runtime Journal + Governance Audit into single queryable surface |
| **Lineage Engine** | Tracks data provenance across nodes, agents, and handoffs |
| **Unified Event Store** | Append-only, durable storage for all observability data |

### II.1.3 Trace Schema

A trace is the complete record of a single execution unit from dispatch to commit.

```yaml
trace:
  trace_id: "<uuid>"
  execution_id: "<uuid>"
  node_id: "<uuid>"
  node_type: "agent" | "capability" | "task" | "gate" | "checkpoint"
  span:
    scheduled_at, dispatched_at, started_at, completed_at, committed_at
  context: { wave_id, lane_id, region_id, execution_mode }
  inputs: { spec_ref, rir_hash, ceg_hash, contract_ref, invocation_context }
  execution: { provider, sandbox_model, tool_invocations, agent_state_snapshot }
  outputs: { status, result_hash, artifacts, evidence, metrics }
  lineage: { predecessor_nodes, predecessor_traces, handoff_from, data_flow }
  commit: { sequence, terminal_status, committed_at }
```

### II.1.4 Replay Determinism Contract

```
REPLAY is deterministic iff:
  1. Same RIR (rir_hash)
  2. Same compiler version
  3. Same plugin versions (agent/tool adapters)
  4. Same provider configurations
  5. ALL external providers are:
     a) deterministic-mode locked, OR
     b) snapshot-replayed (outputs from recorded traces), OR
     c) explicitly mocked via trace inputs
```

### II.1.5 Store Ordering Semantics

```
TOTAL order within a single execution_id
PARTIAL order across execution_ids
NO global ordering guarantee
```

### II.1.6 Observability Invariants (OB-001 through OB-010)

| ID | Invariant |
|----|-----------|
| OB-001 | Observability is passive — never affects execution; records WHAT, never WHY |
| OB-002 | Trace collection is asynchronous — never adds latency to execution path |
| OB-003 | Trace completeness is verified — incomplete traces marked explicitly |
| OB-004 | Replay is deterministic under matching fingerprint + provider constraints |
| OB-005 | Audit entries are immutable once stored |
| OB-006 | Lineage tracks every data flow edge in the CEG |
| OB-007 | Unified Event Store is append-only |
| OB-008 | Observability never writes to Runtime Event Bus or Governance Event Stream |
| OB-009 | Data partitioned by execution_id |
| OB-010 | Trace retention policy configurable per execution mode |

---

## II.2 State Model (ADR-0005)

### II.2.1 Core Principle

```
Snapshot ≠ compressed trace
Snapshot = minimal RESUME state
Trace    = full REPLAY record
```

### II.2.2 Compression Strategy

| Data | In Snapshot? | Rationale |
|------|-------------|-----------|
| Node status map | FULL | Required to know terminal nodes |
| Scheduler state | FULL (cursor) | Required for resume position |
| Coordinator state | FULL | Required for lifecycle continuity |
| Agent state (inflight) | FULL | Required for mid-agent resume |
| Agent state (completed) | SUMMARY | Only outputs + artifacts |
| Tool results (completed) | SUMMARY | Only status + output refs |
| Full tool invocation history | OMIT | Reconstructible from traces |
| Lineage graph | OMIT | Reconstructible from CEG |
| Trace data (ADR-0004) | REFERENCE | trace_ref.last_trace_id only |

### II.2.3 Snapshot Schema

```yaml
snapshot:
  snapshot_id: "<uuid>"
  execution_plan_hash: "<sha256>"
  rir_hash: "<sha256>"
  compiler_version: "<semver>"
  runtime_fingerprint:
    plugin_versions: {}
    adapter_versions: {}
    scheduler_version: "<semver>"
  execution_mode: "3"
  schema_version: "1.0"
  status_map: [{ node_id, status, outputs, artifacts, evidence }]
  agent_state: [{ agent_node_id, plan, context_summary, variables }]
  scheduler_cursor: { current_layer, current_wave, completed_nodes, ready_queue, blocked_nodes }
  coordinator_state: { runtime_state, signal_origin }
  trace_ref: { execution_id, last_trace_id }
  context_snapshot: { session_refs, memory_refs, artifact_refs, variables }
```

### II.2.4 Recovery Protocol

```
RECOVER(snapshot_id):
  1. LOAD snapshot
  2. VALIDATE: execution_plan_hash, rir_hash, compiler_version,
     runtime_fingerprint, execution_mode, schema_version
  3. RESTORE status_map → set all node states
  4. RESTORE agent_state → rebuild inflight agent contexts
  5. RESTORE scheduler_cursor → rebuild wave position
  6. REBUILD wave partition from CEG + status_map + scheduler_cursor
     (deterministic: sort by node_id + rir_hash tie-breaker, SS-012)
  7. RESUME from next wave
```

### II.2.5 Rollback Model

```
ROLLBACK(execution_id, target_wave_id):
  1. VALIDATE: target_wave_id < current_wave_id
  2. LOAD pre-wave snapshot (auto-captured before each wave)
  3. RESTORE status_map to pre-wave state
  4. RESTORE agent states for agents active in target wave
  5. RESET scheduler cursor to target_wave_id
  6. EMIT ROLLBACK_MARKER (overlay, traces NOT modified — OB-005)
  7. RESUME from target wave

Constraints:
  - Max 3 rollbacks per wave (RB-002)
  - Rollback always to wave boundary (RB-001)
  - CEG never mutated (RB-006)
```

### II.2.6 State Invariants (SS-001 through SS-012)

| ID | Invariant |
|----|-----------|
| SS-001 | Snapshot stores minimum resume state — not full execution history |
| SS-002 | Recovery never replays tool invocations or agent executions |
| SS-003 | Recovery never re-derives scheduler state — loads exact cursor |
| SS-004 | Restore requires full hash + runtime fingerprint compatibility |
| SS-005 | Rollback bounded — max 3 per wave |
| SS-006 | Rollback uses overlay markers — traces remain immutable (OB-005) |
| SS-007 | Compression never loses information required for deterministic resume |
| SS-008 | Pre-wave snapshots captured automatically in Mode 3 |
| SS-009 | Cross-execution rollback forbidden |
| SS-010 | Rollback resets cursor only — CEG never mutated |
| SS-011 | Snapshot is materialized derivation of (CEG + traces + scheduler state); not independent truth source |
| SS-012 | Scheduler rebuild is deterministic (sort by node_id + rir_hash) |

---

## II.3 Consistency Controller (ADR-0006)

### II.3.1 Core Purpose

```
The Execution Consistency & Convergence Controller (ECCC)
validates that Snapshot ↔ Trace ↔ Replay/Recovery
remain semantically equivalent within deterministic bounds.

It performs STRUCTURAL DIFFERENTIAL COMPARISON only.
Severity interpretation and resolution are EXTERNAL concerns.
```

### II.3.2 Drift Classes

| Class | Detection |
|-------|-----------|
| **Snapshot Drift** | Snapshot state ≠ valid projection of trace state |
| **Replay Drift** | Replayed trace ≠ original trace (under deterministic constraints) |
| **Recovery Drift** | Recovered execution ≠ valid continuation of original |

### II.3.3 Components

| Component | Responsibility |
|-----------|---------------|
| **Trace Checker** | Internal trace consistency (no gaps, valid transitions) |
| **Snapshot Check** | Snapshot is correct projection of traces |
| **Replay Verifier** | Compares replayed outputs against original traces |
| **Drift Differential Engine** | Structural diff only — emits divergence class + raw expected/actual |
| **Drift Event Bus** | Immutable append-only stream of drift events |

### II.3.4 Drift Event Schema

```yaml
drift_event:
  drift_id: "<uuid>"
  type: "SNAPSHOT_DRIFT" | "REPLAY_DRIFT" | "RECOVERY_DRIFT" | "TRACE_CONSISTENCY"
  subtype: "STATUS_MISMATCH" | "OUTPUT_DIVERGENCE" | "SEQUENCE_GAP" | ...
  divergence_class: "STRUCTURAL" | "TEMPORAL" | "SEQUENCE" | "UNSPECIFIED"
  context:
    deterministic_bounded: true | false
    rir_hash, compiler_version, runtime_fingerprint
  resolution_contract_ref: "<ref>" | null
  immutable: true
```

**Key rule (EC-011):** Replay drift only valid when execution was declared deterministic-bounded under ADR-0004 §6.3. Otherwise → UNSPECIFIED_VARIANCE.

### II.3.5 ECCC Invariants (EC-001 through EC-012)

| ID | Invariant |
|----|-----------|
| EC-001 | Trace is sole authoritative source of execution truth |
| EC-002 | Snapshot must be derivable from (CEG + traces + scheduler state) |
| EC-003 | Replay with identical fingerprint must produce identical trace graph |
| EC-004 | Recovery must preserve completed node outputs, wave boundaries, dependency closure |
| EC-005 | All drift must be explicitly emitted — no silent correction |
| EC-006 | Drift detection is read-only, non-intrusive |
| EC-007 | Controller never modifies execution state, snapshots, traces, or CEG |
| EC-008 | Controller operates post-facto or in parallel; never on critical execution path |
| EC-009 | Drift detection is deterministic — same inputs → same divergence class |
| EC-010 | Drift events are immutable audit artifacts |
| EC-011 | Replay drift only valid when deterministic-bounded; otherwise UNSPECIFIED_VARIANCE |
| EC-012 | Every DRIFT_EVENT includes resolution_contract_ref (nullable) |

---

# BOOK III — FORMAL ABSTRACTION

## III.1 Formal Verification Bridge (ADR-0007)

### III.1.1 Core Purpose

The Formal Verification Bridge translates runtime artifacts (CEG, traces, snapshots, drift events) into a mathematically analyzable state-transition system — the Formal Execution Graph (FEG).

### III.1.2 Formal Execution Graph

```
FEG = (S, T, δ, s₀, F)

S  = finite set of abstract system states
T  = finite set of transition labels
δ  = S × T → S  (deterministic transition function)
s₀ ∈ S          (initial state)
F  ⊆ S          (terminal/accepting states)

System class:
  determinism:      "deterministic"
  state_space:      "bounded_finite"
  concurrency:      "wave_synchronized"
```

### III.1.3 Abstraction Mapping (α/γ)

```
Abstraction function:
  α: ConcreteExecution → FEG

Concretization function:
  γ: FEG → ℘(ConcreteExecution)

Soundness condition (theorem):
  ∀ execution e: behaviors(e) ⊆ behaviors(γ(α(e)))

Corollary (property preservation):
  γ(α(e)) ⊨ P ⇒ e ⊨ P
```

### III.1.4 Verification Properties

| Property | Type | Example Formula |
|----------|------|-----------------|
| Safety | LTL □ | □(executing → deps_satisfied) |
| Liveness | LTL ◇□ | □(executing → ◇ terminal) |
| Determinism | Structural | FEG(E₁) ≅ FEG(E₂) |
| Convergence | DAG | All paths reach F |

### III.1.5 Expressiveness Boundary

```
VERIFIABLE (within FEG):
  ✓ Node execution order respects dependency graph
  ✓ No duplicate execution
  ✓ All nodes reach terminal state
  ✓ Wave/layer progression is monotonic
  ✓ Commit sequence is gap-free and total

NON-VERIFIABLE (outside FEG):
  ✗ Semantic correctness of tool outputs
  ✗ Probabilistic model behavior (temperature > 0)
  ✗ External system effects
  ✗ Real-world I/O truthfulness

ADR-0007 proves STRUCTURAL correctness only.
SEMANTIC correctness is externally validated.
```

### III.1.6 Verification Contract

```yaml
verification_contract:
  contract_id: "<uuid>"
  execution_id: "<uuid>"
  feg:
    state_count, transition_count, depth, has_cycles: false
  properties:
    safety: [{ name, formula, type }]
    liveness: [{ name, formula, type }]
    determinism: { comparator_execution_id, isomorphic }
    convergence: { all_paths_terminal, max_path_length }
  proof_results:
    model_checker: { tool, result, counterexample_trace }
    smt_solver: { tool, result, model }
    theorem_prover: { tool, result, proof_script }
  runtime_fingerprint:
    plugin_versions, adapter_versions, scheduler_version
```

### III.1.7 FVB Invariants (FV-001 through FV-010)

| ID | Invariant |
|----|-----------|
| FV-001 | FEG construction is deterministic |
| FV-002 | FEG never contains cycles |
| FV-003 | Every FEG transition maps to exactly one commit event |
| FV-004 | FEG abstraction preserves execution order and terminal status |
| FV-005 | Verification Bridge never executes runtime logic or modifies runtime state |
| FV-006 | Drift-annotated transitions marked in FEG but do not block verification |
| FV-007 | Verification Contract immutable once generated |
| FV-008 | FEG construction is post-facto |
| FV-009 | FEG is sound abstraction — property preservation via α/γ |
| FV-010 | External provers consume only Verification Contract |

---

# BOOK IV — COMPOSITIONAL CALCULUS

## IV.1 Compositional Verification (ADR-0008)

### IV.1.1 Core Purpose

Enable proof of system-wide properties by proving properties of sub-FEG components and composing those proofs using a formal composition algebra.

### IV.1.2 Composition Operators

| Operator | Name | Use |
|----------|------|-----|
| A ≫ B | Sequential | Agent handoff |
| A ∥ B | Parallel (conflict-free) | Same-wave agents |
| A ▷_p B | Conditional | Gate branching, ESCALATE |
| A* | Bounded iteration | Agent replan loop |

### IV.1.3 Compositional Closure Theorem

```
Class C = {FEG | deterministic, bounded_finite, wave_synchronized}

Theorem: Each operator (≫, ∥, ▷, *) preserves C.
By structural induction: any well-formed composition of atomic FEGs ∈ C.

Parallel composition requires: conflict_free(A, B)
Iteration requires: finite bound N
```

### IV.1.4 Assume-Guarantee Contracts

```yaml
ag_contract:
  contract_id: "<uuid>"
  component_id: "<feg-ref>"
  assume:
    preconditions: [...]
    state_predicates: [...]
  guarantee:
    postconditions: [...]
    safety: [...]
    liveness: [...]
```

**Satisfaction (⊨):** Evaluated over FEG state space — decidable, bounded_finite.

**Composition rule (AG-COMP):**

```
(E ⊨ Aₐ) ∧ (A ⊨_Aₐ Gₐ) ⇒ (E ∥ A) ⊨ Gₐ
```

### IV.1.5 Proof Lattice

```yaml
proof_lattice:
  root: "<verification_contract_id>"
  nodes:
    - contract_id, scope, verified: true | false | partial
    - depends_on: [...]   # parent proofs
    - composed_of: [...]  # child proofs
  edges:
    - from, to, operator: "≫" | "∥" | "▷"
```

**Stability condition:** A verified (sub-FEG, property) pair remains verified iff: F is unchanged, all dependencies are unchanged, and the composition operator preserves C.

### IV.1.6 Cross-Execution Composition

Compose FEGs from different executions (Mode 2 → Mode 3 escalation). Requires state equivalence at handoff: FEG₁.terminal ≅ FEG₂.snapshot under α abstraction.

### IV.1.7 CV Invariants (CV-001 through CV-010)

| ID | Invariant |
|----|-----------|
| CV-001 | Composition operators preserve FEG class (proven via Closure Theorem) |
| CV-002 | AG contracts verified at composition time — mismatch = REJECTED |
| CV-003 | Proof lattice verification is monotonic |
| CV-004 | Compositional verification is sound — verified components compose to verified system |
| CV-005 | Cross-execution composition requires state equivalence at handoff |
| CV-006 | Parallel composition requires conflict-free sub-FEGs |
| CV-007 | Lattice update is incremental — only ancestors re-verified |
| CV-008 | Invariant synthesis is deterministic |
| CV-009 | Bounded iteration preserves termination |
| CV-010 | Composition never introduces new behaviors outside interleaving of components |

---

# BOOK V — UNIFIED INVARIANT SYSTEM

## V.1 Invariant Taxonomy

All invariants from Books I–IV are unified into a single namespace.

```
RI-*   Runtime Invariants          (20 — ADR-0003-B)
TI-*   Tool Invariants             (12 — ADR-0003-D)
AI-*   Agent Invariants            (12 — ADR-0003-C)
GI-*   Governance Invariants       (22 — ADR-0003-E)
OB-*   Observability Invariants    (10 — ADR-0004)
SS-*   State Model Invariants      (12 — ADR-0005)
EC-*   Consistency Invariants      (12 — ADR-0006)
FV-*   Formal Bridge Invariants    (10 — ADR-0007)
CV-*   Compositional Invariants    (10 — ADR-0008)

TOTAL: 120 invariants across 5 books
```

## V.2 Cross-Layer Constraint Map

| Constraint | Layer A | Layer B | Relationship |
|------------|---------|---------|-------------|
| SS-011 + EC-001 | State Model | Consistency | Snapshot is derivation of traces; traces are authoritative |
| RI-012 + SS-004 + FV-009 | Runtime | State | Snapshot restore → hash + fingerprint validation → sound abstraction |
| GI-014 + TI-002 + AI-005 | Governance | Adapters | No synchronous governance calls; local contract enforcement only |
| GI-009 + GI-013 + OB-008 | Governance | Observability | Event streams separated; Signal Adapter is sole bridge |
| RI-009 + all node transitions | Runtime | All layers | Commit Engine is sole node state writer |
| EC-011 + ADR-0004 §6.3 | Consistency | Observability | Replay drift only under deterministic-bounded constraints |
| OB-005 + SS-006 | Observability | State | Traces immutable; rollback uses overlay markers |
| GI-015 + RI-006 | Governance | Runtime | Mandatory signals must be honored; failure → ABORT |
| FV-001 + CV-001 + CV-004 | Formal | Compositional | FEG deterministic → Closure → Composition sound |

## V.3 Global System Guarantees

```
G1. DETERMINISM:
    Same RIR + same runtime fingerprint + same provider constraints
    → identical execution (traces, FEG, verification results)

G2. ISOLATION:
    Plugin failures never crash the core.
    Agent failures never cascade across CEG nodes.
    Sandbox violations produce FAILED node state (never SKIPPED).

G3. AUDITABILITY:
    Every node transition is journaled (RI-016).
    Every governance decision is durably recorded (GI-016).
    Every drift detection is immutable (EC-010).
    All artifacts form a complete, replayable audit trail.

G4. FAIL-CLOSED:
    Compiler: any ambiguity → compile failure (CR-003).
    Governance: unavailable → execution rejected (GI-001).
    Admission: unvalidated → execution denied.
    Mandatory signal: ignored → ABORT (GI-015).

G5. STRUCTURAL CORRECTNESS:
    FEG proves execution structure, order, and dependencies are correct.
    Semantic correctness of outputs is out of scope (§III.1.5).
    The system is sound without claiming semantic omniscience.

G6. COMPOSITIONAL VERIFICATION:
    Sub-FEG proofs compose to system-wide proofs.
    Incremental verification: changed components only re-verify ancestors.
    Closure under ≫ ∥ ▷ * is formally proven.
```

---

## V.4 Specification Metadata

```yaml
specification:
  name: "AI Workstation Core"
  version: "1.0"
  status: "CANONICAL_REFERENCE"
  immutability: "LOCKED"
  source_adrs:
    - "ADR-0001: Canonical Architecture"
    - "ADR-0002: Compiler System (A through F)"
    - "ADR-0003-A: Execution Semantics"
    - "ADR-0003-B: Runtime Engine"
    - "ADR-0003-C: Agent Adapters"
    - "ADR-0003-D: Tool Adapters"
    - "ADR-0003-E: Governance Engine"
    - "ADR-0004: Observability Layer"
    - "ADR-0005: State Model"
    - "ADR-0006: Consistency Controller"
    - "ADR-0007: Formal Verification Bridge"
    - "ADR-0008: Compositional Verification"
  total_invariants: 120
  books: 5
  architectural_boundary: "STRUCTURAL — semantic correctness is external"
  readiness: "IMPLEMENTATION_READY | FORMAL_VERIFICATION_READY"
```

---

**END OF SPECIFICATION v1.0**

This document is the single source of truth for the AI Workstation Core system. All 14 ADRs are collapsed into this canonical reference. No ADR has been redesigned — only recomposed into a unified layered specification with consistent naming, cross-referenced invariants, and a single entry point for tooling, implementation, and formal verification.
