# MEK — Minimal Executable Kernel v1.0

**Status:** CANONICAL REFERENCE
**Source:** Extracted from System Specification v1.0, Book I
**Principle:** Executable projection — not reduced documentation. This is the first physically instantiable semantics of the architecture.

---

## 0. Identity

```
MEK is the Minimal Executable Kernel.
It is the lossless executable projection of the runtime subset of Book I.

MEK defines WHAT executes.
It does not define WHY (Governance), HOW IT IS RECORDED (Observability),
or HOW IT IS PROVEN (Formal Bridge).

MEK ⊂ Book I ⊂ System Spec v1.0
MEK ≠ simplified Book I
MEK = semantically sliced along the execution boundary
```

---

## 1. Kernel Architecture

### 1.1 Components

```
┌─────────────────────────────────────────────────────────┐
│ MEK (Minimal Executable Kernel)                          │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌─────────┐  │
│  │RIR LOADER│  │   CEG    │  │SCHEDULER │  │ COMMIT  │  │
│  │          │  │INTERPRETER│  │(wave eng)│  │ ENGINE  │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬────┘  │
│       │              │             │             │       │
│       └──────────────┼─────────────┼─────────────┘       │
│                      │             │                     │
│                 ┌────▼─────────────▼──────┐              │
│                 │     DISPATCHER          │              │
│                 │  (tool + agent invoke)  │              │
│                 └────────────┬────────────┘              │
│                              │                           │
│                 ┌────────────▼────────────┐              │
│                 │ EXECUTION CONTEXT STORE │              │
│                 │ (ephemeral, per-exec)   │              │
│                 └─────────────────────────┘              │
└─────────────────────────────────────────────────────────┘
```

| # | Component | Responsibility | State |
|---|-----------|---------------|-------|
| 1 | **RIR Loader** | Parse and validate RIR artifact | Immutable input |
| 2 | **CEG Interpreter** | Build DAG, walk graph, evaluate activation | Structural graph (immutable) |
| 3 | **Scheduler** | Compute wave partition, claim nodes, issue DispatchClaim | Wave position, READY set |
| 4 | **Commit Engine** | **Sole writer** of all node state transitions | Node status map |
| 5 | **Dispatcher** | Resolve adapter, invoke execution, normalize results | Ephemeral (results only) |
| 6 | **Execution Context Store** | Hold per-execution variables, artifacts, context | Ephemeral (lost on termination) |

### 1.2 What MEK Explicitly Excludes

```
MEK does NOT contain:

  ✗ Governance Engine        (policy, admission, escalation contracts)
  ✗ Observability Layer      (traces, audit, replay)
  ✗ Formal Bridge (FEG)      (α/γ mapping, verification contract)
  ✗ Compositional Calculus   (≫ ∥ ▷ *, AG contracts, proof lattice)
  ✗ Snapshot Engine           (persist/restore — Mode 3 only)
  ✗ Execution Journal         (durable audit — passive layer)
  ✗ Plugin Registry            (static config for MEK)
  ✗ Capability Governor        (contract enforced by Dispatcher from static config)
  ✗ Admission Controller       (all executions are pre-admitted in MEK)
  ✗ Risk Engine                (advisory only)
  ✗ Policy Evaluator           (control plane)
```

---

## 2. Execution Pipeline

### 2.1 Main Loop

```
MEK_RUN(rir_path):
  1. RIR ← RIR_LOAD(rir_path)
  2. CEG ← CEG_BUILD(RIR.graph)
  3. CEG_VALIDATE(CEG)                    # V-001 through V-014
  4. WAVES ← SCHEDULE_COMPUTE_WAVES(CEG, RIR)
  5. STATUS_MAP ← INIT_STATUS_MAP(CEG)
  6. SCHEDULER_CURSOR ← { layer: 0, wave: 0 }

  7. WHILE ¬TERMINAL(STATUS_MAP, CEG):
       for each lane ℓ in topological layer order:
         for each wave w in WAVES[layer=ℓ]:
           READY ← SCHEDULER_CLAIM(w)
           for each node in READY (concurrently):
             node.status ← RUNNING          # via Commit Engine
             result ← DISPATCH(node)
             node.status ← result.status    # via Commit Engine
             if result.escalation:
               MEK_TERMINATE("gate_escalation")
               metrics.escalation_requested ← true
               goto TERMINATION
           SCHEDULER_WAIT_WAVE(w)
         SCHEDULER_RECOMPUTE()

  TERMINATION:
  8. RETURN STATUS_MAP, metrics

MEK_TERMINATE(reason):
  # External TERMINATE signal or ESCALATE gate branch.
  # Drains the current wave (in-flight nodes complete),
  # then sets all remaining non-terminal nodes → TERMINATED.
  for each node in current wave where status ∈ {RUNNING}:
    wait for completion (best-effort, bounded by deadline)
  for each v in CEG.V:
    if status_map[v] ∉ {SUCCESS, FAILURE, SKIPPED, TERMINATED}:
      COMMIT(v, TERMINATED, result=null)
  termination_reason ← reason
```

