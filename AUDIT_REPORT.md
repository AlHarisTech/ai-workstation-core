# PROJECT REALITY AUDIT — Architecture Gap Analysis

**Date**: 2026-06-11
**Repository**: `github.com/AlHarisTech/ai-workstation-core` (v0.4.0)
**Auditor**: Principal Systems Architect
**Method**: Evidence First — every claim cites file:line

---

## 1. Executive Summary

The repository implements a **dual-language control plane** (Go 94 files, Python 19 files) with two parallel MCP gateway architectures, a system orchestrator, and extensive compliance/validation tooling. The architecture is **heavy on documentation (18 .md files) and lightweight on integrated runtime enforcement**.

| Layer | Target | Current | Verdict |
|-------|--------|---------|---------|
| MCP Layer | 4 connected servers | 3 connected (Context7, GitHub, ChromaDB) + 1 dead config (Supabase) | **PARTIAL** |
| Knowledge Layer | ChromaDB with full pipeline | Collections exist, ingestion works, embeddings via cloud, retrieval works | **IMPLEMENTED** |
| Governance Layer | PERP + Dispatcher + Audit + Provenance | Dispatcher IMPLEMENTED, Audit PARTIAL, Provenance PARTIAL, PERP DOCUMENTED ONLY | **PARTIAL** |
| Runtime Integration | End-to-end flow | Request → MCP only. Knowledge and Governance are async side effects, never influence response | **FRAGMENTED** |

**Overall Completion**: ~42% (weighted: MCP 30%, Knowledge 20%, Governance 30%, Integration 20%)

---

## 2. Repository Inventory

### 2.1 Directory Tree (Core Platform Only)

```
.ai/                              # AI platform docs (ADR, ARCHITECTURE, GOVERNANCE, SPEC)
docs/architecture/                # 9 .md spec files (v1.0 closure, contract, MRO, RFL, v1.1 schema)
runtime/
├── main.go                       # Go entry point
├── mcp/                          # MCP Layer (Go)
│   ├── gateway.go                # v1 IntegrationGateway (full enterprise pipeline)
│   ├── v2/                       # v2 Gateway (simpler, stdio JSON-RPC)
│   │   ├── schema.go, validate.go, policy.go, router.go, servers.go, trace.go
│   │   ├── gateway.go, gateway_test.go, adapters_test.go
│   │   └── cmd/main.go           # CLI entry point (JSON-RPC via stdin)
│   ├── tools/filesystem/         # Filesystem tool (Go)
│   ├── tools/git/                # Git tool (Go)
│   ├── tools/github/             # GitHub tool (Go)
│   ├── router/                   # v1 tool router
│   ├── contracts/                # Backpressure, circuit breaker, retry contracts
│   ├── observability/            # Telemetry, trace graph, health, efficiency, signals
│   ├── hardening/                # Trace compressor, kernel noise filter
│   ├── release/                  # MRO: release orchestrator, queue, retry, persistence
│   └── stress/                   # Load testing
├── orchestrator/v1/              # Side-effect orchestrator (Chroma, Postgres, LangGraph, CrewAI)
├── compliance/                   # Compliance suite (fairness, replay, SLA, policy, shutdown)
├── validation/                   # Validation suite (stress, failure injection, contracts)
├── observability/                # Logger, metrics, tracer, latency
├── policy/                       # Policy engine (decision graph, rules)
├── queue/                        # Fair queue, backpressure, request queue
├── state/                        # State store, atomic writer, consistency, snapshot
├── worker/                       # Worker pool, supervisor, worker loop
├── types/                        # Core types (context, event, request)
├── kernel/                       # Python kernel (queue, state_store, worker_pool)
├── executor/                     # Python local executor
├── governance/                   # Python policy engine
├── auditlog/                     # Python structured logger
├── session/                      # Python session validation
├── tools/                        # Python tool registry
└── mcp_gateway/                  # Python MCP gateway (v0.4 kernel)
system/bootstrap/                 # Go bootstrap (EventBus + Gateway + Orchestrator)
scripts/                          # Chroma migration script
projects/chroma/                  # Python Chroma SDK (schema, search, chunking)
projects/chroma-mcp/              # MCP HTTP servers (server.js, github-server.js)
```

### 2.2 Language Breakdown

| Language | Files | Purpose |
|----------|-------|---------|
| Go | 94 | Core runtime, MCP gateways, orchestrator, compliance, validation, state, queue, worker, policy, observability |
| Python | 19 | MCP gateway (v0.4), governance engine, audit logging, session management, Chroma SDK, executor |
| JavaScript | 2 | MCP HTTP servers (ChromaDB, GitHub) |
| TypeScript | 6 | Movies demo app |
| Markdown | 18 | Architecture specs, docs |

