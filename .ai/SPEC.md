# MCP Control Plane Kernel — Formal Engineering Specification

> **Version:** v0.4.0  
> **Spec Status:** Reference specification. Implementation-authoritative.  
> **Target Runtime:** Go 1.22+ (goroutine workers, channel queues, context cancellation)  
> **Backward Reference:** Python v0.4.0 implementation at `runtime/` (behavior-verified)  
> **Governance:** PERP v2.4 | Fail-Closed | Deterministic | Event-Driven

---

## 1. System Definition

### 1.1 Identity

The MCP Control Plane Kernel is a **deterministic, event-driven control plane** that processes tool execution requests through a bounded queue with persistent worker consumers, strict policy enforcement, and full execution traceability.

### 1.2 What It Is NOT

| Misclassification | Correction |
|---|---|
| Pipeline system | The kernel uses an internal event-driven dispatch model. The pipeline is a *subcomponent* — not the architecture. |
| Orchestration framework | There is no workflow DAG, no step scheduler, no multi-stage coordinator. |
| Multi-agent system | One worker = one request at a time. No agent communication, no delegation. |
| Distributed system | Single-process. No RPC, no consensus, no leader election. |

### 1.3 System Boundaries

```
┌──────────────────────────────────────────┐
│              External World              │
│          (stdin / Unix socket)           │
└────────────────┬─────────────────────────┘
                 │ REQUEST_INGRESS
                 ▼
┌────────────────────────────────────────────┐
│            MCP Gateway Layer               │
│  ┌──────────────────────────────────────┐  │
│  │  Schema Validator                    │  │
│  │  RequestContext Factory              │  │
│  └──────────────┬───────────────────────┘  │
└─────────────────┼──────────────────────────┘
                  │
                  ▼
┌────────────────────────────────────────────┐
│            REQUEST QUEUE LAYER             │
│  Bounded FIFO chan[RequestContext]         │
│  Backpressure: chan full → REJECT          │
└─────────────────┬──────────────────────────┘
                  │ WAIT_EVENT
                  ▼
┌────────────────────────────────────────────┐
│             WORKER POOL LAYER              │
│  N persistent goroutines                   │
│  Each: for ctx := range queue { ... }      │
└─────────────────┬──────────────────────────┘
                  │ FETCH_REQUEST
                  ▼
┌────────────────────────────────────────────┐
│            POLICY ENGINE LAYER             │
│  Deterministic rule evaluation             │
│  Pre-execution gate                        │
│  Decision: ALLOW | DENY                    │
└─────────────────┬──────────────────────────┘
                  │ POLICY_EVALUATION
                  ▼
┌────────────────────────────────────────────┐
│           EXECUTION CORE LAYER             │
│  Isolated tool execution                   │
│  Timeout boundary: context.WithTimeout     │
│  Error containment: recover()              │
└─────────────────┬──────────────────────────┘
                  │ TOOL_EXECUTION
                  ▼
┌────────────────────────────────────────────┐
│            PIPELINE ENGINE LAYER           │
│  STRICT: 7 stages | OPTIMIZED: 6 stages   │
│  StageResult per step                      │
└─────────────────┬──────────────────────────┘
                  │
                  ▼
┌────────────────────────────────────────────┐
│              STATE STORE LAYER             │
│  Append-only file-based event store        │
│  Atomic writes: temp → rename              │
│  Path: .ai/state/                          │
└─────────────────┬──────────────────────────┘
                  │ STATE_COMMIT
                  ▼
┌────────────────────────────────────────────┐
│           OBSERVABILITY LAYER              │
│  Structured JSON log emission              │
│  trace_id, worker_id, latency breakdown    │
└─────────────────┬──────────────────────────┘
                  │ RESPONSE_EMIT
                  ▼
┌────────────────────────────────────────────┐
│              Response Writer               │
│          (stdout / channel)                │
└────────────────────────────────────────────┘
```

---

## 2. Event Model

### 2.1 Event-Driven Architecture

