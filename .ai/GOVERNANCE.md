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

## 5. Runtime Execution Principles (v0.3.0)

These principles govern all runtime code under `/runtime/`.

### 5.1 Middleware Pipeline (Enforced Order)
The gateway MUST process tool.call requests through the 7-stage pipeline:
```
PreValidation → SessionGuard → CapabilityRouting → PreExecution
              → Execution → PostValidation → AuditLog
```
Each stage returns `StageResult(decision="allow"|"deny")`. **DENY stops the pipeline immediately** (fail-closed). Deviation from this order is a CRITICAL governance violation.

### 5.2 Governance Enforcement (Runtime, Not Documentation)
- PolicyEngine loads 6 policies from `runtime/governance/policies/runtime.yaml` at startup.
- Every policy is evaluated BEFORE the corresponding pipeline stage executes.
- DENY verdicts terminate the pipeline — execution never begins after a policy denial.
- All policy decisions are logged (policy_id, decision, reason).

### 5.3 Tool Isolation
- `execute_isolated()` wraps every tool call with timeout and cascade containment.
- Timeout: default 30s. Exceeded → `EXECUTION_TIMEOUT` error envelope. Tool thread abandoned.
- Exception: caught and wrapped in `EXECUTION_ERROR` error envelope. Stack trace preserved.
- **Tool failure MUST NOT cascade to the gateway.** Gateway always remains responsive.

### 5.4 Registry Versioning
- `SUPPORTED_VERSIONS` in `ToolRegistry` defines compatible registry YAML versions.
- Loading an unsupported version raises `IncompatibleRegistryVersionError` → gateway fatal.
- No silent forward-incompatible registries. No breaking changes without version validation.

### 5.5 Request Context Graph
Every request carries a `RequestContext` with mandatory fields:
- `request_id`, `session_id`, `project_id` (all required by POL-001)
- `execution_trace`: ordered array of `StageResult` objects
- `decision_path`: ordered list of stage names visited
- `stage_timings`: dict[stage_name, ms duration]
- `timestamp_start` / `timestamp_end`: lifecycle boundaries

### 5.6 Deterministic Logging (Enhanced)
- Every request produces exactly one audit log entry.
- Log entries include: `execution_trace` (full stage history), `decision_path`, `stage_timings`, `error_code`.
- Log is append-only. No rotation, no truncation in v0.3.0.

---

## 6. Compliance

- All changes to `.ai/` require an ADR entry.
- All ADRs must document Forbidden Alternatives and Failure Modes Prevented.
- Platform changes must not break existing project `.ai.yaml` configurations.
- Every new tool added to `definitions.yaml` must have a handler in `LocalExecutor.HANDLERS`.
- Every tool handler must return `{"status": "success"|"error", ...}` — never raise.
- Governance audit runs must pass before platform changes are accepted.