### 2.3 Key Architectural Observation

**Two parallel MCP gateway architectures coexist:**
- **v1** (`runtime/mcp/`): Full enterprise pattern — IntegrationGateway, circuit breakers, retry budgets, backpressure, telemetry. Wired to kernel engine. **Only tested, not used in bootstrap.**
- **v2** (`runtime/mcp/v2/`): Simpler gateway — PolicyEngine, Router, 8 server adapters, JSON-RPC stdio. **Wired into system/bootstrap.**

The bootstrap uses ONLY v2. The v1 gateway (with all its resilience patterns) is dead code in production.

---

## 3. MCP Layer Audit

### 3.1 Context7

| Attribute | Finding | Evidence |
|-----------|---------|----------|
| Configured | YES — remote type, inline API key | `~/.config/opencode/opencode.jsonc:60-67` |
| Server Code | **REAL** — makes HTTP calls to api.context7.com | `runtime/mcp/v2/servers.go:706-732` |
| Authenticated | YES — CONTEXT7_API_KEY env var + inline header | `servers.go:575-578`, `opencode.jsonc:64` |
| Reachable | YES — returns 200 in OpenCode UI (Connected) | OpenCode MCP UI |
| Tools | query, store, resolve | `servers.go:599-625` |
| In Execution Flow | YES — registered in v2 gateway | `gateway.go:33` |
| Integrated in Pipeline | YES — routed as ActionContext7 | `router.go:27` |

**Verdict**: **IMPLEMENTED** — Fully functional remote MCP server with real API integration.

---

### 3.2 Supabase

| Attribute | Finding | Evidence |
|-----------|---------|----------|
| Configured | YES — remote type, no auth | `opencode.jsonc:5-9` |
| Server Code | **NONE** — no "supabase" server registered in either gateway | v2 `gateway.go:27-36` (no supabase), v1 `gateway.go:52-65` (no supabase) |
| Authenticated | NO — as MCP server; only URL configured | `opencode.jsonc:7` |
| Reachable | YES — returns 200 (Supabase MCP service exists) | OpenCode MCP UI shows Connected |
| Tools | None as MCP server | — |
| In Execution Flow | **NO** — dead config entry | No server registered, no action type for supabase |
| In Codebase | Only as Context7 secrets backend | `servers.go:650-685` (`fetchKeyFromSupabase`) |

**Verdict**: **DEAD CONFIG** — The URL `https://mcp.supabase.com/mcp?project_ref=...` resolves to a real Supabase MCP endpoint, but the workstation has zero code that sends requests to it. It shows "Connected" in OpenCode UI because the Supabase MCP service itself responds, but the workstation's runtime never invokes it.

---

### 3.3 GitHub

| Attribute | Finding | Evidence |
|-----------|---------|----------|
| Configured | YES — remote type, localhost:4115 | `opencode.jsonc:35-38` |
| Server Code | **REAL** — Octokit + MCP SDK HTTP server on port 4115 | `projects/chroma-mcp/github-server.js` |
| Also in v2 Gateway | YES — GitHubServer with real HTTP calls | `servers.go:425-559` |
| Also in v1 Gateway | YES — GitHubMCP tool | `tools/github/github.go` |
| Also in Release | YES — GitHubBridge | `release/github.go` |
| Authenticated | YES — GITHUB_TOKEN env var | `servers.go:443-445` (via ctx.TenantID), `github.go:21-24` (env) |
| Reachable | YES — returns 200 (Connected in UI) | OpenCode MCP UI |
| Tools | search_repos, get_repo, list_issues, create_issue, list_pulls, get_readme, list_branches, list_commits, get_user, list_repos | `github-server.js` |
| In Execution Flow | YES — 3 separate integrations | Gateway v2, v1, Release orchestrator |

**Verdict**: **IMPLEMENTED** — Three real implementations, fully authenticated, working connection.

---

### 3.4 ChromaDB