The kernel is **internally event-driven**. Every state transition is represented as a typed event. Components communicate through channels and produce structured event records. The pipeline is a *consumer* of events, not the architectural backbone.

### 2.2 Event Types

| Event Type | Producer | Consumer | Trigger |
|---|---|---|---|
| `REQUEST_INGRESS` | stdin reader | Gateway | New JSON line on stdin |
| `QUEUE_ENQUEUE` | Gateway | RequestQueue | ctx pushed to channel |
| `QUEUE_DEQUEUE` | RequestQueue | Worker | worker reads from channel |
| `QUEUE_REJECT` | RequestQueue | Gateway | channel full → backpressure |
| `POLICY_EVALUATION` | PolicyEngine | Pipeline | policy check per stage |
| `POLICY_DENIAL` | PolicyEngine | Worker | DENY verdict → stop |
| `TOOL_EXECUTION` | ExecutionCore | Pipeline | tool handler invoked |
| `TOOL_TIMEOUT` | ExecutionCore | Worker | context deadline exceeded |
| `TOOL_ERROR` | ExecutionCore | Worker | tool panic → recovered |
| `STATE_COMMIT` | StateStore | (disk) | trace/session persisted |
| `RESPONSE_EMIT` | ResultCollector | stdout | final response written |

### 2.3 Event Contract

Every event MUST conform to this struct:

```go
type KernelEvent struct {
    EventID     string          `json:"event_id"`     // UUIDv7
    Type        EventType       `json:"type"`          // typed enum
    Timestamp   time.Time       `json:"timestamp"`     // UTC RFC3339
    RequestID   string          `json:"request_id"`    // correlation ID
    WorkerID    string          `json:"worker_id"`     // "wrk_000".."wrk_NNN"
    Payload     json.RawMessage `json:"payload"`       // type-specific
    TraceID     string          `json:"trace_id"`      // = RequestID for now
}
```

### 2.4 Event Lifecycle (Single Request)

```
REQUEST_INGRESS
  → QUEUE_ENQUEUE
    → QUEUE_DEQUEUE
      → POLICY_EVALUATION (×N policies)
        → [POLICY_DENIAL | continue]
          → TOOL_EXECUTION
            → [TOOL_TIMEOUT | TOOL_ERROR | success]
              → STATE_COMMIT
                → RESPONSE_EMIT
```

---

## 3. Component Specifications

### 3.1 MCP Gateway (Ingress Layer)

```go
type Gateway struct {
    queue   chan<- RequestContext    // send-only channel
    metrics *GatewayMetrics
}

func (g *Gateway) Ingest(ctx context.Context, raw json.RawMessage) error
func (g *Gateway) validate(raw json.RawMessage) (*RequestContext, error)
```

| Responsibility | Implementation |
|---|---|
| Accept external tool execution requests | `Ingest()` — called from stdin reader goroutine |
| Validate request schema | `validate()` — JSON parse + mandatory field check |
| Attach RequestContext | Factory: `NewRequestContext(raw)` |
| Forward to RequestQueue | `g.queue <- ctx` — non-blocking send with backpressure handling |

**Constraint:** `Ingest()` MUST NOT execute tools directly. MUST NOT bypass the queue.

**Go mapping:** `g.queue` is a buffered `chan RequestContext`. When full, return `QUEUE_FULL` error immediately (fail-closed).

### 3.2 Request Queue Layer

```go
type RequestQueue struct {
    ch      chan RequestContext  // buffered channel
    maxSize int
    metrics *QueueMetrics
}

func NewRequestQueue(maxSize int) *RequestQueue
func (q *RequestQueue) Enqueue(ctx context.Context, req RequestContext) error
func (q *RequestQueue) Chan() <-chan RequestContext  // consumers read this
```

| Behavior | Specification |
|---|---|
| **Type** | Bounded FIFO — `make(chan RequestContext, maxSize)` |
| **Backpressure** | Channel full → return `ErrQueueFull` immediately. NEVER block indefinitely. |
| **Ordering** | FIFO at channel level. Workers consume in channel order. Response order is NOT guaranteed. |
| **States** | `ENQUEUED`, `DEQUEUED`, `REJECTED_QUEUE_FULL` |