### 2.2 Node Status Lattice

```
BLOCKED | READY | RUNNING | SUCCESS | FAILURE | SKIPPED | TERMINATED

Terminal: SUCCESS, FAILURE, SKIPPED, TERMINATED
Non-terminal: BLOCKED, READY, RUNNING

INIT_STATUS_MAP(CEG):
  for each node v in CEG.V:
    if indegree(v) = 0:
      if CEG_EVALUATE_ACTIVATION(v, empty_status_map) = true:
        v.status ← READY
      else:
        v.status ← SKIPPED         # root conditional with false predicate
    else:
      v.status ← BLOCKED

PENDING is NOT a state. It has been eliminated.
Root nodes start READY (or SKIPPED if activation is permanently unsatisfiable).
All others start BLOCKED.
No node begins in a state with no exit transition.
```

### 2.3 Terminal Condition

```
TERMINAL(status_map, CEG):
  All nodes in CEG have status ∈ {SUCCESS, FAILURE, SKIPPED, TERMINATED}
  OR
  MEK received external TERMINATE signal
```

### 2.4 State Transitions (all via Commit Engine)

```
READY     → RUNNING    (Commit Engine, on DispatchClaim from Scheduler)
RUNNING   → SUCCESS    (Commit Engine, on ExecutionResult from Dispatcher)
RUNNING   → FAILURE    (Commit Engine, on ExecutionResult from Dispatcher)
BLOCKED   → READY      (Commit Engine, on recompute from Scheduler)
BLOCKED   → SKIPPED    (Commit Engine, on recompute from Scheduler)
*         → TERMINATED  (Commit Engine, on TerminateRequest — external signal)
```

---

## 3. Component Specifications

### 3.1 RIR Loader

```
RIR_LOAD(path) → RIR:
  1. Parse RIR JSON from path
  2. Validate schema_version = "1.0"
  3. Validate execution_mode = "2"        # MEK v1 supports Mode 2 only
     If execution_mode ≠ "2": ABORT("UNSUPPORTED_MODE: MEK v1 supports Mode 2 only")
  4. Validate RIR invariants (I-001 through I-006)
  5. Validate: no checkpoint-type nodes in units[]
     If any unit.type = "checkpoint": ABORT("CHECKPOINT_FORBIDDEN_IN_MEK")
  6. Return validated RIR

Failure: Any validation failure → ABORT (no partial load)
```

### 3.2 CEG Interpreter

```
CEG_BUILD(rir_graph) → CEG:
  1. Extract nodes V from rir_graph.dag.nodes
  2. Extract edges E from rir_graph.dag.edges
  3. Classify each node by type (agent, capability, task, gate)
     # checkpoint nodes are FORBIDDEN in MEK v1 (Mode 2 only)
  4. Build dependency closure R (transitive closure of E)
  5. Compute topological ordering τ
  6. Validate: V-001 (no cycles), V-003 (indegree ≤ 64), V-004 (depth ≤ 128)
  7. Return CEG

CEG is IMMUTABLE after construction. No component may modify it.

CEG_EVALUATE_ACTIVATION(node, status_map) → bool:
  if node.activation.condition = "all_success":
    return ∀ p ∈ node.activation.requires: status_map[p] = SUCCESS
  if node.activation.condition = "any_success":
    return ∃ n ∈ (node.activation.requires ∪ node.activation.optional):
           status_map[n] = SUCCESS
  if node.activation.condition starts with "conditional:":
    return EVALUATE_PREDICATE(node.activation.condition, status_map)
```

### 3.3 Scheduler

