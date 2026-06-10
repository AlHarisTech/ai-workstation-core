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