**Go mapping:** The channel IS the queue. No wrapper list needed. `select` with `default` for non-blocking enqueue.

```go
func (q *RequestQueue) Enqueue(ctx context.Context, req RequestContext) error {
    select {
    case q.ch <- req:
        q.metrics.Enqueued.Inc()
        return nil
    default:
        q.metrics.Rejected.Inc()
        return ErrQueueFull{MaxSize: q.maxSize}
    }
}
```

### 3.3 Worker Pool

```go
type WorkerPool struct {
    workers   []*Worker
    queue     <-chan RequestContext      // receive-only
    results   chan<- ProcessedContext     // send-only
    wg        sync.WaitGroup
}

type Worker struct {
    ID        string                     // "wrk_000"
    queue     <-chan RequestContext
    results   chan<- ProcessedContext
    policy    *PolicyEngine
    executor  *ExecutionCore
    pipeline  *PipelineEngine
    logger    *StructuredLogger
}
```

| Behavior | Specification |
|---|---|
| **Model** | N persistent goroutines. NOT per-request goroutines. |
| **Lifecycle** | `for ctx := range worker.queue { process(ctx) }` — range exits when channel closes. |
| **Shutdown** | Close `queue` channel. Workers exit `range` loop. `wg.Wait()`. |
| **Isolation** | Workers share NO mutable state. Each worker has its own `RequestContext` copy. |
| **Fatal Protection** | `recover()` in worker loop catches panics. Panicking worker logs error, continues. |

**Go worker loop:**

```go
func (w *Worker) Run(ctx context.Context) {
    defer w.wg.Done()
    for reqCtx := range w.queue {
        w.processOne(ctx, reqCtx)
    }
}

func (w *Worker) processOne(ctx context.Context, reqCtx RequestContext) {
    defer func() {
        if r := recover(); r != nil {
            w.logger.Log(KernelEvent{Type: "WORKER_PANIC", ...})
        }
    }()
    reqCtx.WorkerID = w.ID
    result := w.pipeline.Process(ctx, reqCtx)
    w.results <- result
}
```

### 3.4 Policy Engine (Gatekeeper)

```go
type PolicyEngine struct {
    rules    []PolicyRule          // ordered by priority
    version  string
}

type PolicyRule struct {
    ID       string
    Name     string
    Stage    string                // which pipeline stage
    Priority int
    Evaluate func(ctx *RequestContext, toolDef *ToolDef) PolicyVerdict
}

type PolicyVerdict struct {
    Decision    string              // "ALLOW" | "DENY"
    Reason      string
    RuleID      string
    RuleChain   []PolicyDecision    // all rules evaluated
    Timestamp   time.Time
}

type PolicyDecision struct {
    RuleID    string `json:"rule_id"`
    Decision  string `json:"decision"`
    Reason    string `json:"reason"`
    Stage     string `json:"stage"`
    Priority  int    `json:"priority"`
}
```

| Behavior | Specification |
|---|---|
| **Evaluation** | Rules sorted by priority. Evaluated sequentially. First DENY stops chain. |
| **Fail-Closed** | Default verdict for missing rules = DENY. |
| **Decision Graph** | All decisions (ALLOW + DENY) appended to `RequestContext.PolicyGraph`. |
| **Composability** | Future: rules can reference output of prior rules via `context.Carry`. |

**Go evaluation:**

```go
func (pe *PolicyEngine) EvaluateStage(stage string, ctx *RequestContext, toolDef *ToolDef) PolicyVerdict {
    var chain []PolicyDecision
    for _, rule := range pe.rules {
        if rule.Stage != stage {
            continue
        }
        verdict := rule.Evaluate(ctx, toolDef)
        chain = append(chain, PolicyDecision{
            RuleID: rule.ID, Decision: verdict.Decision,
            Reason: verdict.Reason, Stage: stage, Priority: rule.Priority,
        })
        ctx.PolicyGraph = append(ctx.PolicyGraph, chain[len(chain)-1])
        if verdict.Decision == "DENY" {
            return PolicyVerdict{Decision: "DENY", RuleChain: chain, ...}
        }
    }
    return PolicyVerdict{Decision: "ALLOW", RuleChain: chain, RuleID: "default-allow"}
}
```

