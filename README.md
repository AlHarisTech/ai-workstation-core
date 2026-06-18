# AI Workstation Core

**Release:** v1.0.0-alpha
**Baseline:** L4-PREDICTIVE-SEALED
**Class:** Deterministic Predictive Control Platform

---

## Status

```
✓ Architecturally Sealed
✓ Reference Implementation Complete
✓ Archive Grade Ready
✓ 120 Tests Passing
✓ 0 Races Detected
⚠ API Evolvable — Compatibility Not Yet Frozen
```

## Architecture

```
Applications
      ▲
ADR-0009 (L2-L4 Adaptive Control)
      ▲
MEK v1.4 (L1 Immutable Verification Substrate)
```

### MEK v1.4 — Deterministic Verification Substrate

- Deterministic execution engine (Wave Scheduler, Commit Engine, Dispatcher)
- Structural verification (CEG constraint system)
- Replay equivalence oracle
- Cross-domain consistency lattice (5 formal theorems)
- 18 runtime invariants, 6 system guarantees

### ADR-0009 — Adaptive Control Plane

- Signal ingestion from MEK verification outputs
- Feedback controller (6 built-in rules: R01–R06)
- Action bus (at-least-once delivery, TTL, idempotency)
- Drift predictor (trend-based, deterministic)

## Quick Start

```bash
# Build
cd mek && go build ./...

# Run all tests
go test ./... -count=1 -timeout 120s

# Race detection
go test -race ./... -count=1 -timeout 120s

# Verify MEK against a RIR fixture
go run ./cmd/mek/ -rir test/fixtures/simple_dag.json
```

## Formal Properties

| Category | Count | IDs |
|----------|-------|-----|
| Runtime Invariants | 18 | M-001…M-018 |
| System Guarantees | 6 | G1…G6 |
| Control Invariants | 8 | AC-001…AC-008 |
| Consistency Theorems | 5 | T1…T5 |
| **Total** | **37** | |

## Specifications

```
spec/
├── system-spec-v1.0.md      ← Canonical system specification (5 books, 14 ADRs)
├── mek-v1.0.md               ← MEK executable specification (18 invariants)
├── architecture/
│   ├── adr/adr-0009.md       ← Adaptive control specification
│   ├── adr/adr-0009-e.md     ← Implementation notes & evidence
│   └── theorem/consistency-lattice.md ← Formal proofs (T1–T5)
└── freeze/L4-PREDICTIVE-SEALED/
    ├── BASELINE.md            ← System class, layers, invariants
    ├── TEST-MANIFEST.md       ← 120 tests, invariant coverage map
    ├── CONTRACTS.md           ← Public types, endpoint contract
    └── BUILD-MANIFEST.md      ← Build commands, package map, checksum
```

## Baseline Checksum

```
e424f6cc8ab2abadd2e92e9304e9d1b43c205a29ba657a1f3a52627b51fe637e
```

## License

MIT