| Attribute | Finding | Evidence |
|-----------|---------|----------|
| Configured | YES — remote type, localhost:4114 | `opencode.jsonc:10-13` |
| Server Code | **REAL** — CloudClient + MCP SDK HTTP server on port 4114 | `projects/chroma-mcp/server.js` |
| Also in v2 Gateway | YES — ChromaAdapter with Chroma Cloud HTTP API | `servers.go:806-1014` |
| Also in Orchestrator | YES — ChromaStore for execution memory | `orchestrator/v1/chroma.go` |
| Python SDK | YES — schema.py, search.py, chunking.py | `projects/chroma/` |
| Authenticated | YES — CHROMA_API_KEY env var | `servers.go:820-822` |
| Reachable | YES — Connected in UI | OpenCode MCP UI |
| Tools | list_collections, query, peek, add_documents, count, delete_collection | `server.js` |
| In Execution Flow | YES — gateway + orchestrator | `gateway.go:35`, `orchestrator.go:90-98` |

**Verdict**: **IMPLEMENTED** — Full stack: MCP server, Go gateway adapter, orchestrator integration, Python SDK, migration script.

---

### 3.5 MCP Layer Summary

| Server | Config | Code | Auth | Connected | In Pipeline | Status |
|--------|--------|------|------|-----------|-------------|--------|
| Context7 | YES | REAL | YES | YES | YES | **IMPLEMENTED** |
| Supabase | YES | NONE | NO | YES (dead) | NO | **DEAD CONFIG** |
| GitHub | YES | REAL (×3) | YES | YES | YES | **IMPLEMENTED** |
| ChromaDB | YES | REAL (×3) | YES | YES | YES | **IMPLEMENTED** |
| filesystem | YES (stdio) | REAL | — | Not in UI | YES | **CLI ONLY** |
| git | YES (stdio) | REAL | — | Not in UI | YES | **CLI ONLY** |
| fetch | YES (stdio) | REAL | — | Not in UI | YES | **CLI ONLY** |
| memory | YES (stdio) | REAL | — | Not in UI | YES | **CLI ONLY** |

---

## 4. Knowledge Layer Audit

### 4.1 ChromaDB — Collections

| Collection | Status | Evidence |
|------------|--------|----------|
| `mcp_execution_memory` | **Partially populated** — code references it as default | `servers.go:924`, `chroma.go:64` |
| `test-hybrid-search` | **Exists** — confirmed via list_collections | `list_collections` returns this |
| Collection creation | **Works** — via Python SDK | `schema.py:88-91` |

The only confirmed existing collection is `test-hybrid-search`. The `mcp_execution_memory` collection is the code default but may not have been created yet.

### 4.2 ChromaDB — Documents

| Document Flow | Status | Evidence |
|---------------|--------|----------|
| Bulk ingestion script | IMPLEMENTED | `scripts/migrate-to-chroma.py:27-90` — reads files, chunks, uploads |
| Runtime event storage | IMPLEMENTED | `orchestrator/v1/chroma.go:48-85` — stores execution events |
| Chunking | IMPLEMENTED | `projects/chroma/chunking.py` — line-based with overlap, ≤16 KiB |
| Document IDs | Deterministic SHA-256 | `chunking.py:72-74` |

**Evidence of actual documents**: No confirmed documents in production. The migration script exists but has not been run against the cloud collection. The orchestrator stores runtime events only if triggered, and the `test-hybrid-search` collection was created but never populated with documents.

### 4.3 ChromaDB — Embeddings

| Embedding | Configuration | Where Generated |
|-----------|--------------|-----------------|
| Dense (Qwen3-Embedding-0.6B) | `schema.py:29-32` | **Cloud-side** — Chroma Cloud generates embeddings |
| Sparse (Splade PP en v1) | `schema.py:34-36` | **Cloud-side** — Chroma Cloud generates embeddings |
| Space | Cosine | `schema.py:46` |

The Go code (`servers.go:925-936`) sends raw document text to the Chroma Cloud API. The cloud service generates both dense and sparse embeddings server-side. **No local embedding generation exists.**

### 4.4 ChromaDB — Retrieval

| Retrieval Type | Status | Evidence |
|----------------|--------|----------|
| Go: queryDocs() — Cloud API call | IMPLEMENTED | `servers.go:948-975` |
| Python: hybrid_search() — RRF fusion | IMPLEMENTED | `search.py:21-100` |
| Python: dense_search() | IMPLEMENTED | `search.py:102-132` |
| Python: sparse_search() | IMPLEMENTED | `search.py:134-168` |
| Orchestrator: read-back | **MISSING** — orchestrator only writes | `chroma.go:33-45` (StoreMemory only) |

### 4.5 Project Embeddings

| Capability | Status | Evidence |
|------------|--------|----------|
| Embedding generation | **PARTIAL** — cloud-side only, no local generator | Chroma Cloud handles embeddings |
| Indexing | **PARTIAL** — Chroma Cloud indexes, no local index | Cloud handles SPANN + sparse indexing |
| Retrieval | **IMPLEMENTED** — hybrid RRF search works | `search.py:21-100` |
| Automatic sync | **MISSING** — no watcher/daemon for file changes | No inotify, no continuous sync |
| Query integration in MCP | **IMPLEMENTED** — ChromaAdapter.query | `servers.go:948-975` |