```
SCHEDULE_COMPUTE_WAVES(CEG, RIR) → Waves:
  1. For each topological layer ℓ:
     a. Extract nodes in layer ℓ
     b. Build conflict graph: edge between (a, b) if
        - SIDE_EFFECT_CONFLICT(a, b)  # overlapping side_effect_surface
        - OR DATA_FLOW_CONFLICT(a, b) # shared data contract
     c. Color conflict graph (deterministic, node-id-ordered)
     d. Each color class = one wave
     e. Waves ordered by color index (ascending)
  2. Return Waves[layer][wave_index] → [node_id]

SCHEDULER_CLAIM(wave) → READY_nodes:
  For each node in wave where node.status = READY:
    Mark node as CLAIMED
    Issue DispatchClaim to Commit Engine
  Return CLAIMED nodes

SCHEDULER_WAIT_WAVE(wave):
  Block until ALL nodes in wave have terminal status.
  Timeout: deadline is measured from wave start (not from individual
  node RUNNING). `scheduling.deadline` is a compile-time field on
  every node and applies regardless of whether the node ever entered
  RUNNING. A BLOCKED conditional node that never activates will hit
  the wave deadline and trigger TERMINATE — this is the termination
  guarantee for nodes whose predicates are permanently unsatisfiable.
  On expiry: all non-terminal nodes in wave → TERMINATED.

  Wave deadlines are COLLECTIVE. Expiry of a wave deadline
  terminates ALL non-terminal nodes in the wave, including nodes
  that are otherwise progressing normally. This behavior is
  intentional and preserves wave atomicity (M-007) and bounded
  termination (G4).

SCHEDULER_RECOMPUTE():
  Subscribe to NodeCommitted events
  For each node v where status = BLOCKED (node-id-ordered):
    if CEG_EVALUATE_ACTIVATION(v, status_map) = true:
      Issue status transition BLOCKED → READY to Commit Engine
    else if v.activation.condition = "all_success":
      # A single failed/skipped required predecessor → SKIPPED
      if ∃ p ∈ v.activation.requires:
         status_map[p] ∈ {FAILURE, SKIPPED, TERMINATED}:
        Issue status transition BLOCKED → SKIPPED to Commit Engine
     else if v.activation.condition = "any_success":
       # ALL candidate predecessors terminal AND NONE SUCCESS → SKIPPED
       candidates ← v.activation.requires ∪ v.activation.optional
       if ALL c ∈ candidates are terminal
          AND NONE c ∈ candidates has status = SUCCESS:
         Issue status transition BLOCKED → SKIPPED to Commit Engine
     else if v.activation.condition starts with "conditional:":
       # Conditional predicates are black-box arbitrary functions.
       # Provability of permanent unsatisfiability is undecidable.
       # The node remains BLOCKED. The deadline mechanism
       # (SCHEDULER_WAIT_WAVE timeout) handles non-termination.
       # NO automatic SKIP for conditional nodes.
       pass
```

**Note on conditional nodes:** Unlike `all_success`/`any_success` (monotonic functions of SUCCESS membership, provably satisfiable or not), conditional predicates are arbitrary and may depend on context beyond `requires` alone (optional predecessors, external data, etc.). Proving that a conditional predicate "will never become true" is equivalent to the halting problem. MEK does not attempt this. The `deadline` mechanism in `SCHEDULER_WAIT_WAVE` is the termination guarantee for nodes that block indefinitely.

**Determinism guarantee:** `SCHEDULE_COMPUTE_WAVES` is deterministic — same CEG + same RIR → identical wave structure. `SCHEDULER_RECOMPUTE` uses node-id-ordered evaluation for determinism.

### 3.4 Commit Engine

```
COMMIT_ENGINE is the SOLE WRITER of all node state.

COMMIT(node_id, new_status, result):
  LOCK node_id
  VALIDATE transition is legal:
    valid_transitions = {
      READY → RUNNING,
      RUNNING → SUCCESS,
      RUNNING → FAILURE,
      BLOCKED → READY,
      BLOCKED → SKIPPED,
      * → TERMINATED
    }
    if transition ∉ valid_transitions: ABORT("INVALID_TRANSITION")
  APPLY:
    node.status ← new_status
    if result: node.outputs ← result.outputs
    if result: node.artifacts ← result.artifacts
  UNLOCK node_id
  PUBLISH NodeCommitted(node_id, new_status)

DOUBLE_COMMIT_GUARD:
  if node.status already terminal and new attempt to commit:
    REJECT with WARNING (idempotent, not ABORT)
```

