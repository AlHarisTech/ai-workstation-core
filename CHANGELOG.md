# Changelog

All notable changes to AI Workstation Core are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.4.0] — 2026-06-10

### Added

- **Execution Queue Layer** — bounded FIFO `RequestQueue` with configurable max size and backpressure (`QUEUE_FULL` rejection)
- **Shared Worker Pool** — N persistent worker threads (configurable via `AI_GATEWAY_WORKERS`) pulling from the queue
- **Persistent State Layer** — file-based `StateStore` (.ai/state/) with atomic writes for sessions, execution traces, and platform metadata
- **Composable Policy Engine** — `policy_decision_graph` tracks all policy decisions (allow + deny) per request
- **Pipeline Dual Mode** — `strict` (7 stages) and `optimized` (6 stages) selectable per request via `pipeline_mode` field
- **Latency Breakdown** — per-request `queue_wait_time_ms`, `routing_ms`, `execution_ms`, `audit_ms`, `validation_ms`
- **Worker Observability** — `worker_id` and `queue_wait_time_ms` in every response and audit entry

### Changed

- `runtime/mcp_gateway/main.py` — rewritten: queue-driven architecture with worker pool, result collector, state persistence
- `runtime/mcp_gateway/pipeline.py` — dual mode support; `_finalize_latency()`, `_run_audit_log()` on all paths
- `runtime/mcp_gateway/context.py` — added `queue_wait_time_ms`, `worker_id`, `pipeline_mode`, `latency_breakdown`, `policy_decision_graph`
- `runtime/governance/policy_engine.py` — `evaluate_stage()` appends to `policy_decision_graph`
- `.ai/ARCHITECTURE.md` §13 — updated for v0.4.0 kernel architecture
- `.ai/GOVERNANCE.md` §5 — added queue/worker governance, enhanced context fields
- `.ai/ADR_LOG.md` — added ADR-007

---

## [0.3.0] — 2026-06-10

### Added

- **Middleware pipeline architecture** — 7-stage deterministic pipeline (PreValidation → SessionGuard → CapabilityRouting → PreExecution → Execution → PostValidation → AuditLog) with explicit StageResult per stage
- **Request Context Graph** — `RequestContext` dataclass with mandatory fields (request_id, session_id, project_id), execution_trace array, decision_path, stage_timings, lifecycle timestamps
- **Governance Enforcement Engine** — `PolicyEngine` (runtime/governance/policy_engine.py) with 6 YAML-defined policies evaluated BEFORE execution; DENY blocks pipeline
- **Runtime governance policies** — `runtime/governance/policies/runtime.yaml` with POL-001 to POL-006 covering field presence, tool existence, session gating, path access, timeout, error isolation
- **Tool isolation layer** — `execute_isolated()` wraps tool execution with ThreadPoolExecutor timeout; EXECUTION_TIMEOUT and EXECUTION_ERROR envelopes prevent cascade failure
- **SessionGuard middleware** — formal session validation pipeline stage (runtime/session/session_guard.py)
- **Registry versioning system** — `SUPPORTED_VERSIONS` validates registry YAML at load time; incompatible versions raise fatal error
- **Enhanced observability** — audit log captures execution_trace, decision_path, stage_timings, error_code per request

### Changed

- `runtime/logging/` → `runtime/auditlog/` (stdlib `logging` module collision fix)
- `runtime/tools/definitions.yaml` — bumped registry version to 0.3.0
- `runtime/mcp_gateway/main.py` — rewritten to use Pipeline + PipelineServices
- `runtime/executor/local_executor.py` — added `execute_isolated()` with timeout
- `runtime/tools/registry.py` — added version validation, `IncompatibleRegistryVersionError`
- `.ai/ARCHITECTURE.md` §13 — updated for v0.3.0 control plane pipeline details
- `.ai/GOVERNANCE.md` §5 — updated for governance enforcement engine and isolation
- `.ai/ADR_LOG.md` — added ADR-006

---

## [0.2.0] — 2026-06-10

### Added

- **MCP Gateway runtime** — stdio-based gateway process (`runtime/mcp_gateway/main.py`) with full request pipeline: Parse → Route → Session Validate → Execute → Audit
- **Tool Registry runtime** — YAML-driven tool registry (`runtime/tools/registry.py`) with capability-based lookup and in-memory state
- **Session validation layer** — fail-closed validator (`runtime/session/session_validator.py`) enforcing session_id and project_id requirements
- **Local execution adapter** — 8 tool handlers for local function execution (echo, filesystem_read, filesystem_write, git_status, fetch_url, memory_store, memory_retrieve, session_create)
- **Structured logging** — append-only JSON audit log (`runtime/logging/structured_logger.py`) with full lifecycle trace per request
- **Tool definitions** — YAML registry (`runtime/tools/definitions.yaml`) with 8 registered tools, capabilities, and governance metadata

### Changed

- `.ai/ARCHITECTURE.md` — Added §13: MCP Gateway Execution Mapping and Runtime Truth Alignment notes
- `.ai/GOVERNANCE.md` — Added §5: Runtime Execution Principles (execution-first, fail-closed, deterministic logging, minimal surface, pipeline order)
- `.ai/ADR_LOG.md` — Added ADR-005: MCP Gateway Runtime Implementation
- `.gitignore` — Added `.ai/governance/audit/*.log` exclusion

---

## [0.1.0] — 2026-06-10

### Added

- **Architecture specification** — Complete Phase 1 architecture covering all 15 sections:
  - System classification and directory structure
  - MCP Gateway design with Unix socket transport
  - Tool Registry with 12 tool definitions
  - Capability Routing with domain-based resolution
  - Dynamic Tool Injection with 4 injection methods
  - Session Lifecycle with 6-state state machine
  - Security Boundaries with 6 enforcement layers
  - Multi-Project Isolation with 6 orthogonal dimensions
  - ChromaDB Namespace Strategy with hierarchical model
  - Runtime Governance with 6 policy categories
  - Integration points (Supabase, ChromaDB, OpenCode, LangGraph, CrewAI, Context7)
  - Expansion points (12 future directions)
  - Key tradeoffs documentation

- **Governance constitution** — `.ai/GOVERNANCE.md` with 6 platform laws, 9 forbidden patterns, operational boundaries, and compliance rules.

- **Architectural Decision Log** — `.ai/ADR_LOG.md` with 4 initial ADRs covering baseline architecture, gateway transport, ChromaDB namespaces, and tool injection.

- **Repository foundation artifacts**:
  - README.md with project overview, architecture summary, and roadmap
  - LICENSE (Apache 2.0)
  - CHANGELOG.md (this file)
  - VERSION file
  - CONTRIBUTING.md
  - CODE_OF_CONDUCT.md
  - .gitignore

- **Directory structure** — `.ai/` with all subdirectories for gateway, registry, routing, sessions, governance, memory, context, agents, and config.
