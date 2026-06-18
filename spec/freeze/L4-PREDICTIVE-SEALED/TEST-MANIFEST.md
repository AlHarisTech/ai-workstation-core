# L4-PREDICTIVE-SEALED — Test Manifest

---

## Full Test Suite

```bash
cd mek
go test ./... -count=1 -timeout 120s
```

### Expected Output (PASS)

```
ok  internal/adaptctl/actionbus
ok  internal/adaptctl/feedback
ok  internal/adaptctl/predict
ok  internal/adaptctl/signal
ok  internal/journal
ok  internal/replay
ok  internal/trace
ok  internal/verify
ok  test/integration
ok  test/invariants
```

---

## Race Detection

```bash
cd mek
go test -race ./... -count=1 -timeout 120s
```

Expected: `NO_RACES_DETECTED` under executed test paths.

---

## Determinism Verification

```bash
cd mek
go test ./test/invariants/ -run TestM005 -count=1      # 100 iterations
go test ./test/invariants/ -run TestG1 -count=1         # 500 iterations
go test ./test/invariants/ -run TestPathInvariance -count=1
go test ./internal/adaptctl/feedback/ -run TestDeterminism -count=1   # 500 iterations
go test ./internal/adaptctl/predict/ -run TestDeterminism -count=1    # 500 iterations
```

Expected: all PASS, zero divergence across iterations.

---

## Invariant Coverage

### M-Invariants (MEK Runtime)

| Invariant | Test |
|-----------|------|
| M-001: Commit Engine sole writer | `TestM001_CommitEngineSoleWriter` |
| M-002: CEG immutability | `TestM002_CEGImmutability` |
| M-003: No execution outside READY | `TestM003_NoExecutionOutsideReady` |
| M-004: No scheduler bypass | `TestM004_NoSchedulerBypass` |
| M-005: Deterministic transitions | `TestM005_DeterministicTransitions` |
| M-006: Deterministic waves | `TestM006_DeterministicWaves` |
| M-007: No cross-wave overlap | `TestM007_NoCrossWaveOverlap` |
| M-008: Node-id-ordered recompute | `TestM008_DeterministicRecompute` |
| M-009: Isolated context | `TestM009_IsolatedContext` |
| M-010: No shared state | `TestM010_NoSharedState` |
| M-011: Dispatcher no CEG mutation | `TestM011_DispatcherNoMutation` |
| M-012: Failure is terminal | `TestM012_FailureIsTerminal` |
| M-013: Termination closure | `TestM013_TerminationClosure` |
| M-014: No partial corruption | `TestM014_NoPartialCorruption` |
| M-015..M-017: No external layers | `TestM015_M017_NoExternalLayers` |
| M-018: No compiler in MEK | `TestM018_NoCompiler` |

### G-Guarantees

| Guarantee | Test |
|-----------|------|
| G1: Deterministic | `TestG1_Determinism` (500 iterations) |
| G2: Isolated | `TestG2_Isolation` |
| G3: Consistent | `TestG3_Consistency` |
| G4: Bounded | `TestG4_BoundedTermination` |
| G5: Pure | `TestG5_Pure` |
| G6: Verifiable | `TestG6_Verifiable` |

### AC-Invariants (ADR-0009)

| Invariant | Verified By |
|-----------|-------------|
| AC-001: Never modifies MEK | Zero imports + read-only contracts |
| AC-002: No business decisions | Actions are data structures |
| AC-003: Same inputs → same actions | 500-run determinism tests |
| AC-004: Trend-based prediction | Rate analysis only, no ML |
| AC-005: At-least-once + TTL | `TestRetry_*`, `TestTTL_*` |
| AC-006: Application isolation | Per-Bus endpoint, no shared state |
| AC-007: MEK stateless across apps | Signal ingestion per-execution |
| AC-008: Signals immutable | Convention-level, hardening deferred |

---

## Stress Tests

```bash
go test ./test/invariants/ -run TestStress -v -count=1
```

| Test | Description | Expected |
|------|-------------|----------|
| `TestStress_Linear10` | 10-node chain | PASS |
| `TestStress_Linear100` | 100-node chain | PASS |
| `TestStress_Linear1000` | 1000-node chain (depth > 128) | REJECTED |
| `TestStress_Deep128` | Exactly 128 depth | PASS |
| `TestStress_Deep129` | Exceeds depth limit | REJECTED |
| `TestStress_Wide100` | 100 parallel nodes | PASS |
| `TestStress_Wide500` | 500 parallel nodes | PASS |
| `TestStress_InvalidRIR_Rejection` | Missing spec_hash | REJECTED |

---

## Path Invariance

```bash
go test ./test/invariants/ -run TestPathInvariance -v -count=1
```

All entry paths (RIR, Replay, Structural, Consistency) converge to identical execution.

---

## Fixture Catalog

```
test/fixtures/
├── simple_dag.json        ← 3 nodes, linear dependency
├── diamond_dag.json       ← 4 nodes, fork-join
├── escalate_dag.json      ← gate with ESCALATE branch
└── failure_propagation.json ← all_success + any_success propagation
```
