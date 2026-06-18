# L4-PREDICTIVE-SEALED — Build Manifest

---

## Build Environment

| Component | Version |
|-----------|---------|
| Go | 1.21+ |
| OS | Linux (amd64) |
| Module | `github.com/anomalyco/mek` |

---

## Build Commands

```bash
cd mek

# Build all packages
go build ./...

# Run all tests
go test ./... -count=1 -timeout 120s

# Race detection
go test -race ./... -count=1 -timeout 120s

# Build CLI binary
go build -o bin/mek ./cmd/mek/
```

---

## Module Dependencies

```
github.com/anomalyco/mek
  (zero external dependencies — standard library only)
```

---

## Package Map

```
mek/
├── cmd/mek/                    ← CLI entry point
├── pkg/types/                  ← Shared types (RIR, CEG, StatusMap)
├── internal/
│   ├── rir/                    ← RIR loader + validator
│   ├── ceg/                    ← CEG builder + activation
│   ├── scheduler/              ← Wave partition + execution loop
│   ├── commit/                 ← Single-writer state machine
│   ├── dispatcher/             ← Adapter dispatch + gate
│   ├── contextstore/           ← Ephemeral execution context
│   ├── runtime/                ← MEK orchestration
│   ├── journal/                ← Causal truth ledger (passive)
│   ├── trace/                  ← Temporal truth collector (passive)
│   ├── replay/                 ← Determinism oracle
│   ├── verify/                 ← Structural proof + consistency
│   └── adaptctl/               ← ADR-0009 control plane
│       ├── signal/             ← Canonical signal schema
│       ├── feedback/           ← Deterministic rule engine
│       ├── actionbus/          ← At-least-once delivery
│       └── predict/            ← Trend-based drift forecast
└── test/
    ├── fixtures/               ← RIR test fixtures (JSON)
    ├── integration/            ← End-to-end MEK tests
    └── invariants/             ← M/G invariants + stress + path invariance
```

---

## Test Count by Package

```
internal/adaptctl/actionbus    14 tests
internal/adaptctl/feedback     15 tests
internal/adaptctl/predict      15 tests
internal/adaptctl/signal       18 tests
internal/journal                5 tests
internal/replay                 4 tests
internal/trace                  4 tests
internal/verify                10 tests
test/integration                4 tests
test/invariants                31 tests
────────────────────────────────────
TOTAL                         120 tests
```

---

## Line Count (Approximate)

```
Go source:      ~5500 LOC
Specifications: ~3500 lines (Markdown)
Test fixtures:  ~200 lines (JSON)
```

---

## Reproducible Build

```bash
cd mek
go build -trimpath -ldflags="-s -w" -o bin/mek ./cmd/mek/
sha256sum bin/mek
```

---

## Freeze Checksum

The archival fingerprint of the reference implementation source:

```
e424f6cc8ab2abadd2e92e9304e9d1b43c205a29ba657a1f3a52627b51fe637e
```

**Reproduction procedure (portable):**

```bash
cd mek
git ls-files '*.go' | sort | xargs cat | sha256sum
```

If git is unavailable, a locale-independent fallback:

```bash
LC_ALL=C find . -name '*.go' -type f -print0 | sort -z | xargs -0 cat | sha256sum
```

This documents the exact source state at freeze time. It is NOT a compatibility guarantee — it is an archival fingerprint.
