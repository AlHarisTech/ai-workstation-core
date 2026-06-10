# AI Workstation Core

**Project-agnostic AI infrastructure platform for multi-project software engineering.**

AI Workstation Core is a production-grade platform that provides AI capabilities (agents, tooling, memory, governance) to any software project — without coupling to any specific language, framework, or domain.

---

## Architecture

All AI infrastructure lives under `.ai/`, isolated from project source code:

```
.ai/                              # AI Workstation root
├── ARCHITECTURE.md               # Platform architecture specification
├── ADR_LOG.md                    # Architectural Decision Log
├── GOVERNANCE.md                 # Platform governance constitution
├── gateway/                      # MCP Gateway — central routing layer
├── registry/                     # Tool Registry — inventory of all tools
├── routing/                      # Capability Routing — intent → tool mapping
├── sessions/                     # Session Lifecycle management
├── governance/                   # Runtime Governance Model
├── memory/                       # Memory subsystem (ChromaDB + filesystem)
├── context/                      # Context7 integration
├── agents/                       # Agent definitions
└── config/                       # Platform-wide configuration
```

Projects consume AI capabilities through well-defined interfaces via the MCP Gateway. Each project declares its AI needs in a minimal `.ai.yaml` configuration file.

---

## Supported Tools & Capabilities

| Category | Tools |
|---|---|
| **Orchestrators** | OpenCode |
| **Agent Frameworks** | LangGraph, CrewAI |
| **Data Sources** | PostgreSQL (MCP), ChromaDB (MCP) |
| **Context Providers** | Context7 |
| **Built-in** | Filesystem, Git, GitHub, Fetch, Memory |

---

## Principles

| Principle | Implementation |
|---|---|
| **Precise** | Explicit capability declarations; deterministic routing |
| **Explicit** | Declarative YAML configuration. Zero implicit behavior. |
| **Rigorous** | Fail-closed at every enforcement point. No silent fallbacks. |
| **Principled** | All governance policies are auditable. No ad-hoc exceptions. |

---

## Roadmap

### Phase 1 — AI Workstation Core Architecture
*Current release — v0.1.0*

Complete architecture specification with:
- System classification and directory structure
- MCP Gateway design
- Tool Registry design
- Capability Routing design
- Dynamic Tool Injection design
- Session Lifecycle design
- Security Boundaries
- Multi-Project Isolation Model
- ChromaDB Namespace Strategy
- Runtime Governance Model

### Phase 2 — MCP Gateway
*Next*

Implementation of the MCP Gateway:
- Unix socket listener with MCP protocol
- Authentication middleware (Unix peer credentials, mTLS, tokens)
- Rate limiting
- Request routing to tool backends
- Plugin system for custom backends
- Health checking and heartbeat monitoring
- Graceful shutdown and connection draining

### Phase 3 — ChromaDB Integration

ChromaDB vector store integration:
- ChromaDB MCP server implementation
- Namespace management per project
- Collection lifecycle automation
- Embedding pipeline (session archival, decision logging)
- Namespace-enforcing gateway middleware
- Vector search routing

### Phase 4 — Dispatcher Runtime

Session lifecycle engine:
- Session state machine (Pending → Active → Suspend → Close → Archived)
- Session context management
- Dynamic tool injection based on session context
- Tool teardown policies (session-scoped, project-scoped, idle-timeout)
- Session heartbeat and orphan detection
- Audit trail integration

### Phase 5 — LangGraph Runtime

LangGraph workflow engine integration:
- LangGraph MCP bridge plugin
- Workflow definition and execution
- State persistence across graph steps
- Tool access via gateway from within graph nodes
- Human-in-the-loop interrupt support
- Workflow observability and tracing

### Phase 6 — CrewAI Integration

CrewAI multi-agent integration:
- CrewAI MCP bridge plugin
- Role-based agent definitions
- Task delegation and execution
- Crew-to-gateway tool access
- Multi-crew coordination
- Crew execution observability

### Phase 7 — Observability and Governance

Production observability and governance tooling:
- OpenTelemetry instrumentation across all components
- Structured logging (JSON) pipeline
- Metrics collection (request rates, latencies, error rates)
- Governance dashboard for policy visualization
- Audit trail query and reporting
- Alert rules for governance violations
- Cost tracking per session and per project

---

## Quick Start

```bash
# Verify the workstation platform structure
ls -la .ai/

# Read the architecture specification
cat .ai/ARCHITECTURE.md

# Onboard a new project
# 1. Create projects/<name>/
# 2. Add projects/<name>/.ai.yaml (see ARCHITECTURE.md §9.2)
# 3. Add governance policy at .ai/governance/policies/<name>.yaml
# 4. Add routing policy at .ai/routing/policies/<name>.yaml
```

---

## Governance

All changes to the AI Workstation platform require an ADR (Architectural Decision Record). See:
- `.ai/GOVERNANCE.md` — Platform constitution and laws
- `.ai/ADR_LOG.md` — Decision log with all ADRs
- `.ai/ARCHITECTURE.md` — Full architecture specification

---

## License

Apache 2.0 — see [LICENSE](LICENSE).
