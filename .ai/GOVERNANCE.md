# AI Engineering Workstation — Governance Constitution

> **Platform governance constitution.** Defines hard boundaries, forbidden patterns, and mandatory disciplines for all changes to the AI Workstation platform.

---

## 1. Platform Classification

The AI Engineering Workstation is a **Project-Agnostic AI Infrastructure Platform**.

| Classification | Meaning |
|---|---|
| **Platform** | Hosts and orchestrates AI capabilities consumed by multiple projects |
| **Project-Agnostic** | Zero assumptions about any project's language, framework, or domain |
| **Infrastructure** | Foundational layer — stable, minimal, invisible to projects |
| **Governed** | All changes require ADR. No silent deviation. |

---

## 2. Platform Laws

### Law 1: Project Agnosticism
- No component in `.ai/` may reference a specific project by name in its logic.
- Project-specific configuration lives in `.ai/governance/policies/` and `.ai/routing/policies/` — NOT in code.
- A new project must be onboardable by adding config only — zero code changes.

### Law 2: Isolation
- Every project operates in an isolated sandbox (filesystem, process, memory, database).
- Cross-project access is DENIED by default, ALLOWED only by explicit governance exception.
- No tool subprocess may access files outside its project's workspace root.

### Law 3: Fail-Closed
- Every enforcement point must reject on uncertainty. Silent fallback is forbidden.
- Missing policy → deny. Missing tool registration → reject. Missing session context → reject.

### Law 4: Evidence-First
- All tool invocations produce audit records.
- Governance violations produce structured reports.
- No operation is invisible.

### Law 5: Minimal-Diff
- Platform changes must minimize disruption to existing projects.
- New capabilities must be additive (opt-in, not breaking).
- Configuration defaults must match existing behavior.

### Law 6: No Mock Implementations
- Every component in `.ai/` must be production-grade.
- Test doubles are permitted in test directories only.
- No "placeholder" or "TODO" implementations in production paths.

---

## 3. Forbidden Patterns

| Pattern | Why Forbidden |
|---|---|
| **Project-specific code in platform** | Violates Law 1. Creates coupling. |
| **Silent fallback on auth/authz failure** | Violates Law 3. Masks security failures. |
| **No-op audit entries** | Violates Law 4. Audit must contain meaningful data. |
| **Breaking config changes without migration** | Violates Law 5. Destroys existing project compatibility. |
| **Stub/placeholder implementations** | Violates Law 6. Masks incomplete work. |
| **Gateway bypass** | All tool access must go through the gateway. Direct tool invocation is forbidden. |
| **Magic discovery** | All tools must be registered in the registry. Dynamic discovery without registration is forbidden. |
| **Hardcoded secrets** | All secrets in `.ai/config/secrets/`, encrypted at rest. |
| **Cross-project default access** | Cross-project is always opt-in via explicit governance exception. |

---

## 4. Operational Boundaries

| Component | Owns | Does NOT Own |
|---|---|---|
| **Gateway** | Connection routing, auth, rate limiting | Tool logic, data storage |
| **Registry** | Tool definitions, capability declarations | Runtime tool state |
| **Routing** | Intent→tool mapping, access policies | Tool execution, session state |
| **Sessions** | Lifecycle state, context assembly | Tool injection, governance enforcement |
| **Governance** | Policies, enforcement, audit | Session management, routing |
| **Memory** | Vector store, file store | Session context, agent definitions |

---

## 5. Runtime Execution Principles (v0.2.0)

These principles govern all runtime code under `/runtime/`.

### 5.1 Execution-First Architecture
- No abstraction may exist without a runtime implementation.
- Every registry entry (`definitions.yaml`) must have a corresponding handler in `LocalExecutor.HANDLERS`.
- Drift between registry declarations and handler mapping is a CRITICAL governance violation.

### 5.2 Fail-Closed in Execution
- Session validation: missing `session_id` or `project_id` → reject immediately, do not proceed.
- Path access: denied paths (`/etc/`, `/proc/`, `.ai/config/secrets/`) → reject before I/O.
- Unknown tools: tool not in registry → return `TOOL_NOT_FOUND`, never crash.
- Invalid JSON: malformed input → return `PARSE_ERROR`, never crash.

### 5.3 Deterministic Logging
- Every request must produce exactly one audit log entry.
- Log entries always include: `request_id`, `session_id`, `project_id`, `tool_id`, `status`, `execution_result`, `timestamp`.
- Log is append-only. No rotation, no truncation in v0.2.0.

### 5.4 Minimal Surface
- v0.2.0 uses `stdio` (stdin/stdout) for transport. No network listeners.
- v0.2.0 uses local function execution. No containers, no subprocesses for tool execution.
- Python standard library only, except `pyyaml` for registry loading.

### 5.5 Pipeline Order (Enforced)
The gateway MUST process requests in this order:
```
Parse JSON → Route (resolve tool_id) → Check Tool Exists
  → Validate Session (if tool requires) → Execute Tool
  → Assemble Response → Log Audit → Emit Response
```
Deviation from this order is a MEDIUM governance violation.

---

## 6. Compliance

- All changes to `.ai/` require an ADR entry.
- All ADRs must document Forbidden Alternatives and Failure Modes Prevented.
- Platform changes must not break existing project `.ai.yaml` configurations.
- Every new tool added to `definitions.yaml` must have a handler in `LocalExecutor.HANDLERS`.
- Every tool handler must return `{"status": "success"|"error", ...}` — never raise.
- Governance audit runs must pass before platform changes are accepted.