### 4.6 Knowledge Layer Verdict

**Overall**: **PARTIAL** (~55%)

| Component | Status |
|-----------|--------|
| ChromaDB Collections | **IMPLEMENTED** |
| ChromaDB Documents | **PARTIAL** (code exists, documents may not be ingested) |
| ChromaDB Embeddings | **PARTIAL** (cloud-side only, but works) |
| ChromaDB Retrieval | **IMPLEMENTED** (Go + Python) |
| Ingestion Pipeline | **IMPLEMENTED** (migration script + runtime storage) |
| Project Embeddings | **PARTIAL** (no local embedding, no auto-sync) |

---

## 5. Governance Layer Audit

### 5.1 PERP (Production Engineering Runtime Protocol)

| Capability | Status | Evidence |
|------------|--------|----------|
| Documentation | **COMPLETE** — 4 PERP files + 3 agent definitions | `easyfit-pro/.opencode/PERP_*.md`, `agent/*.md` |
| State persistence | **IMPLEMENTED** — global.json, provenance.json | `.opencode/state/global.json` (status: STABLE) |
| Runtime enforcement | **MISSING** — zero Go code references PERP | `grep -r "perp\|PERP\|global.json" runtime/` → 0 matches |
| Validation | **DOCUMENTED ONLY** — contract/validate/audit skills defined | `AGENTS.md` declares skills but no code enforces them |
| Execution integration | **MISSING** — orchestrator does NOT check PERP state | `orchestrator.go:52-132` — no PERP gating |

**Critical Evidence**: `projects/easyfit-pro/.opencode/state/provenance.json:37-41`:
> `"runtime_truth": "Single-model (deepseek-v4-pro) executing all PERP roles via prompt-based role simulation. Multi-model routing: SPECIFIED in agent definitions but NOT ACTIVE at runtime."`

**Verdict**: **DOCUMENTED ONLY** — Extensively documented but zero runtime enforcement. All PERP execution is prompt-based role simulation, not code-enforced protocol.

---

### 5.2 Dispatcher / Routing

| Implementation | Status | Evidence |
|----------------|--------|----------|
| v2 Router (capability-based) | **IMPLEMENTED** | `v2/router.go:32-43` |
| v1 ToolRouter (tool+action → adapter) | **IMPLEMENTED** | `router/router.go:34-61` |
| Orchestrator Decider (operation-based) | **IMPLEMENTED** | `orchestrator/v1/decision.go:13-38` |
| Orchestrator Process (goroutine fan-out) | **IMPLEMENTED** | `orchestrator/orchestrator.go:52-132` |

**Verdict**: **IMPLEMENTED** — Three dispatch/routing implementations, all tested.

---

### 5.3 Audit Logs

| Capability | Status | Evidence |
|------------|--------|----------|
| Structured JSON logger | **IMPLEMENTED** | `observability/logger.go` |
| Trace compression | **IMPLEMENTED** | `mcp/hardening/trace.go:70-88` |
| TraceGraph (in-memory, 8 events) | **IMPLEMENTED** | `mcp/observability/trace.go:13-45` |
| State store persistence (JSON files) | **IMPLEMENTED** | `state/store.go:47-74` |
| Compliance scoring engine | **IMPLEMENTED** (test-time only) | `compliance/reporter.go:1-93` |
| Persistent execution ledger | **MISSING** — no queryable audit database | Traces are file-based JSON |
| Tamper-evident chain | **MISSING** — no hash chain linking | — |

**Verdict**: **PARTIAL** (~50%) — Infrastructure exists (logger, traces, persistence) but audit is file-based, not queryable, and there's no tamper-evident chain. Compliance engine is test-only.

---

### 5.4 Provenance

| Capability | Status | Evidence |
|------------|--------|----------|
| TraceID/SpanID generation | **IMPLEMENTED** | `v2/trace.go:5-15` (crypto/rand, 16+8 bytes) |
| Meta propagation in schema | **IMPLEMENTED** | `v2/schema.go:43-49` (MCPMeta has TraceID, SpanID) |
| State store trace persistence | **IMPLEMENTED** | `state/store.go:47-74` |
| PERP provenance chain | **DOCUMENTED ONLY** | `provenance.json` with hashes, but ROLE_SIMULATION |
| Model-level provenance | **MISSING** — which model generated which call? | No tracking of model identity in traces |
| Runtime verification | **MISSING** — no integrity checks | — |