### 3.5 Dispatcher

```
DISPATCH(node) → ExecutionResult:
  # Gate nodes: native execution — no adapter, no sandbox, no provider
  if node.type = "gate":
    branch ← EVALUATE_PREDICATE(node.gate.branches)
    if branch.target = "ESCALATE":
      # ESCALATE is a reserved gate branch target (ADR-0003-A §6).
      # Valid only in execution_mode="2" — which is exactly MEK v1's
      # sole supported mode. MEK treats ESCALATE as an implicit
      # TERMINATE signal: all non-terminal nodes → TERMINATED.
      # MEK does NOT interpret the policy meaning of escalation —
      # it leaves reclassification to the external adaptive-router
      # (F-004). MEK only records escalation_requested in metrics.
      emit ESCALATION_REQUESTED
      return ExecutionResult(status="terminated", escalation=true)
    return synthetic ExecutionResult(selected_branch=branch.target)

  # All other node types: adapter-based execution
  1. RESOLVE adapter for node.type + node.binding.contract
  2. ENFORCE isolation:
     - required ← REGION_TO_ADAPTER_ISOLATION(node.region.isolation_requirement)
     - VALIDATE: adapter.isolation ≥ required (per isolation ordering)
     - If adapter.isolation < required: FAIL("INSUFFICIENT_ISOLATION")
  3. APPLY constraints from adapter_config:
     - timeout ← min(node.scheduling.deadline, adapter.timeout_ms)
     - filesystem_scope ← adapter.filesystem_access
     - network_scope ← adapter.network_access
  4. EXECUTE:
     result ← adapter.execute(node, context)
     with timeout enforcement (SIGKILL on expiry)
  5. NORMALIZE result per node type:
     - agent:      aggregate tool results → agent status
     - capability: raw result status
     - task:       all sub-units must be SUCCESS
  6. RETURN ExecutionResult

# Region isolation requirement → minimum adapter isolation
# isolation_requirement is a SECURITY/ISOLATION constraint axis.
# It is NOT a resource/performance axis.
REGION_TO_ADAPTER_ISOLATION:
  no_shared_state   → process
  no_network        → process
  no_filesystem     → process
  full_sandbox      → container

# Isolation ordering: used for ≥ comparison
inline < process < container

DISPATCH_CANCEL(node_id):
  Best-effort kill of inflight adapter process.
  Node status is set TERMINATED immediately regardless.
```

### 3.6 Execution Context Store

```
CONTEXT is ephemeral — lives only for the duration of MEK_RUN.

CONTEXT_STORE:
  variables: { "<key>": "<value>" }     # scoped runtime variables
  artifacts: ["<ref>"]                  # references to produced artifacts
  session_data: {}                      # per-node session data (agent state)

CONTEXT is:
  - Per-execution (no cross-execution state)
  - Not persisted (no snapshot in MEK)
  - Passed to adapters as invocation context
```

---

## 4. MEK Invariants

### 4.1 Execution Integrity

| ID | Invariant |
|----|-----------|
| M-001 | Commit Engine is the only state mutator — no other component writes node status |
| M-002 | CEG is immutable after CEG_BUILD |
| M-003 | No node executes outside READY state |
| M-004 | No execution bypasses Scheduler → Dispatcher → Commit Engine pipeline |
| M-005 | All state transitions are deterministic given identical RIR |

### 4.2 Scheduling Determinism

| ID | Invariant |
|----|-----------|
| M-006 | Wave partition is deterministic — same CEG → same wave structure |
| M-007 | No cross-wave execution overlap — wave N+1 starts only after wave N completes |
| M-008 | Scheduler recompute uses node-id-ordered evaluation for determinism |

### 4.3 Isolation

| ID | Invariant |
|----|-----------|
| M-009 | Each node executes in isolated context |
| M-010 | No shared mutable execution state between concurrently executing nodes. Provider internal state (connection pools, immutable caches, transport reuse) is excluded ONLY if it cannot produce observable nondeterminism in node execution. Any provider state that affects output determinism violates M-010. |
| M-011 | Dispatcher cannot mutate CEG or Scheduler state |

### 4.4 Failure Semantics

| ID | Invariant |
|----|-----------|
| M-012 | Node FAILURE is terminal — no implicit retry inside MEK |
| M-013 | System termination occurs only via terminal closure OR external TERMINATE signal |
| M-014 | No partial graph corruption — if Commit Engine ABORTs, MEK terminates |