### 3.5 Execution Core

```go
type ExecutionCore struct {
    registry    *ToolRegistry
    defaultTimeout time.Duration  // 30s
}

type ExecResult struct {
    Status      string            // "success" | "error"
    Result      json.RawMessage
    Error       string
    ErrorCode   string            // "EXECUTION_TIMEOUT" | "EXECUTION_ERROR"
    DurationMs  float64
}
```

| Behavior | Specification |
|---|---|
| **Isolation** | Each tool runs with `context.WithTimeout(ctx, timeout)`. Deadline exceeded → `EXECUTION_TIMEOUT`. |
| **Panic recovery** | `defer recover()` in the tool dispatch. Panic → `EXECUTION_ERROR`. |
| **Worker safety** | Tool execution failure MUST NOT crash the worker goroutine. |
| **Timeout** | Default 30s. Per-tool override via `toolDef.Governance.TimeoutMs`. |

**Go execution with context:**

```go
func (ec *ExecutionCore) Execute(ctx context.Context, toolID string, args map[string]interface{}) ExecResult {
    toolDef := ec.registry.Get(toolID)
    timeout := ec.resolveTimeout(toolDef)

    execCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    resultCh := make(chan ExecResult, 1)
    go func() {
        defer func() {
            if r := recover(); r != nil {
                resultCh <- ExecResult{Status: "error", ErrorCode: "EXECUTION_ERROR", Error: fmt.Sprint(r)}
            }
        }()
        resultCh <- ec.dispatch(execCtx, toolDef, args)
    }()

    select {
    case result := <-resultCh:
        return result
    case <-execCtx.Done():
        return ExecResult{Status: "error", ErrorCode: "EXECUTION_TIMEOUT", Error: "tool timeout"}
    }
}
```

### 3.6 Pipeline Engine

```go
type PipelineEngine struct {
    stages       map[string]StageHandler
    strictOrder  []string   // ["pre_validation", "session_guard", "capability_routing", ...]
    optimizedOrder []string // same minus "audit_log"
}

type StageHandler func(ctx context.Context, req *RequestContext, svc *PipelineServices) StageResult
```

| Mode | Stages | Use |
|---|---|---|
| `STRICT` | 7 stages (pre_validation → ... → audit_log) | Full governance, full audit |
| `OPTIMIZED` | 6 stages (pre_validation → ... → post_validation) | Performance-sensitive; audit_log runs at pipeline completion |

**Go process:**

```go
func (pe *PipelineEngine) Process(ctx context.Context, req *RequestContext, svc *PipelineServices) *RequestContext {
    order := pe.strictOrder
    if req.PipelineMode == "optimized" {
        order = pe.optimizedOrder
    }
    for _, stage := range order {
        handler := pe.stages[stage]
        result := handler(ctx, req, svc)
        req.AppendTrace(result)
        if result.Decision == "DENY" {
            req.Finalize("denied", ...)
            pe.runAuditLog(ctx, req, svc)   // always audit on deny
            return req
        }
    }
    req.Finalize("success", ...)
    pe.runAuditLog(ctx, req, svc)            // always audit on success
    return req
}
```

### 3.7 State Store

```go
type StateStore struct {
    root string   // .ai/state/
    mu   sync.Mutex
}

type TraceRecord struct {
    RequestID        string              `json:"request_id"`
    Status           string              `json:"status"`
    ExecutionTrace   []StageResult       `json:"execution_trace"`
    DecisionPath     []string            `json:"decision_path"`
    StageTimings     map[string]float64  `json:"stage_timings"`
    WorkerID         string              `json:"worker_id"`
    PipelineMode     string              `json:"pipeline_mode"`
    PolicyGraph      []PolicyDecision    `json:"policy_graph"`
    LatencyBreakdown LatencyBreakdown    `json:"latency_breakdown"`
    Timestamp        time.Time           `json:"timestamp"`
}
```