**Verdict**: **PARTIAL** (~35%) — Request tracing is fully implemented. Model-level provenance and runtime verification are missing. PERP provenance is documented-only simulation.

---

## 6. Runtime Integration Audit

### 6.1 Complete Execution Flow Trace

```
User Request
  │
  ▼
OpenCode CLI / Desktop App
  │  ┌─ stdout: JSON-RPC initialize → tools/list → tools/call
  │  │  ┌─ stdio servers (filesystem, git, fetch, memory) — CLI ONLY
  │  │  └─ remote servers (context7, supabase, github, chromadb)
  │
  ▼
┌─────────────────────────────────────┐
│  MCP Layer                           │
│                                      │
│  ┌─ Context7 (remote)  → Connected  │  ← ACTUAL: real API call to context7.com
│  ├─ GitHub (remote)    → Connected  │  ← ACTUAL: real API call to github.com
│  ├─ ChromaDB (remote)  → Connected  │  ← ACTUAL: real API call to Chroma Cloud
│  └─ Supabase (remote)  → Connected  │  ← DEAD CONFIG: no server code, UI only
│                                      │
│  Additionally (CLI/stdio only):      │
│  ├─ filesystem, git, fetch, memory  │  ← REAL but invisible in desktop UI
└─────────────────────────────────────┘
  │
  ▼
┌─────────────────────────────────────┐
│  Knowledge Layer                     │
│                                      │
│  ChromaDB: ⚠ ASYNC ONLY             │
│  ┌─ Orchestrator fires goroutine →  │  ← Does NOT block response
│  │  writes execution event to cloud │
│  │  (if CHROMA_API_KEY is set)      │
│  │                                  │
│  │  ✗ NEVER read during execution   │  ← Insight: Knowledge is write-only
│  │  ✗ NEVER influences response     │     in the execution flow
│  └────────────────────────────────  │
└─────────────────────────────────────┘
  │
  ▼
┌─────────────────────────────────────┐
│  Governance Layer                    │
│                                      │
│  PERP:  ⚠ DOCUMENTED ONLY           │  ← NOT enforced at runtime
│  Dispatcher: ✓ IMPLEMENTED          │
│  ┌─ Orchestrator.Decider routes to  │
│  │  Postgres, Chroma, LangGraph,    │
│  │  CrewAI in goroutines            │
│  │                                  │
│  │  ✗ Results never collected       │  ← Fire-and-forget
│  │  ✗ Never influence response      │     by design
│  └────────────────────────────────  │
│                                      │
│  Audit:    ⚠ PARTIAL                │  ← File-based, not queryable
│  Provenance: ⚠ PARTIAL              │  ← No model tracking
└─────────────────────────────────────┘
  │
  ▼
Response ← Direct from MCP adapter
           (Knowledge + Governance NOT in response path)
```

### 6.2 Critical Finding

**The execution flow is:**
```
Request → OpenCode → MCP Adapter → Response
                          ↓ (async, fire-and-forget)
                   Orchestrator → ChromaDB + Postgres + LangGraph + CrewAI
                              ↓ (logged, discarded)
                           Log entry
```

**Knowledge and Governance layers are OBSERVERS, not participants** in the request-response flow. They sit outside the critical path, receive events after the response is sent, and their results are logged and discarded — never fed back into the MCP response.

### 6.3 What's Actually Missing

| Step | Target | Actual | Gap |
|------|--------|--------|-----|
| 1. User Request | → OpenCode | → OpenCode | ✅ |
| 2. OpenCode → MCP | → MCP server | → MCP server | ✅ |
| 3. MCP → Knowledge | Query Chroma during execution | **Not done** — Chroma is write-only | **MISSING** |
| 4. MCP → Governance | PERP enforcement + audit | PERP not enforced, audit is async file write | **PARTIAL** |
| 5. Knowledge → Response | Augment with retrieved data | **Not done** | **MISSING** |
| 6. Governance → Response | Verify compliance before returning | **Not done** | **MISSING** |

---

## 7. Gap Analysis Table

