# AI Engineering Workstation — Architectural Decision Log

> **Persistent decision registry.** Every architectural decision for the AI Workstation platform is recorded here.
>
> **Discipline:** No silent deviation. Every structural change produces an ADR.

---

## ADR Index

| ADR | Title | Status | Date |
|---|---|---|---|
| ADR-001 | AI Workstation Baseline Architecture | Accepted | 2026-06-10 |
| ADR-002 | Unix Socket Gateway over TCP | Accepted | 2026-06-10 |
| ADR-003 | Per-Project ChromaDB Namespaces | Accepted | 2026-06-10 |
| ADR-004 | Session-Scoped Tool Injection | Accepted | 2026-06-10 |
| ADR-005 | MCP Gateway Runtime Implementation (v0.2.0) | Accepted | 2026-06-10 |
| ADR-006 | Deterministic Control Plane Kernel (v0.3.0) | Accepted | 2026-06-10 |
| ADR-007 | Adaptive Runtime Kernel (v0.4.0) | Accepted | 2026-06-10 |
| ADR-008 | Compliance Verification & Evidence Layer | Accepted | 2026-06-10 |

---

## ADR-001: AI Workstation Baseline Architecture

**Status:** Accepted
**Date:** 2026-06-10
**Author:** Staff Engineer

### Context
Need a project-agnostic AI infrastructure platform that serves all current and future software projects from a single workspace root. Infrastructure must be isolated from project source code.

### Decision
Establish `.ai/` as the single authoritative location for all AI infrastructure at the workspace root. Projects declare AI configuration via a minimal `.ai.yaml` file. The gateway, registry, routing, sessions, governance, memory, context, and agents all live under `.ai/`.

### Forbidden Alternatives
- AI config per project (duplication, no central governance)
- AI config in global home directory (invisible, not version-controllable per workspace)
- Monolithic tool integration (no isolation, no runtime injection)

### Consequences
- **Positive:** Clean separation, central governance, multi-project support
- **Negative:** `.ai/` must be explicitly included in workspace tooling config

### Failure Modes Prevented
- AI infrastructure mixed with project source code
- Cross-project configuration drift
- No single point of governance enforcement

---

## ADR-002: Unix Socket Gateway over TCP

**Status:** Accepted
**Date:** 2026-06-10
**Author:** Staff Engineer

### Context
The MCP Gateway needs a transport layer for local agent-to-tool communication. Options: TCP, Unix socket, or both.

### Decision
Primary transport is Unix socket at `.ai/gateway/gateway.sock`. Secondary TCP on localhost with TLS for services that require it (e.g., SSE MCP servers).

### Forbidden Alternatives
- TCP-only (network attack surface, no peer credential auth)
- Named pipes (less portable, harder to monitor)

### Consequences
- **Positive:** Kernel-authenticated peer credentials, no network exposure
- **Negative:** Not accessible from remote hosts, Docker containers require socket mount

### Failure Modes Prevented
- Remote exploitation of gateway
- Unauthenticated tool access

---

## ADR-003: Per-Project ChromaDB Namespaces

**Status:** Accepted
**Date:** 2026-06-10
**Author:** Staff Engineer

### Context
Vector store needs to isolate project data while sharing a single ChromaDB instance.

### Decision
Namespace strategy: `global:*` for platform data, `project:<project-id>:*` for per-project data. Namespace enforced at the gateway level, not the database level.

### Forbidden Alternatives
- Separate ChromaDB instances per project (resource waste, management overhead)
- Single flat namespace (no isolation)
- Single collection with metadata filtering (query leak risk, complex access control)

### Consequences
- **Positive:** Strict isolation, single instance, gateway-enforced
- **Negative:** No cross-project vector search without explicit bridge

### Failure Modes Prevented
- Cross-project memory contamination
- Accidental data access via incorrect filter

---

## ADR-004: Session-Scoped Tool Injection

**Status:** Accepted
**Date:** 2026-06-10
**Author:** Staff Engineer

### Context
Tools (MCP servers) need to be available per session without manual startup. Decision between session-scoped (start/stop per session) and persistent (always running).

### Decision
Injection method varies per tool:
- `session-scoped`: ephemeral tools (fetch, langgraph workflows)
- `project-scoped`: tools shared within project sessions (postgresql, git)
- `persistent`: always-on services (gateway, context7, chromadb)

### Forbidden Alternatives
- All tools persistent (resource waste, unnecessary attack surface)
- All tools session-scoped (startup latency for every session, connection churn)

