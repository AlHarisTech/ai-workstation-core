# Changelog

All notable changes to AI Workstation Core are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