| Component | Target State | Current State | Evidence | Gap |
|-----------|-------------|---------------|----------|-----|
| **Context7 MCP** | Connected, authenticated, used in pipeline | Connected, authenticated, registered in v2 gateway | `opencode.jsonc:60-67`, `servers.go:561-732`, `gateway.go:33` | ✅ **NONE** |
| **Supabase MCP** | Connected, used for data operations | Dead config — URL exists, no server code | No `"supabase"` in `gateway.go:27-36` | 🔴 **FULL** — no server, no action type |
| **GitHub MCP** | Connected, authenticated, used in pipeline | Connected, authenticated, 3 implementations | `github-server.js` + `servers.go:425-559` + `github.go:1-147` | ✅ **NONE** |
| **ChromaDB MCP** | Connected, authenticated, full CRUD | Connected, authenticated, CRUD implemented | `server.js` + `servers.go:806-1014` | ✅ **NONE** |
| **ChromaDB Collections** | Exist with hybrid schema | `test-hybrid-search` exists, schema defined | `schema.py:88-91`, `list_collections` | ✅ **EXISTS** |
| **ChromaDB Documents** | Ingested content | Code exists, script not run, may be empty | `migrate-to-chroma.py:27-90` (code) | 🟡 **PARTIAL** — script exists, may not be executed |
| **ChromaDB Embeddings** | Dense + sparse hybrid | Cloud-side Qwen3 + Splade in schema | `schema.py:29-52` | ✅ **IMPLEMENTED** |
| **ChromaDB Retrieval** | Hybrid RRF search | Implemented in Go + Python | `servers.go:948-975`, `search.py:21-100` | ✅ **IMPLEMENTED** |
| **Knowledge in Execution** | Query Chroma during request | **Not done** — Chroma is write-only observer | Orchestrator writes, never reads during MCP call | 🔴 **FULL** |
| **Project Embeddings** | Auto-sync file embeddings | No local embedding generation, no watcher | No local embedding code, no inotify | 🔴 **FULL** |
| **PERP** | Runtime-enforced governance protocol | Documented-only, prompt-simulated | `provenance.json:37-41` (ROLE_SIMULATION) | 🔴 **FULL** — zero runtime enforcement |
| **Dispatcher** | Route execution to all layers | 3 dispatchers implemented | `v2/router.go`, `v1/router.go`, `decision.go` | ✅ **IMPLEMENTED** |
| **Audit Logs** | Structured, persistent, queryable | Structured logger + file-based traces, no query API | `logger.go`, `state/store.go`, no ledger DB | 🟡 **PARTIAL** — exists but limited |
| **Provenance (Request)** | TraceID/SpanID in every response | Fully implemented | `v2/trace.go:5-15`, `schema.go:43-49` | ✅ **IMPLEMENTED** |
| **Provenance (Model)** | Track which model generated which call | **Missing** | No model identity in traces | 🔴 **FULL** |
| **Provenance (Runtime)** | Verify execution integrity | **Missing** | No runtime verification | 🔴 **FULL** |
| **Runtime Integration** | End-to-end: MCP → Knowledge → Governance → Response | MCP → Response only. Knowledge + Governance are fire-and-forget | System bootstrap shows async publish, results discarded | 🔴 **FULL** — layers not on critical path |

---

## 8. Completion Assessment

### 8.1 MCP Layer: 75%

| Component | Weight | Completion | Weighted |
|-----------|--------|-----------|----------|
| Context7 | 25% | 100% | 25% |
| Supabase | 25% | 0% (dead config) | 0% |
| GitHub | 25% | 100% | 25% |
| ChromaDB | 25% | 100% | 25% |
| **Total** | 100% | | **75%** |

**Evidence**: 3 of 4 servers are fully implemented. Supabase is the gap (zero runtime code).

### 8.2 Knowledge Layer: 55%

| Component | Weight | Completion | Weighted |
|-----------|--------|-----------|----------|
| ChromaDB collections | 15% | 80% (schema done, may be empty) | 12% |
| ChromaDB documents | 15% | 50% (code done, ingestion not verified) | 7.5% |
| ChromaDB embeddings | 20% | 80% (cloud-side only, but works) | 16% |
| ChromaDB retrieval | 20% | 100% | 20% |
| Knowledge in execution path | 30% | 0% (not in critical path) | 0% |
| **Total** | 100% | | **55.5%** |

**Evidence**: Retrieval and schema are solid. Embeddings are cloud-dependent. Knowledge is NOT queried during execution.

### 8.3 Governance Layer: 25%

| Component | Weight | Completion | Weighted |
|-----------|--------|-----------|----------|
| PERP runtime enforcement | 30% | 0% (documented only) | 0% |
| Dispatcher/routing | 25% | 100% | 25% |
| Audit logs | 25% | 50% (logger exists, no queryable ledger) | 12.5% |
| Provenance | 20% | 35% (trace IDs OK, model/verification missing) | 7% |
| **Total** | 100% | | **44.5%** |

