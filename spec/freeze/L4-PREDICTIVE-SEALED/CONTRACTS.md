# L4-PREDICTIVE-SEALED — Public Contracts

---

## MEK → ADR-0009 Contract

MEK exports the following read-only, by-value types:

### StatusMap (pkg/types)

```go
type StatusMap struct { /* private */ }

func (sm *StatusMap) Get(nodeID string) (NodeStatus, bool)
func (sm *StatusMap) GetState(nodeID string) *NodeState
func (sm *StatusMap) All() map[string]*NodeState
```

**Guarantee:** All methods are read-only. StatusMap is populated by Commit Engine during execution. No write path exists outside Commit Engine (M-001).

**Note:** `All()` returns references to internal `NodeState` objects for inspection only. Modifying returned state is undefined behavior — consumers MUST treat the returned map and its values as read-only.

### Report (internal/replay)

```go
type Report struct {
    Match       bool
    Divergences []Divergence
    /* ... */
}
```

**Guarantee:** Produced by `replay.Verify()`. Immutable after construction.

### Report (internal/verify)

```go
type Report struct {
    Pass       bool
    Violations []Violation
    Stats      Stats
}
```

**Guarantee:** Produced by `verify.Structural()`. Immutable after construction.

### ConsistencyReport (internal/verify)

```go
type ConsistencyReport struct {
    Pass   bool
    Checks []ConsistencyCheck
}
```

**Guarantee:** Produced by `verify.FullConsistencyCheck()`. Immutable after construction.

---

## ADR-0009 → Application Contract

### Signal (adaptctl/signal)

```go
type Signal struct {
    ID           string
    Timestamp    time.Time
    Source       Source
    Verification VerificationResult
    Divergences  []Divergence
    Metrics      Metrics
    Drift        *Drift
    State        State
}
```

**Guarantee:** Produced by `Ingestor.From*()`. Fields are readable. Immutability is convention-level (AC-008 — hardening deferred). A Signal represents a canonical snapshot of a single MEK verification run.

### Action (adaptctl/feedback)

```go
type Action struct {
    ID       string
    Type     ActionType   // Notify, Reexecute, Escalate, Halt
    SignalID string
    RuleID   string
    Target   string       // application endpoint
    Payload  map[string]interface{}
    IssuedAt time.Time
    TTL      time.Duration
}
```

**Guarantee:** Produced by `Engine.Evaluate()`. Action.ID is the idempotency key (AB-002). Actions are recommendations only — they carry no execution authority (AC-002).

### Prediction (adaptctl/predict)

```go
type Prediction struct {
    GeneratedAt time.Time
    Horizon     time.Duration
    Direction   Direction      // Stable, Improving, Degrading
    Confidence  float64
    State       signal.State
    Probability float64
    EarliestAt  *time.Time
    SampleSize  int
    Factors     []Factor
}
```

**Guarantee:** Produced by `Predict()`. Pure function of (history, horizon, now). Trend-based, no ML (AC-004).

---

## Application → ActionBus Contract

### Endpoint (adaptctl/actionbus)

```go
type Endpoint interface {
    Deliver(ctx context.Context, action feedback.Action) error
}
```

**Delivery Guarantees:**

| Property | Guarantee |
|----------|-----------|
| Delivery model | At-least-once |
| Ordering | Best-effort (sorted by IssuedAt within a Publish batch) |
| Idempotency | Action.ID is the idempotency key |
| TTL | Actions expire after TTL; zero TTL = never expires |
| Retry | Configurable MaxAttempts with RetryDelay |
| Concurrency | Sequential delivery within Publish; goroutine-safe across calls |

**Application Responsibilities:**

1. Treat `Action.ID` as idempotency key
2. Respond to `Deliver()` within context deadline
3. Accept that actions may be delivered more than once (at-least-once)
4. Do not depend on cross-Publish ordering
5. Do not assume actions reflect current MEK state (signals are snapshots)

---

## Compatibility Policy

| Dimension | Policy |
|-----------|--------|
| Invariants | LOCKED — cannot change without new ADR |
| Boundaries | LOCKED — no cross-layer imports |
| Public type fields | STABLE — additions allowed, removals require migration |
| Method signatures | STABLE — additions allowed, changes require version bump |
| Serialization format | NOT_YET_FROZEN — JSON schema may evolve |
| Import paths | NOT_YET_FROZEN — module path may change |
| Go version | NOT_YET_FROZEN — currently go 1.21 |