### Consequences
- **Positive:** Right-sized lifecycle per tool type, resource efficient
- **Negative:** More complex lifecycle management code

### Failure Modes Prevented
- Orphaned tool processes
- Startup latency on every session for heavyweight tools

---

## ADR-005: MCP Gateway Runtime Implementation (v0.2.0)

**Status:** Accepted
**Date:** 2026-06-10
**Author:** Staff Engineer

### Context
Architecture specification (v0.1.0) exists. No runtime. Need a minimal, runnable MCP Gateway that demonstrates the full pipeline: request → session validation → capability routing → tool execution → audit logging.

### Decision
Implement the v0.2.0 runtime with these constraints:
1. **Python stdio gateway** — no Unix sockets, no TCP. stdin/stdout for transport.
2. **Local function execution** — all tools execute as Python methods. No subprocesses, no containers.
3. **YAML-driven registry** — tool definitions loaded from `runtime/tools/definitions.yaml` at startup.
4. **Append-only JSON audit log** — one JSON line per request at `.ai/governance/audit/gateway.log`.
5. **Fail-closed session validation** — missing `session_id` or `project_id` → immediate rejection for tools that require session.
6. **8 tools implemented**: echo, filesystem_read, filesystem_write, git_status, fetch_url, memory_store, memory_retrieve, session_create.

### Forbidden Alternatives
- Unix socket gateway (adds complexity before stdio is proven)
- Container-based tool injection (Phase 3/4 scope, not v0.2.0)
- Subprocess execution backend (adds process lifecycle management prematurely)
- Inline Python tool definitions (YAML registry maintains architecture alignment)
- Multi-file JSON logging with rotation (YAGNI for MVP)
- Any agent framework integration (LangGraph/CrewAI — explicitly OUT of scope)

### Consequences
- **Positive:** Working runtime, end-to-end pipeline verified, deterministic logging, architecture-to-execution traceability
- **Negative:** stdio transport limits concurrency to one request per process. No network services. YAML dependency requires `pyyaml`.

### Operational Reasoning
The stdio gateway processes one JSON request per line from stdin. This is sufficient for single-agent, sequential operation — which matches the current OpenCode usage pattern. The pipeline order is enforced: Route → Session → Execute → Audit. Every request produces one audit entry. Fail-closed rejection prevents silent errors.

### Failure Modes Prevented
- Architecture-only drift (spec without execution = untested architecture)
- Silent session bypass (tools that require session now hard-fail on missing credentials)
- Path traversal (deny list prevents `/etc/`, `/proc/`, `.ai/config/secrets/` access)
- Unknown tool crash (caught and returned as structured error)
- Audit gap (all requests logged; log is append-only)

### Governance Implications
Establishes: **Every registry entry must have a runtime handler.** Drift between `definitions.yaml` and `LocalExecutor.HANDLERS` is a CRITICAL governance violation. Establishes execution-first principle: no abstraction without runtime backing.

### Compliance
- `.ai/governance/audit/gateway.log` must contain exactly one entry per gateway request
- Every tool handler must return `{"status": "success"|"error", ...}` — never raise
- `python3 runtime/mcp_gateway/main.py` must start and process a request end-to-end
- Session rejection must produce `code: "SESSION_INVALID"` response

### Related
- ADR-001: AI Workstation Baseline Architecture
- ADR-002: Unix Socket Gateway over TCP (deferred for v0.2.0 — stdio used instead)
- ADR-003: Per-Project ChromaDB Namespaces (memory_store/retrieve use in-process dict, not ChromaDB)
- ADR-004: Session-Scoped Tool Injection (session_create/validate implemented, tool scoping deferred)

---

## ADR-006: Deterministic Control Plane Kernel (v0.3.0)

**Status:** Accepted
**Date:** 2026-06-10
**Author:** Staff Engineer

### Context
v0.2.0 gateway used ad-hoc linear execution with no formal pipeline, no governance runtime enforcement, no tool isolation, and minimal observability. The system needed to evolve from a prototype into a deterministic control plane kernel where every operation is traceable, enforcible, and isolated.