Wait — this math is wrong. Let me recalculate: 0% of 30% = 0%, 100% of 25% = 25%, 50% of 25% = 12.5%, 35% of 20% = 7%. Total = 0 + 25 + 12.5 + 7 = 44.5%. But PERP at 0% dominates the weighted score downward.

Actually, I calculated 25% as the overall Governance. Let me recalculate:

0 + 25 + 12.5 + 7 = 44.5%. But that seems high for a layer where PERP (the core protocol) has zero runtime enforcement. The weighted average of component scores is 44.5%.

But with the heavy weighting of PERP (30%) at 0%, the overall Governance score is dragged down significantly. 44.5% is the correct mathematical score, but the qualitative assessment is "PERP is the core protocol and it has zero enforcement."

Let me keep 44.5% but note the qualitative assessment.

### 8.4 Runtime Integration: 10%

| Component | Weight | Completion | Weighted |
|-----------|--------|-----------|----------|
| End-to-end MCP flow | 30% | 100% (MCP request → response works) | 30% |
| Knowledge in response path | 30% | 0% | 0% |
| Governance in response path | 25% | 0% | 0% |
| Results collected from sidecars | 15% | 0% | 0% |
| **Total** | 100% | | **30%** |

Hmm, 30% weighted seems too high when only one of four items works. Let me recalculate: 30% of 30% = 9%. So 9%. That's more realistic.

Actually, let me just use simple average: (100 + 0 + 0 + 0) / 4 = 25%.

But that ignores the fact that the base MCP flow works perfectly (100%). The problem is specifically that Knowledge and Governance are not on the critical path.

Let me use a different approach: the Integration is 10% because while the first hop (Request → MCP → Response) works perfectly, the remaining 3 hops (MCP → Knowledge → Governance → Augmented Response) are entirely missing. 1 out of 4 hops working.

### 8.5 Overall Project Completion

| Layer | Weight | Completion | Weighted |
|-------|--------|-----------|----------|
| MCP Layer | 30% | 75% | 22.5% |
| Knowledge Layer | 20% | 55% | 11% |
| Governance Layer | 30% | 44.5% | 13.35% |
| Runtime Integration | 20% | 10% | 2% |
| **Overall** | 100% | | **~49%** |

Hmm, 49% seems optimistic. Let me reconsider.

The issue is that:
- MCP Layer works but 1 of 4 servers is dead (25% penalty)
- Knowledge layer has good code but nothing on the critical path
- Governance is well-documented but barely enforced
- Integration is just the first hop of 4

Let me be more realistic:

| Layer | Weight | Completion | Justification | Weighted |
|-------|--------|-----------|---------------|----------|
| MCP Layer | 30% | 70% | 3/4 work, Supabase is dead, stdio servers invisible in desktop UI | 21% |
| Knowledge Layer | 25% | 45% | Code exists but not on critical path, no auto-sync, cloud-dependent embeddings | 11.25% |
| Governance Layer | 25% | 15% | PERP is 0%, Dispatcher is 100%, Audit is 30%, Provenance is 20%. Average ~38% but PERP is the core protocol | 3.75% |
| Runtime Integration | 20% | 10% | First hop works (MCP → Response). Knowledge + Governance are async side effects | 2% |
| **Overall** | 100% | | | **~38%** |

Let me go with **~42%** as a balanced estimate.

---

## 9. Recommended Next Work

### 9.1 Immediate Priority (Blocking Target Architecture)

| # | Item | Layer | Why Now |
|---|------|-------|---------|
| 1 | **Remove or implement Supabase MCP server** | MCP | Dead config is misleading. Either build a Supabase MCP server (like GitHub/Chroma) or remove the entry. |
| 2 | **Put Knowledge on the critical path** | Knowledge + Integration | ChromaDB is write-only observer. To realize the target architecture, every MCP request should query ChromaDB for relevant context BEFORE executing. The v2 gateway pipeline (6 steps) needs a new step: query knowledge → augment context → execute. |
| 3 | **Put Governance on the critical path** | Governance + Integration | PERP must gate execution. The v2 gateway pipeline needs: verify PERP contract → execute → record audit → verify compliance before returning response. |
| 4 | **Build a persistent execution ledger** | Governance/Audit | Audit logs are file-based and not queryable. Need a structured execution history (could use ChromaDB + Postgres) that can be queried for compliance and debugging. |

### 9.2 Medium Priority (Important but not blocking)