### 4.5 Control Boundary

| ID | Invariant |
|----|-----------|
| M-015 | No governance evaluation inside MEK |
| M-016 | No observability instrumentation inside MEK |
| M-017 | No formal verification logic inside MEK |
| M-018 | MEK operates on pre-compiled RIR — no Compiler inside MEK |

---

## 5. Adapter Interface (MEK Boundary)

### 5.1 Adapter Contract

MEK defines a minimal adapter interface. Adapters are loaded from static configuration — not from a plugin registry.

**Isolation ordering (enforced at DISPATCH):**

```
inline < process < container

Where:
  inline    = in-process function call (no sandbox; only for idempotent, side-effect-free tools)
  process   = separate OS process (seccomp/apparmor)
  container = OCI container (Docker/Podman)

adapter.isolation ≥ node.region.isolation_requirement
means: the adapter's isolation level is STRICTER THAN OR EQUAL TO
the region's requirement. A node requiring container isolation
cannot be served by an inline adapter.
```

```yaml
adapter_config:
  agent_adapters:
    - agent_ref: "<agent-ref>"
      type: "reactive" | "planned" | "autonomous"
      provider: "<provider-id>"
      allowlist: ["<tool-ref>"]
      max_tool_invocations: <int>
      max_duration_ms: <int>
      isolation: "inline" | "process" | "container"
      filesystem_access: "none" | "read_only:<path>" | "scoped:<path>"
      network_access: "none" | "restricted:<domain-list>" | "full"
      side_effect_surface: ["<surface-ref>"]

  tool_adapters:
    - tool_ref: "<tool-ref>"
      provider: "<provider-id>"
      idempotent: true | false
      timeout_ms: <int>
      isolation: "inline" | "process" | "container"
      filesystem_access: "none" | "read_only:<path>" | "scoped:<path>"
      network_access: "none" | "restricted:<domain-list>" | "full"
      side_effect_surface: ["<surface-ref>"]

  providers:
    - provider_id: "<id>"
      type: "llm" | "cli" | "api" | "internal"
      config: {}                         # provider-specific config
```

### 5.2 Adapter Execution Interface

Every adapter MUST implement:

```
execute(inputs: Map<Key, Value>, context: ExecutionContext) → ExecutionResult
```

**ExecutionResult:**

```yaml
execution_result:
  status: "success" | "failure" | "timeout" | "cancelled" | "terminated"
  outputs: { "<key>": "<value>" }
  artifacts: ["<ref>"]
  escalation: true | null            # optional, gate-only. Present when a gate
                                     # selects the reserved ESCALATE branch target.
                                     # Triggers MEK_TERMINATE in the main loop.
  metrics:
    duration_ms: <int>
    tokens_used: <int> | null
```

### 5.3 Built-in Adapters (MEK v1)

MEK ships with two built-in adapters for bootstrapping:

| Adapter | Type | Description |
|---------|------|-------------|
| `internal/noop` | tool | Returns success immediately — used for testing only |
| `internal/echo` | tool | Returns inputs as outputs — used for testing |

**Gate nodes do NOT use adapters.** Gate execution is native to the Dispatcher — it evaluates the predicate, selects the branch, and returns a synthetic ExecutionResult. No adapter execution, no provider call, no sandbox.

**Checkpoint nodes are FORBIDDEN in MEK v1.** RIR_LOAD rejects any RIR containing checkpoint-type units. MEK v1 is Mode 2 only — checkpoints require Mode 3 (Snapshot Engine).

All other adapters are loaded from `adapter_config`.

---

## 6. MEK Invocation

### 6.1 CLI Interface

```
mek run <rir_path> [--adapter-config <path>] [--timeout <ms>]
```

### 6.2 Programmatic Interface

```typescript
interface MEK {
  run(rir: RIR, config: AdapterConfig): Promise<StatusMap>;
  terminate(): void;
}
```

### 6.3 Output

```
MEK_RUN returns:
  status_map: Map<NodeId, { status, outputs, artifacts }>
  metrics: {
    total_duration_ms,
    nodes_executed,
    nodes_failed,
    waves_completed,
    escalation_requested: true | false   # set when a gate selects ESCALATE branch
  }
```

---

## 7. Error Handling

### 7.1 RIR Validation Failure