### Decision
Implement a deterministic control plane kernel with:
1. **7-stage middleware pipeline** — PreValidation, SessionGuard, CapabilityRouting, PreExecution, Execution, PostValidation, AuditLog. Each stage explicit and independently measurable. DENY stops the pipeline.
2. **RequestContext graph** — every request carries `execution_trace`, `decision_path`, `stage_timings`, and lifecycle timestamps.
3. **Governance Enforcement Engine** — `PolicyEngine` loads 6 YAML-defined policies. Evaluated BEFORE execution. Runtime DENY blocks execution.
4. **Tool isolation layer** — `execute_isolated()` wraps tools with `ThreadPoolExecutor` timeout. Timeout → `EXECUTION_TIMEOUT` envelope. Exception → `EXECUTION_ERROR` envelope. No cascade.
5. **Registry versioning** — `SUPPORTED_VERSIONS` validates registry YAML at load time. Unsupportable versions → `IncompatibleRegistryVersionError` (fatal).
6. **Enhanced observability** — audit log captures full execution_trace, decision_path, stage_timings, error_code.

### Forbidden Alternatives
- Inline pipeline stages in main.py (untestable, no stage-level metrics)
- Governance as documentation-only (v0.2.0 state — no runtime enforcement)
- No isolation (tool failure could crash gateway)
- No version validation (silent registry incompatibility)
- Monolithic request handling (no per-stage timing, no trace reconstruction)

### Consequences
- **Positive:** Deterministic pipeline, runtime governance enforcement, tool isolation, full traceability, backward-compatible registry loading
- **Negative:** `runtime/logging/` renamed to `runtime/auditlog/` (stdlib collision). Stderr output no longer captured (pipeline handles errors internally).

### Operational Reasoning
The 7-stage pipeline provides deterministic ordering. Each stage is independently measurable — stage_timings reveal exactly where latency occurs. Governance enforcement is now runtime: POL-004 (path-access-control) blocks `/etc/passwd` reads at the PreExecution stage. Tool isolation prevents a misbehaving tool from crashing the gateway — timeout containment ensures gateway always remains responsive.

### Failure Modes Prevented
- Cascade failure: tool exception no longer crashes gateway (envelope containment)
- Governance bypass: path deny list now enforced at PreExecution stage by PolicyEngine
- Silent registry incompatibility: unknown versions reject at load time
- Missing observability: full execution_trace reconstructable from audit log alone
- Undetected mandatory field absence: PreValidation stage enforces request_id, session_id, project_id

### Governance Implications
Establishes: **Governance is runtime, not documentation.** Every policy in `runtime/governance/policies/runtime.yaml` is evaluated by `PolicyEngine` before the corresponding pipeline stage executes. DENY verdicts are terminal. Establishes pipeline order as a CRITICAL governance violation if deviated from.

### Compliance
- `runtime/governance/policies/runtime.yaml` must contain exactly 6 policies
- All 7 pipeline stages must execute in order for every `tool.call` request
- `execute_isolated()` must be used for all tool dispatch (never bare `execute()`)
- Registry `.yaml` version must be validated at load time
- Audit log entries must include `execution_trace`, `decision_path`, and `stage_timings`

### Related
- ADR-005: MCP Gateway Runtime Implementation (v0.2.0 — predecessor)
- ADR-003: Per-Project ChromaDB Namespaces (memory_store/retrieve still in-process)
- ADR-002: Unix Socket Gateway over TCP (still deferred — stdio used)

---

## ADR-007: Adaptive Runtime Kernel (v0.4.0)

**Status:** Accepted
**Date:** 2026-06-10
**Author:** Staff Engineer

### Context
v0.3.0 control plane processed requests sequentially through a single-threaded pipeline. No queue, no concurrency, no persistence. The kernel needed to handle multiple requests concurrently with backpressure, shared resources, and state durability.

### Decision
Implement an adaptive runtime kernel:
1. **Execution Queue** (`RequestQueue`) — bounded FIFO with backpressure. Full queue → `QUEUE_FULL` rejection.
2. **Shared Worker Pool** — N persistent threads pulling from the queue. Configurable via `AI_GATEWAY_WORKERS`.
3. **Persistent State** (`StateStore`) — file-based JSON storage for sessions, execution traces, and platform metadata. Atomic writes (temp → rename).
4. **Composable Policy Engine** — `policy_decision_graph` appended per request. All policy decisions (allow + deny) tracked.
5. **Dual Pipeline Modes** — `strict` (7 stages + audit on all paths) and `optimized` (6 stages, audit only on completion). Selectable per request via `pipeline_mode` field.
6. **Enhanced Observability** — `queue_wait_time_ms`, `worker_id`, `latency_breakdown`, and `policy_decision_graph` per request.