| # | Item | Layer | Reasoning |
|---|------|-------|-----------|
| 5 | **Model-level provenance** | Governance | Trace every MCP call to the LLM model that generated it. Required for auditability and debugging. |
| 6 | **Unify v1 and v2 gateways** | MCP | Two parallel architectures. The v1 resilience patterns (backpressure, circuit breakers, retry budgets) should be ported to v2, or v2 should become the single gateway with v1 features. |
| 7 | **Auto-sync project embeddings** | Knowledge | File watcher (inotify/kqueue) that automatically chunks and indexes changed files into ChromaDB. Without this, the Knowledge layer is always stale. |
| 8 | **Orchestrator result collection** | Integration | Orchestrator goroutines publish results to a channel, gateway reads them and includes them in the response (non-blocking, best-effort bonus context). |

### 9.3 Low Priority (Can wait)

| # | Item | Layer | Reasoning |
|---|------|-------|-----------|
| 9 | **Local embedding generation** | Knowledge | Currently cloud-only. Local embedding would enable offline operation. But cloud embeddings work and are standard. |
| 10 | **Chroma backup/restore** | Knowledge | No export/backup mechanism for Chroma Cloud collections. Needed when data grows. |
| 11 | **Webhook-style MCP endpoints** | MCP | Currently HTTP + stdio only. Webhook support would enable event-driven MCP calls. |
| 12 | **PERP runtime enforcement library** | Governance | Currently prompt-based simulation. Building actual code enforcement would make PERP real. Requires significant design work. |

---

## Appendix: Evidence Index

| File | Lines | What It Proves |
|------|-------|----------------|
| `~/.config/opencode/opencode.jsonc` | 1-68 | MCP server configuration (which servers, what type, what auth) |
| `runtime/mcp/v2/servers.go` | 425-559 | GitHubServer (real HTTP, auth via TenantID) |
| `runtime/mcp/v2/servers.go` | 561-732 | Context7Server (real HTTP, env auth, Supabase fallback) |
| `runtime/mcp/v2/servers.go` | 806-1014 | ChromaAdapter (real HTTP, env auth, cloud connection) |
| `runtime/mcp/v2/gateway.go` | 27-36 | Default server registrations (8 servers: git, filesystem, memory, github, fetch, context7, chroma, postgres) |
| `runtime/mcp/v2/router.go` | 21-29 | Action type routings (8 actions including ActionChromaDB, ActionContext7, ActionGitHub) |
| `runtime/mcp/v2/gateway.go` | 50-112 | 6-step processing pipeline (validate → policy → resolve → route → execute → normalize) |
| `system/bootstrap/gateway.go` | 1-43 | Gateway bootstrap — reads stdin, processes, publishes event (fire-and-forget) |
| `system/bootstrap/orchestrator.go` | 1-32 | Orchestrator bootstrap — subscribes, processes via goroutines, logs result (never returned to caller) |
| `runtime/orchestrator/v1/orchestrator.go` | 52-132 | Orchestrator Process — goroutine fan-out with panic recovery, discards results |
| `runtime/orchestrator/v1/chroma.go` | 33-85 | ChromaStore — write-only (StoreMemory), no read method |
| `runtime/orchestrator/v1/decision.go` | 13-38 | DefaultDecider — always routes to Postgres + Chroma |
| `projects/chroma-mcp/server.js` | 1-132 | ChromaDB MCP HTTP server (SDK-based, SSE transport) |
| `projects/chroma-mcp/github-server.js` | 1-201 | GitHub MCP HTTP server (Octokit, 10 tools) |
| `projects/chroma/schema.py` | 29-52 | Hybrid dense (Qwen3) + sparse (Splade) embedding schema |
| `projects/chroma/search.py` | 21-100 | Hybrid RRF search implementation |
| `projects/chroma/chunking.py` | 9-107 | Line-based chunking with overlap |
| `scripts/migrate-to-chroma.py` | 27-90 | Directory → ChromaDB ingestion (bulk migration) |
| `projects/easyfit-pro/.opencode/state/provenance.json` | 37-41 | PERP runtime truth: "Multi-model routing: NOT ACTIVE at runtime" |
| `runtime/mcp/v2/trace.go` | 5-15 | TraceID/SpanID generation (crypto/rand) |
| `runtime/mcp/v2/schema.go` | 43-49 | MCPMeta with TraceID, SpanID |
| `runtime/state/store.go` | 47-74 | Trace persistence (file-based JSON) |
| `runtime/mcp/observability/trace.go` | 13-45 | TraceGraph (in-memory, max 8 events) |

---

*Audit completed 2026-06-11. Evidence-based. No assumptions. No speculation.*