```
RIR fails schema validation → MEK aborts before any execution.
Error: { code: "INVALID_RIR", details: "<validation-error>" }
```

### 7.2 CEG Validation Failure

```
CEG has cycles OR violates structural constraints → MEK aborts.
Error: { code: "INVALID_CEG", details: "<validation-error>" }
```

### 7.3 Adapter Failure

```
Adapter timeout → node FAILED (M-012)
Adapter crash → node FAILED
Adapter returns invalid result → node FAILED
Sandbox violation → node FAILED
```

### 7.4 Commit Engine Failure

```
Invalid state transition attempted → MEK ABORT
Double commit attempt → WARNING (idempotent reject)
Internal inconsistency → MEK ABORT
```

### 7.5 External TERMINATE

```
MEK receives TERMINATE signal:
  - Current wave drains (in-flight nodes complete)
  - All non-terminal nodes → TERMINATED (via Commit Engine)
  - MEK returns partial status_map
```

---

## 8. What MEK Guarantees

```
G1. DETERMINISTIC: Same RIR + same adapter config → same execution result.
G2. ISOLATED: Node failures do not crash MEK.
G3. CONSISTENT: All state transitions flow through Commit Engine.
G4. BOUNDED: Every execution either reaches terminal closure or is TERMINATED.
G5. PURE: No side effects beyond adapter invocations.
G6. VERIFIABLE: Output status_map is sufficient to validate against CEG structure.
```

---

## 9. What MEK Does NOT Guarantee

```
N1. PERSISTENCE: No snapshot, no journal. MEK is ephemeral.
N2. AUDIT: No trace collection. Observability is external.
N3. RECOVERY: No resume from checkpoint. MEK runs once.
N4. GOVERNANCE: No policy evaluation. All executions are pre-admitted.
N5. FORMAL PROOF: No FEG construction. Verification is external.
N6. COMPOSITION: Single execution only. Cross-execution is external.
```

---

## 10. MEK Invariant Map to Book I

```
M-001  ↔ RI-009   (Commit Engine sole writer)
M-002  ↔ RI-001   (CEG immutability)
M-003  ↔ ADR-0003-A activation rule
M-004  ↔ RI-008   (no dispatch bypass)
M-005  ↔ RI-004   (determinism)
M-006  ↔ ADR-0003-A wave model
M-007  ↔ ADR-0003-A wave ordering
M-008  ↔ RI-010   (scheduler determinism)
M-009  ↔ TI-001   (isolation per invocation)
M-010  ↔ ADR-0003-A conflict model
M-011  ↔ RI-007   (dispatcher no CEG mutation)
M-012  ↔ ADR-0003-A failure model
M-013  ↔ ADR-0003-A terminal closure
M-014  ↔ RI-006   (ABORT integrity)
M-015  ↔ GI-001   (no governance in runtime)
M-016  ↔ OB-001   (no observability in execution path)
M-017  ↔ FV-005   (no formal logic in runtime)
M-018  ↔ IR-001   (compiler/runtime separation: no compiler in runtime) | BR-003 (agent execution ≠ system logic)
```

---

## 11. MEK Specification Metadata

```yaml
mek:
  name: "Minimal Executable Kernel"
  version: "1.0"
  status: "CANONICAL_REFERENCE"
  derived_from: "System Specification v1.0, Book I"
  invariants: 18 (M-001 through M-018)
  components: 6
  supported_modes: ["2"]
  node_types: ["agent", "capability", "task", "gate"]   # checkpoint FORBIDDEN
  isolation_ordering: "inline < process < container"
  excluded_layers:
    - "Governance Engine"
    - "Observability Layer"
    - "Formal Verification Bridge"
    - "Compositional Calculus"
    - "Snapshot Engine (Mode 3)"
    - "Execution Journal (durable audit)"
  guarantees: 6 (G1-G6)
  non_guarantees: 6 (N1-N6)
  node_status_lattice: "BLOCKED | READY | RUNNING | SUCCESS | FAILURE | SKIPPED | TERMINATED"
  pending_eliminated: true
  skipped_propagates: true
  readiness: "IMPLEMENTATION_READY"
  first_executable: true
```

---

**END OF MEK v1.0**

This document defines the first executable boundary of the AI Workstation Core. It is a runtime contract — not a documentation subset. Every invariant maps to a specific constraint in Book I. Every exclusion is deliberate. The MEK is the point where specification becomes execution.