| Behavior | Specification |
|---|---|
| **Type** | Append-only file-based event store |
| **Path** | `.ai/state/traces/<request_id>.json`, `.ai/state/sessions/<session_id>.json`, `.ai/state/meta.json` |
| **Writes** | Atomic: write to `<name>.tmp` → `os.Rename(tmp, name)` |
| **Constraint** | NO in-place mutation of historical records. Append-only semantics. |
| **Recovery** | Full execution trace reconstructable from state store alone. |

### 3.8 Observability Layer

```go
type StructuredLogger struct {
    w    io.Writer           // .ai/governance/audit/gateway.log
    mu   sync.Mutex
}

type LogEntry struct {
    RequestID            string              `json:"request_id"`
    SessionID            string              `json:"session_id"`
    ProjectID            string              `json:"project_id"`
    ToolID               string              `json:"tool_id"`
    Status               string              `json:"status"`
    DecisionPath         []string            `json:"decision_path"`
    StageTimings         map[string]float64  `json:"stage_timings"`
    TotalMs              float64             `json:"total_ms"`
    QueueWaitTimeMs      float64             `json:"queue_wait_time_ms"`
    WorkerID             string              `json:"worker_id"`
    PipelineMode         string              `json:"pipeline_mode"`
    LatencyBreakdown     LatencyBreakdown    `json:"latency_breakdown"`
    PolicyGraph          []PolicyDecision    `json:"policy_decision_graph"`
    ExecutionTrace       []StageResult       `json:"execution_trace"`
    Error                string              `json:"error"`
    ErrorCode            string              `json:"error_code"`
    Timestamp            time.Time           `json:"timestamp"`
}
```

| Required Telemetry | Field |
|---|---|
| request_id | `LogEntry.RequestID` |
| worker_id | `LogEntry.WorkerID` |
| queue_wait_time_ms | `LogEntry.QueueWaitTimeMs` |
| execution_latency_breakdown | `LogEntry.LatencyBreakdown` |
| policy_decision_graph | `LogEntry.PolicyGraph` |
| execution_trace | `LogEntry.ExecutionTrace` |

---

## 4. RequestContext Specification

```go
type RequestContext struct {
    // Mandatory fields (validated at ingress)
    RequestID       string              `json:"request_id"`
    SessionID       string              `json:"session_id"`
    ProjectID       string              `json:"project_id"`

    // Populated by pipeline stages
    ToolID          string              `json:"tool_id"`
    Capability      string              `json:"capability"`
    Arguments       map[string]interface{} `json:"arguments"`

    // Execution tracking
    Status          string              `json:"status"`           // "pending" | "success" | "error" | "denied"
    ExecutionTrace  []StageResult       `json:"execution_trace"`
    DecisionPath    []string            `json:"decision_path"`
    StageTimings    map[string]float64  `json:"stage_timings"`

    // Worker metadata
    WorkerID        string              `json:"worker_id"`
    QueueWaitTimeMs float64             `json:"queue_wait_time_ms"`
    PipelineMode    string              `json:"pipeline_mode"`    // "strict" | "optimized"

    // Observability
    LatencyBreakdown LatencyBreakdown   `json:"latency_breakdown"`
    PolicyGraph      []PolicyDecision   `json:"policy_graph"`

    // Lifecycle
    TimestampStart  time.Time           `json:"timestamp_start"`
    TimestampEnd    time.Time           `json:"timestamp_end"`
    Result          json.RawMessage     `json:"result"`
    Error           string              `json:"error"`
    ErrorCode       string              `json:"error_code"`

    // Go-specific: context cancellation
    Ctx             context.Context     `json:"-"`              // not serialized
}

type LatencyBreakdown struct {
    TotalMs        float64 `json:"total_ms"`
    QueueWaitMs    float64 `json:"queue_wait_ms"`
    RoutingMs      float64 `json:"routing_ms"`
    ExecutionMs    float64 `json:"execution_ms"`
    AuditMs        float64 `json:"audit_ms"`
    ValidationMs   float64 `json:"validation_ms"`
}

type StageResult struct {
    Stage       string              `json:"stage"`
    Decision    string              `json:"decision"`           // "allow" | "deny" | "skip"
    DurationMs  float64             `json:"duration_ms"`
    Detail      map[string]interface{} `json:"detail"`
    Error       string              `json:"error"`
    Timestamp   time.Time           `json:"timestamp"`
}
```