### Forbidden Alternatives
- External message broker (distributed systems — out of scope)
- External database for state (file-based persistence sufficient)
- Per-request thread pool (expensive, no worker reuse)
- Always-audit mode only (reduced observability for OPTIMIZED cases)
- Dynamic pipeline reconfiguration (added complexity without benefit)

### Consequences
- **Positive:** Concurrent request processing, backpressure protection, durable state, per-request mode selection, full policy graph
- **Negative:** Slight startup latency for worker pool initialization. Optimized mode must still run audit_log at pipeline completion.

### Failure Modes Prevented
- Queue exhaustion: bounded queue with backpressure rejects overflow
- State loss: atomic writes prevent partial persistence
- Worker crash isolation: each worker runs in its own thread; supervisor (main) thread handles fatal errors
- Pipeline mode mismatch: context validates mode at creation; unknown modes default to strict
- Policy blind spots: all policy decisions (allow + deny) captured in `policy_decision_graph`

### Governance Implications
Establishes: **Queue depth and worker count are configurable, not hard-coded.** Environment variables `AI_GATEWAY_WORKERS` and `AI_QUEUE_SIZE` control at startup. Establishes `QUEUE_FULL` as a structured error code for backpressure rejection.

### Compliance
- `.ai/state/` must contain `meta.json` after gateway startup
- Every request must include `worker_id` and `queue_wait_time_ms` in response
- `policy_decision_graph` must have ≥ 3 entries for valid requests
- OPTIMIZED_MODE requests must still produce audit log entries
- StateStore writes must be atomic (temp file → rename verified)

### Related
- ADR-006: Deterministic Control Plane Kernel (v0.3.0 — predecessor)
- ADR-005: MCP Gateway Runtime Implementation (v0.2.0)
- ADR-003: Per-Project ChromaDB Namespaces

---

## ADR-008: Compliance Verification & Evidence Layer (v0.6.1)

**Status:** Accepted
**Date:** 2026-06-10
**Author:** Staff Engineer

### Context

The kernel was feature-complete through v0.6.0 with semantic guarantees for fairness, replay, SLA, policy enforcement, and graceful shutdown. No mechanism existed to prove these guarantees hold at any given point in time. Compliance was asserted, not verified.

### Decision

Create a formal Compliance Verification & Proof Layer capable of validating all semantic guarantees through measurable, reproducible evidence:

1. **5 Verifiers** — `FairnessVerifier`, `ReplayVerifier`, `SLAVerifier`, `PolicyVerifier`, `ShutdownVerifier` — each producing a structured compliance report.
2. **Compliance Score Engine** — weighted scoring: Fairness 20, Replay 25, SLA 20, Policy 20, Shutdown 15 = 100 total.
3. **Certification Levels** — Production Certified (95+), Production Ready (85+), Limited Production (70+), Non-Compliant (<70).
4. **Evidence Reproducibility** — all verifiers create isolated temporary workspaces. Deterministic: same input → same score.
5. **Compliance Reporter** — JSON output via `ComplianceReport` struct, persisted via `WriteComplianceReport()`.

### Forbidden Alternatives
- Manual verification (non-reproducible, not auditable)
- Separate test framework (duplicate infrastructure)
- Documentation-only compliance (not evidence-based)
- Runtime compliance check (overhead on hot path; verification is offline)

### Consequences
- **Positive:** Provable guarantees, reproducible evidence, automated certification scoring
- **Negative:** `compliance_test.go` produces output to stdout during tests (by design — evidence must be visible)

### Failure Modes Prevented
- Silent fairness violations (detected by starvation monitoring)
- Replay divergence (execution hash mismatch caught)
- SLA drift (p50/p95/p99 measured against contract thresholds)
- Policy bypass (execution never occurs after DENY — verified)
- Shutdown data loss (inflight == completed, zero lost)

### Compliance
- `go test -v ./runtime/compliance/` must pass all 5 domains
- Any BYPASS in policy verifier → FAIL (MUST NOT pass)
- Any request lost in shutdown verifier → FAIL
- SLA violations exceeding thresholds → FAIL
- Fairness max starvation > 30s → FAIL

### Related
- ADR-007: Adaptive Runtime Kernel (v0.4.0)
- ADR-006: Deterministic Control Plane Kernel (v0.3.0)
- `.ai/SPEC.md` §4-8 (semantic guarantees being verified)