---

## 5. Determinism Rules

| Rule | Specification | Enforcement |
|---|---|---|
| **5.1 Policy determinism** | `same_context + same_toolDef → same_verdict` | Policy rules are pure functions — no I/O, no time, no random |
| **5.2 Execution isolation** | Workers share NO mutable memory. Communication via channels only. | Go race detector (`-race`) must show zero violations |
| **5.3 Queue ordering** | FIFO ordering guaranteed at channel level. Completion ordering NOT guaranteed. | Channel semantics enforce FIFO |
| **5.4 Idempotent state writes** | Writing same trace twice produces identical file. | Atomic rename — last write wins |
| **5.5 Deterministic worker IDs** | `fmt.Sprintf("wrk_%03d", index)` — stable per process lifetime | Assigned at pool construction |

---

## 6. Failure Model (Fail-Closed)

| Failure Condition | System Response | Audit Trail |
|---|---|---|
| **Invalid session** (missing session_id/project_id) | DENY at `pre_validation` stage | `POL-001` denial in policy_graph |
| **Missing tool** (not in registry) | DENY at `capability_routing` stage | `POL-002` denial in policy_graph |
| **Policy denial** (e.g. path access) | DENY at `pre_execution` stage | Policy ID + reason in policy_graph |
| **Queue overflow** (channel full) | `QUEUE_FULL` error returned to caller | Rejected count in queue metrics |
| **Execution timeout** (context deadline) | `EXECUTION_TIMEOUT` error envelope | Full trace up to timeout point |
| **Tool panic** (unrecovered error) | `recover()` → `EXECUTION_ERROR` envelope | Stack trace in error_trace field |
| **Worker panic** (unrecovered in loop) | `recover()` in worker loop. Worker continues. | `WORKER_PANIC` event logged |

**Critical principle:** NO partial execution. Every failure produces a complete audit entry. System degrades via REJECT, never via crash.

---

## 7. Performance Model

| Metric | Target | Configuration |
|---|---|---|
| **Max concurrent requests** | N (worker count) | `AI_GATEWAY_WORKERS` env var (default: 4) |
| **Queue depth** | 128 | `AI_QUEUE_SIZE` env var |
| **Tool execution timeout** | 30s default, per-tool override | `toolDef.Governance.TimeoutMs` |
| **Queue wait timeout** | 5s for enqueue | Hard-coded, fail-closed on timeout |
| **Worker drain on shutdown** | 300ms wait, then force close | Non-configurable |
| **Degradation strategy** | REJECT (never crash) | Queue full → `QUEUE_FULL`; timeout → `EXECUTION_TIMEOUT` |

---

## 8. State Consistency Model

| Guarantee | Implementation |
|---|---|
| **Append-only event sourcing** | Traces written once per request_id. No overwrite. |
| **Atomic writes** | Write to `<name>.tmp` → `os.Rename(tmp, name)`. POSIX guarantees atomic rename. |
| **No in-place mutation** | Historical records (`traces/*.json`) are never modified after commit. |
| **Recovery** | All state reconstructable from `.ai/state/` directory. `meta.json` provides version reference. |
| **Event log as source of truth** | The state store IS the event log. No separate WAL/journal. |

---

## 9. Implementation Mapping: Python → Go

| Python (v0.4.0) | Go Equivalent | Primitive |
|---|---|---|
| `threading.Thread` | `goroutine` | Lightweight thread |
| `threading.Lock` / `threading.Condition` | `sync.Mutex` / `sync.Cond` or channel | Lock |
| `RequestQueue` (list + lock) | `chan RequestContext` (buffered) | Channel |
| `WorkerPool` (thread management) | `sync.WaitGroup` + goroutine `for range` | WaitGroup |
| `ThreadPoolExecutor` (per-request) | `go func()` with channel result | Goroutine |
| `time.sleep()` shutdown drain | `time.After()` + `select` | Timer |
| `threading.Event` (shutdown signal) | `context.Context` cancellation | Context |
| `StateStore._atomic_write()` | `os.Rename()` | POSIX atomic rename |
| `json.dumps` / `json.loads` | `encoding/json` | Standard library |

---

## 10. Configuration Surface

```go
type KernelConfig struct {
    WorkspaceRoot string        // AI_WORKSTATION_ROOT or cwd
    WorkerCount   int           // AI_GATEWAY_WORKERS (default 4)
    QueueSize     int           // AI_QUEUE_SIZE (default 128)
    LogPath       string        // .ai/governance/audit/gateway.log
    StatePath     string        // .ai/state/
    RegistryPath  string        // runtime/tools/definitions.yaml
    PolicyPath    string        // runtime/governance/policies/runtime.yaml
}
```

---

## 11. Non-Goals (Explicit Exclusions)

| Exclusion | Justification |
|---|---|
| Distributed execution systems | Single-process only. No RPC, no mesh. |
| Kubernetes orchestration | No container awareness. No pod scheduling. |
| Multi-agent frameworks (LangGraph, CrewAI) | Explicitly out of scope per v0.3.0/v0.4.0 constraints. |
| External database dependencies | File-based state store only. No PostgreSQL, no Redis. |
| AI planning layers | No LLM integration in kernel. |
| Unix socket listener | stdio only. Deferred to future release. |
| Dynamic pipeline reconfiguration | Pipeline stages fixed at compile time. |
| gRPC / protobuf transport | JSON-RPC over stdio only. |

---

## 12. Success Criteria (Go Implementation)

A Go implementation is **VALID** only if ALL of these pass:

- [x] Requests flow through `channel → goroutine pool → policy engine → execution core`
- [x] Policy engine blocks execution before runtime (DENY stops pipeline)
- [x] Execution is isolated: tool timeout/panic does NOT crash any goroutine
- [x] Full execution trace is reconstructable from `.ai/state/traces/*.json`
- [x] System supports both STRICT and OPTIMIZED pipeline modes
- [x] `AI_GATEWAY_WORKERS` controls goroutine count
- [x] `AI_QUEUE_SIZE` controls channel buffer size
- [x] Queue full → `QUEUE_FULL` error (fail-closed, no silent drop)
- [x] Go race detector (`go test -race`) reports zero data races
- [x] `context.WithTimeout` enforces per-tool execution boundaries

---

## 13. Architecture Decision Reference

| ADR | Title | Spec Section |
|---|---|---|
| ADR-001 | AI Workstation Baseline Architecture | §1.3 System Boundaries |
| ADR-002 | Unix Socket Gateway over TCP | §3.1 (deferred, stdio used) |
| ADR-003 | Per-Project ChromaDB Namespaces | (out of kernel scope) |
| ADR-004 | Session-Scoped Tool Injection | §3.6 Pipeline Engine |
| ADR-005 | MCP Gateway Runtime Implementation | §3.1-3.3 |
| ADR-006 | Deterministic Control Plane Kernel | §3.4-3.6 |
| ADR-007 | Adaptive Runtime Kernel | §2, §3.2-3.3, §3.7 |

---

*This specification is the authoritative reference for any Go implementation of the MCP Control Plane Kernel. The Python runtime at `runtime/` serves as the behavior-verified reference implementation. All deviations from this spec require an ADR.*
