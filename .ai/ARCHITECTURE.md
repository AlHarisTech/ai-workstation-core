# AI Engineering Workstation — Architecture

> **Platform architecture.** This document defines the structure, boundaries, and governance of the AI Engineering Workstation — a project-agnostic platform for multi-project AI-assisted software engineering.
>
> **Scope:** All AI infrastructure lives under `.ai/`. No project source code resides here. Projects consume AI capabilities through well-defined interfaces.
>
> **Status:** Baseline. Subject to ADR-driven evolution.
>
> **Principles:** PERP (Precise, Explicit, Rigorous, Principled) | Minimal-Diff | Fail-Closed | Evidence-First

---

## 1. System Classification

The AI Engineering Workstation is a **Project-Agnostic AI Infrastructure Platform** — NOT a project, NOT an application, NOT a framework.

| Classification | Meaning |
|---|---|
| **Platform** | Hosts and orchestrates AI capabilities consumed by multiple projects |
| **Project-Agnostic** | Zero assumptions about any project's language, framework, or domain |
| **Infrastructure** | Foundational layer — stable, minimal, invisible to projects |
| **Governed** | All changes require ADR. No silent deviation. |

---

## 2. Directory Structure

```
.ai/                                          # AI Workstation root (isolated from projects)
├── ARCHITECTURE.md                           # This document
├── ADR_LOG.md                                # Architectural Decision Log
├── GOVERNANCE.md                             # Platform governance constitution
│
├── gateway/                                  # MCP Gateway — central routing layer
│   ├── gateway.yaml                          # Gateway configuration
│   ├── plugins/                              # Gateway plugin definitions
│   └── certs/                                # TLS/mTLS certificates
│
├── registry/                                 # Tool Registry — inventory of all tools
│   ├── registry.yaml                         # Registry index
│   ├── tools/                                # Tool definitions (one YAML per tool)
│   │   ├── opencode.yaml
│   │   ├── mcp-servers.yaml
│   │   ├── context7.yaml
│   │   ├── filesystem.yaml
│   │   ├── git.yaml
│   │   ├── github.yaml
│   │   ├── fetch.yaml
│   │   ├── memory.yaml
│   │   ├── postgresql.yaml
│   │   ├── chromadb.yaml
│   │   ├── langgraph.yaml
│   │   └── crewai.yaml
│   └── schemas/                              # JSON Schema for tool definitions
│
├── routing/                                  # Capability Routing — intent → tool mapping
│   ├── rules/                                # Routing rules (per capability domain)
│   ├── policies/                             # Access policies (per project)
│   └── cache/                                # Route resolution cache
│
├── sessions/                                 # Session Lifecycle — ephemeral per-project context
│   ├── lifecycle.yaml                        # Session state machine definition
│   └── hooks/                                # Session lifecycle hooks
│
├── governance/                               # Runtime Governance Model
│   ├── policies/                             # Governance policies (per project)
│   ├── enforcement/                          # Enforcement plugin definitions
│   ├── audit/                                # Audit trail storage config
│   └── constraints/                          # Resource constraints (per project)
│
├── memory/                                   # Memory subsystem
│   ├── chromadb/
│   │   ├── config.yaml                       # ChromaDB connection and namespace config
│   │   └── migrations/                       # Collection schema migrations
│   └── filesystem/
│       └── stores/                           # File-backed memory stores
│
├── context/                                  # Context7 integration
│   └── config.yaml                           # Context7 connection config
│
├── agents/                                   # Agent definitions
│   ├── projects/                             # Project-specific agent overrides
│   └── templates/                            # Agent definition templates
│
└── config/                                   # Platform-wide configuration
    ├── platform.yaml                         # Platform settings
    ├── logging.yaml                          # Structured logging configuration
    └── secrets/                              # Encrypted secrets (keys, tokens)
```

**Rules:**
1. `.ai/` is the SINGLE authoritative location for all AI infrastructure.
2. No project directory contains AI infrastructure configuration. Projects configure consumption only.
3. `.ai/` is a hidden directory — invisible to project tooling by default.

---

## 3. MCP Gateway Design

### 3.1 Purpose

The MCP (Model Context Protocol) Gateway is the sole entry point for all AI-to-tool communication. Every tool invocation, regardless of origin (OpenCode agent, LangGraph workflow, CrewAI crew), passes through the gateway.

### 3.2 Architecture

```
                     ┌─────────────────────────────────┐
                     │       AI Orchestrators           │
                     │  (OpenCode / LangGraph / CrewAI) │
                     └──────────────┬──────────────────┘
                                    │ MCP Protocol
                                    ▼
                     ┌─────────────────────────────────┐
                     │         MCP Gateway              │
                     │                                   │
                     │  ┌─────────┐  ┌───────────────┐  │
                     │  │ Auth N   │  │ Rate Limiter   │  │
                     │  │ g        │  │                │  │
                     │  └────┬─────┘  └───────┬───────┘  │
                     │       │                 │          │
                     │  ┌────▼─────────────────▼───────┐  │
                     │  │       Router                  │  │
                     │  │  (Tool Registry Lookup)       │  │
                     │  └────┬─────────────────┬───────┘  │
                     │       │                 │          │
                     │  ┌────▼────┐    ┌───────▼──────┐  │
                     │  │Session  │    │ Governance    │  │
                     │  │Context   │    │ Enforcer      │  │
                     │  └────┬────┘    └───────┬──────┘  │
                     │       │                 │          │
                     └───────┼─────────────────┼──────────┘
                             │                 │
              ┌──────────────┼─────────────────┼──────────────┐
              │              │                 │              │
              │    ┌─────────▼───────┐  ┌──────▼─────────┐   │
              │    │  MCP Backend    │  │  MCP Backend   │   │
              │    │ (PostgreSQL)    │  │  (ChromaDB)    │   │
              │    └─────────────────┘  └────────────────┘   │
              │                                               │
              │         ... more MCP backends ...            │
              └───────────────────────────────────────────────┘
```

### 3.3 Connection Types

| Connection | Protocol | Use Case |
|---|---|---|
| **MCP over stdio** | stdio (JSON-RPC) | Local subprocess tools (OpenCode, Git, filesystem) |
| **MCP over SSE** | HTTP SSE | Remote MCP servers (PostgreSQL, ChromaDB, Context7) |
| **MCP over WebSocket** | WSS | Persistent bidirectional (realtime memory, streaming) |
| **Custom bridge** | Plugin-defined | LangGraph/CrewAI integration adapters |

### 3.4 Gateway Pipeline

Every request passes through this pipeline in order:

```
Authenticate → Authorize → Rate-Limit → Route → Inject Session → Enforce Governance → Execute → Audit
```

| Stage | Responsibility | Fail-Closed Behavior |
|---|---|---|
| **Authenticate** | Verify caller identity (mTLS, token, or Unix socket peer credential) | Reject with 401 |
| **Authorize** | Check caller has permission for the requested tool on the target project | Reject with 403 |
| **Rate-Limit** | Enforce per-project and per-tool rate limits | Queue or reject with 429 |
| **Route** | Look up tool in registry, resolve backend endpoint | Reject with 404 (tool unknown) |
| **Inject Session** | Attach session context (project ID, trace ID, user ID) to request | Reject if session invalid |
| **Enforce Governance** | Check request against active governance policies | Reject with 403 (policy violation) |
| **Execute** | Forward to backend, collect response | Propagate backend error |
| **Audit** | Log request/response/error to audit trail | Log failure; do not block response |

### 3.5 Gateway Configuration

File: `.ai/gateway/gateway.yaml`

```yaml
gateway:
  version: "1.0"
  listen:
    - type: unix
      path: /tmp/opencode/gateway.sock
    - type: tcp
      address: "127.0.0.1:8234"
      tls: true

  authentication:
    default: unix-peer
    methods:
      unix-peer:
        enabled: true
      mcp-token:
        enabled: true
        token_path: ".ai/config/secrets/gateway-tokens"

  rate_limiting:
    default:
      requests_per_second: 100
      burst: 50
    per_project:
      enabled: true

  backends:
    postgresql:
      type: mcp-sse
      url: "http://localhost:8235"
      timeout: 30s
    chromadb:
      type: mcp-sse
      url: "http://localhost:8236"
      timeout: 30s
    filesystem:
      type: mcp-stdio
      command: ["npx", "@modelcontextprotocol/server-filesystem"]
      workspace_root: "/home/asem/workspace"
```

### 3.6 Risk

| Risk | Mitigation |
|---|---|
| Gateway becomes single point of failure | Unix socket + localhost TCP only; no network exposure. Stateless — restartable. |
| Plugin loading creates attack surface | All plugins verified against schema. Cryptographic signature verification optional. |
| stdio subprocess lifecycle leaks | Timeout + SIGKILL on deadline. Subprocess group isolation. |

---

## 4. Tool Registry Design

### 4.1 Purpose

The Tool Registry is the authoritative inventory of every tool available to the AI Workstation. It defines each tool's interface, capabilities, security requirements, and routing destination.

### 4.2 Registry Schema

File: `.ai/registry/registry.yaml`

```yaml
registry:
  version: "1.0"
  tools:
    - id: "opencode"
      name: "OpenCode"
      type: "orchestrator"
      version: "latest"
      description: "Autonomous coding agent framework"
      entrypoint: "opencode"
      protocol: "mcp-stdio"
      capabilities:
        - "code-generation"
        - "code-analysis"
        - "file-edit"
        - "test-execution"
      governance:
        require_session: true
        audit_level: "full"
      isolation:
        namespace: "opencode"
        workspace: true

    - id: "context7"
      name: "Context7"
      type: "context-provider"
      version: "latest"
      description: "Context management and retrieval for AI agents"
      protocol: "mcp-sse"
      endpoint: "http://localhost:8237"
      capabilities:
        - "context-retrieval"
        - "context-storage"
      governance:
        require_session: true
        audit_level: "summary"

    - id: "postgresql"
      name: "PostgreSQL MCP Server"
      type: "data-source"
      version: "latest"
      description: "PostgreSQL database access via MCP"
      protocol: "mcp-sse"
      endpoint: "http://localhost:8235"
      capabilities:
        - "query-execution"
        - "schema-inspection"
        - "migration-apply"
      governance:
        require_session: true
        audit_level: "full"
      security:
        read_only_projects: true
        schema_scope: true

    - id: "chromadb"
      name: "ChromaDB MCP Server"
      type: "vector-store"
      version: "latest"
      description: "Vector database for semantic memory and retrieval"
      protocol: "mcp-sse"
      endpoint: "http://localhost:8236"
      capabilities:
        - "vector-search"
        - "document-embedding"
        - "collection-management"
      governance:
        require_session: true
        audit_level: "full"
      isolation:
        namespace_strategy: "per-project"  # See ChromaDB Namespace Strategy

    - id: "langgraph"
      name: "LangGraph Agent"
      type: "agent-framework"
      version: "latest"
      description: "LangGraph workflow orchestration"
      protocol: "custom-bridge"
      bridge_type: "langgraph"
      capabilities:
        - "workflow-orchestration"
        - "stateful-agent-execution"
        - "multi-step-planning"
      governance:
        require_session: true
        audit_level: "full"

    - id: "crewai"
      name: "CrewAI Agent"
      type: "agent-framework"
      version: "latest"
      description: "CrewAI multi-agent collaboration"
      protocol: "custom-bridge"
      bridge_type: "crewai"
      capabilities:
        - "multi-agent-collaboration"
        - "role-based-execution"
        - "task-delegation"
      governance:
        require_session: true
        audit_level: "full"

    - id: "filesystem"
      name: "Filesystem"
      type: "built-in"
      version: "1.0"
      description: "Local filesystem read/write operations"
      protocol: "mcp-stdio"
      command: ["npx", "@modelcontextprotocol/server-filesystem"]
      capabilities:
        - "file-read"
        - "file-write"
        - "file-search"
        - "directory-list"
      governance:
        require_session: false
        audit_level: "summary"

    - id: "git"
      name: "Git"
      type: "built-in"
      version: "1.0"
      description: "Git operations (commit, branch, diff, log)"
      protocol: "mcp-stdio"
      command: ["uvx", "mcp-git"]
      capabilities:
        - "status"
        - "diff"
        - "log"
        - "commit"
        - "branch"
        - "push"
        - "pull"
        - "merge"
      governance:
        require_session: true
        audit_level: "full"

    - id: "github"
      name: "GitHub"
      type: "built-in"
      version: "1.0"
      description: "GitHub API operations (PRs, issues, repos)"
      protocol: "mcp-stdio"
      command: ["npx", "@modelcontextprotocol/server-github"]
      capabilities:
        - "pull-requests"
        - "issues"
        - "repositories"
        - "workflows"
      governance:
        require_session: true
        audit_level: "full"

    - id: "fetch"
      name: "Fetch"
      type: "built-in"
      version: "1.0"
      description: "HTTP fetch for web content"
      protocol: "mcp-stdio"
      command: ["npx", "@modelcontextprotocol/server-fetch"]
      capabilities:
        - "http-get"
        - "http-post"
      governance:
        require_session: false
        audit_level: "summary"

    - id: "memory"
      name: "Memory"
      type: "built-in"
      version: "1.0"
      description: "Persistent memory across sessions"
      protocol: "mcp-stdio"
      command: ["npx", "@modelcontextprotocol/server-memory"]
      capabilities:
        - "store"
        - "retrieve"
        - "search"
      governance:
        require_session: true
        audit_level: "summary"
```

### 4.3 Tool Categories

| Category | Examples | Lifecycle |
|---|---|---|
| **Built-in** | filesystem, git, github, fetch, memory | Shipped with workstation, always available |
| **MCP Servers** | postgresql, chromadb | Managed processes, start/stop on demand |
| **Agent Frameworks** | langgraph, crewai | Spawned per workflow execution |
| **Orchestrators** | opencode | Long-running agent sessions |
| **Context Providers** | context7 | Background service, always available |

### 4.4 Tool Registration Protocol

```
1. Tool process starts (stdio/SSE/WebSocket) and announces via MCP initialize
2. Gateway validates tool announcement against registry schema
3. Gateway registers tool as available
4. Tool state tracked in registry (available, busy, degraded, offline)
5. Heartbeat monitoring — tool marked offline after N missed pings
```

### 4.5 Risk

| Risk | Mitigation |
|---|---|
| Registry drift (config vs reality) | Periodic health-check reconciliation; alert on mismatch |
| Tool version incompatibility | Semantic version constraints in registry; CI validates contract |
| Registry grows stale | `registry validate` command compares config against actual tool announcements |

---

## 5. Capability Routing Design

### 5.1 Purpose

Map user/agent intent to the correct tool(s) based on capability declarations, project context, and governance policy.

### 5.2 Routing Model

```
                     ┌─────────────────────────┐
                     │   Natural Language /     │
                     │   Structured Intent      │
                     └───────────┬─────────────┘
                                 │
                                 ▼
                     ┌─────────────────────────┐
                     │   Intent Parser          │
                     │   (extract capability)   │
                     └───────────┬─────────────┘
                                 │
                                 ▼
                     ┌─────────────────────────┐
                     │   Capability Matcher     │
                     │   tool.has(capability)   │
                     │   project.allows(tool)   │
                     └───────────┬─────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │                         │
                    ▼                         ▼
           ┌─────────────────┐    ┌──────────────────────┐
           │ Single Tool     │    │ Multi-Tool Workflow  │
           │ (direct route)  │    │ (orchestration plan) │
           └─────────────────┘    └──────────────────────┘
```

### 5.3 Routing Rules

File: `.ai/routing/rules/`

Each rule file defines a capability domain.

```yaml
# .ai/routing/rules/data-access.yaml
domain: "data-access"
rules:
  - capability: "query-execution"
    preferred_tool: "postgresql"
    fallback: []
  - capability: "vector-search"
    preferred_tool: "chromadb"
    fallback: []
  - capability: "schema-inspection"
    preferred_tool: "postgresql"
    fallback: []
```

```yaml
# .ai/routing/rules/code-operations.yaml
domain: "code-operations"
rules:
  - capability: "code-generation"
    preferred_tool: "opencode"
    fallback: ["langgraph"]
  - capability: "file-edit"
    preferred_tool: "opencode"
    fallback: ["filesystem"]
  - capability: "code-analysis"
    preferred_tool: "opencode"
    fallback: ["langgraph"]
```

### 5.4 Resolution Algorithm

```
resolve(capability, project_id, context):
  1. Load routing rules for capability domain
  2. Filter tools that declare the capability
  3. Filter tools allowed by project governance policy
  4. Sort by preference (preferred > fallback > available)
  5. Filter by current availability (heartbeat OK, not busy)
  6. Return first match, or fail-closed if no match

If no match: return RoutingError rather than silently falling back to unsafe tool
```

### 5.5 Access Policies

File: `.ai/routing/policies/`

Each project gets an access policy file that declares which tools/capabilities are permitted.

```yaml
# .ai/routing/policies/easyfit-pro.yaml
project: "easyfit-pro"
allowed_tools:
  - "opencode"
  - "filesystem"
  - "git"
  - "github"
  - "fetch"
  - "memory"
  - "postgresql"       # via MCP
  - "chromadb"         # via MCP
  - "context7"
  - "langgraph"
  - "crewai"

denied_tools: []
capability_overrides:
  postgresql:
    - "query-execution": read_only
    - "migration-apply": denied

resource_limits:
  max_concurrent_sessions: 3
  max_tools_per_session: 10
```

### 5.6 Risk

| Risk | Mitigation |
|---|---|
| Overly broad routing matches wrong tool | Explicit capability declarations; fail-closed on ambiguity |
| Policy drift (project allowed tools != actual usage) | Governance audit compares policy vs session logs |
| Routing cache staleness | TTL-based invalidation; force-reload on tool registry change |

---

## 6. Dynamic Tool Injection Design

### 6.1 Purpose

Enable tools to be loaded, configured, and injected into session contexts at runtime — without restarting the gateway or modifying project code.

### 6.2 Injection Model

```yaml
# .ai/registry/tools/postgresql.yaml  (extended definition)
tool:
  id: "postgresql"
  dynamic_injection:
    enabled: true
    method: "container"
    container:
      image: "mcp/postgres:latest"
      env_from_project:
        - key: "DATABASE_URL"
          project_config_path: "supabase/.temp/project-ref"
      port: 8235
      health_check:
        path: "/health"
        interval: 10s
        timeout: 3s
```

### 6.3 Injection Lifecycle

```
1. SESSION_CREATED event triggers injection evaluation
2. Gateway checks: does this project's policy allow tool X?
3. Gateway checks: is tool X already running for this project?
4. If not running → start tool container/process
5. Wait for health check OK
6. Register tool as available in gateway's routing table
7. Attach tool handle to session context
8. On SESSION_CLOSED → evaluate teardown (keep-alive or shutdown)
```

### 6.4 Injection Methods

| Method | When to Use | Example |
|---|---|---|
| **Container** | Network services, need isolation | PostgreSQL MCP, ChromaDB MCP |
| **Subprocess** | CLI tools, no network needed | Filesystem MCP, Git MCP |
| **Lazy stdio** | One-shot, on-demand | Fetch MCP |
| **Persistent daemon** | Always-on services | Context7, Gateway itself |

### 6.5 Teardown Policies

| Policy | Behavior | Use Case |
|---|---|---|
| `session-scoped` | Destroy when session ends | Ephemeral tool instances |
| `project-scoped` | Keep alive while any session for project exists | Database connections |
| `persistent` | Always running, never auto-destroy | Gateway, context7 |
| `idle-timeout` | Destroy after N minutes of inactivity | Low-use tools |

### 6.6 Risk

| Risk | Mitigation |
|---|---|
| Container startup latency delays first request | Pre-warm for frequently used tools; health check before routing |
| Zombie processes from failed teardown | Subprocess group isolation; watchdog cleanup goroutine |
| Resource exhaustion from many injected tools | Per-project resource quotas in governance policies |
| Injection race on concurrent session creation | Mutex per tool+project pair |

---

## 7. Session Lifecycle Design

### 7.1 Purpose

Manage the ephemeral context within which all AI operations occur. A session is the atomic unit of work — it binds identity, project, tool instances, and governance scope.

### 7.2 State Machine

```
                  ┌──────────┐
                  │  PENDING  │
                  └────┬─────┘
                       │ create
                       ▼
                  ┌──────────┐
         ┌───────►│ ACTIVE   │◄────────┐
         │        └────┬─────┘         │
         │             │               │
         │      ┌──────┴──────┐        │
         │      │             │        │
         │      ▼             ▼        │
         │ ┌─────────┐  ┌──────────┐   │
         │ │ SUSPEND │  │ DRAINING │   │
         │ └────┬────┘  └────┬─────┘   │
         │      │             │         │
         │      └──────┬──────┘         │
         │             │ resume         │
         │             ▼                │
         │        ┌──────────┐          │
         └────────┤ ACTIVE   │──────────┘
                  └────┬─────┘  (re-activate)
                       │ close
                       ▼
                  ┌──────────┐
                  │ CLOSED   │
                  └──────────┘
                       │ archive
                       ▼
                  ┌──────────┐
                  │ ARCHIVED │
                  └──────────┘
```

### 7.3 Lifecycle Events

| Event | Trigger | Side Effects |
|---|---|---|
| `create` | Agent/workflow starts | Generate session ID, resolve project context, load governance policies, inject tools |
| `activate` | First tool invocation | Tool injection, routing table setup, session context broadcast |
| `suspend` | Inactivity timeout | Tear down session-scoped tools, persist session state |
| `resume` | New request for suspended session | Re-inject session-scoped tools, restore context |
| `draining` | Close requested, in-flight ops pending | Graceful shutdown — wait for pending operations (max 30s) |
| `close` | All operations complete | Finalize audit record, flush memory buffers, cleanup all tools |
| `archive` | Post-close persistence | Compress and store session log for retrieval |

### 7.4 Session Context Structure

```yaml
session:
  id: "ses_abc123"
  project_id: "easyfit-pro"
  user_id: "user_default"          # Local user (not app user)
  created_at: "2026-06-10T10:00:00Z"
  expires_at: "2026-06-10T11:00:00Z"

  context:
    workspace_root: "/home/asem/workspace"
    project_path: "/home/asem/workspace/projects/easyfit-pro"
    memory_namespace: "project:easyfit-pro"
    chroma_collection: "project:easyfit-pro"

  tools:
    active:
      - id: "filesystem"
        connection: "stdio"
        pid: 12345
      - id: "git"
        connection: "stdio"
        pid: 12346
    injected:
      - id: "postgresql"
        connection: "sse"
        endpoint: "http://localhost:8235"

  governance:
    policies_applied:
      - "data-access/read-only"
    audit_level: "full"

  state:
    status: "active"
    tool_count: 3
    total_requests: 47
    errors: 0
```

### 7.5 Session Isolation

| Dimension | Isolation Mechanism |
|---|---|
| **Process** | Each injected tool subprocess runs in the session's process group |
| **Filesystem** | Tool `workspace_root` is restricted to project directory |
| **Network** | MCP backends scoped by namespace/project |
| **Memory** | ChromaDB collection per project namespace |
| **Audit** | All session records tagged with project_id |

### 7.6 Risk

| Risk | Mitigation |
|---|---|
| Session context leaks between sessions | Strict `project_id` scoping on all context keys; runtime assertion on read |
| Orphaned sessions from agent crash | Heartbeat TTL — session auto-closes after N missed heartbeats |
| Session ID collision | UUIDv7 with nanosecond timestamp + random suffix |
| State explosion from long-running sessions | Implicit session rotation after N requests or M hours |

---

## 8. Security Boundaries

### 8.1 Threat Model

| Threat | Vector | Impact |
|---|---|---|
| Cross-project data access | Tool operating on wrong project directory | Data contamination |
| Unauthorized tool execution | Agent invoking tool without permission | Governance bypass |
| Session hijacking | Attacker reusing session token | Identity spoofing |
| Privilege escalation | Tool escaping its sandbox | Host compromise |
| Audit tampering | Attacker modifying audit logs | Undetected breach |

### 8.2 Boundary Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                     HOST OS (Linux)                            │
│                                                                │
│  ┌────────────────────────────────────────────────────────┐   │
│  │              AI Workstation Platform                    │   │
│  │  ┌──────────┐  ┌──────────┐  ┌────────────────────┐   │   │
│  │  │ Gateway  │  │ Registry │  │ Audit Trail        │   │   │
│  │  │ (unix     │  │ (config  │  │ (append-only log)  │   │   │
│  │  │  socket)  │  │  only)   │  │                    │   │   │
│  │  └────┬─────┘  └──────────┘  └────────────────────┘   │   │
│  │       │                                                │   │
│  └───────┼────────────────────────────────────────────────┘   │
│          │                                                    │
│  ┌───────┴────────────────────────────────────────────────┐   │
│  │               PROJECT SANDBOX                          │   │
│  │                                                        │   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────────┐   │   │
│  │  │ Tool Proc  │  │ Tool Proc  │  │ Agent Proc     │   │   │
│  │  │ (filesys)  │  │ (git)      │  │ (OpenCode)    │   │   │
│  │  │ PID: 12345 │  │ PID: 12346 │  │ PID: 12347    │   │   │
│  │  └────────────┘  └────────────┘  └────────────────┘   │   │
│  │                                                        │   │
│  │  Workspace: /home/asem/workspace/projects/easyfit-pro  │   │
│  │  Network:   loopback only                              │   │
│  └────────────────────────────────────────────────────────┘   │
│                                                                │
│  ┌────────────────────────────────────────────────────────┐   │
│  │               ISOLATED BACKENDS                        │   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────────┐   │   │
│  │  │ PostgreSQL │  │ ChromaDB   │  │ Context7       │   │   │
│  │  │ Port 8235  │  │ Port 8236  │  │ Port 8237      │   │   │
│  │  └────────────┘  └────────────┘  └────────────────┘   │   │
│  └────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────┘
```

### 8.3 Enforcement Layers

| Layer | Mechanism | Scope |
|---|---|---|
| **OS-level** | Unix user isolation, process groups, cgroups | All platform processes |
| **Container** | Docker/Podman containers for network services | PostgreSQL, ChromaDB MCP servers |
| **Filesystem** | `workspace_root` restriction per tool invocation | Filesystem MCP, Git MCP |
| **Gateway** | Authentication, authorization, rate limiting | All MCP requests |
| **Protocol** | Tool capability declarations restrict surface area | All MCP tools |
| **Audit** | Append-only log of all operations | All tool invocations |

### 8.4 Authentication Methods

| Method | Use Case | Security Level |
|---|---|---|
| **Unix peer credentials** | Local tools (stdio MCP) — kernel-authenticated PID → UID | High |
| **mTLS** | SSE/WebSocket MCP connections | High |
| **Bearer token** | Context7, external tool access | Medium (token rotation) |
| **Anonymous** | Read-only fetch, public context (audited) | Low |

### 8.5 Secret Management

- Secrets stored encrypted in `.ai/config/secrets/`
- Encryption key derived from host identity (TPM if available, otherwise file-based key)
- No secrets in environment variables passed to subprocesses — injected via file descriptor or tmpfs
- Database connection strings fetched by MCP servers from encrypted store at startup

### 8.6 Risk

| Risk | Mitigation |
|---|---|
| Local privilege escalation via stdio tool | Unix peer credential authentication; tool runs as non-root user |
| Cross-project path traversal | Workspace root restriction enforced at gateway level, not tool level |
| Secret leak via process environment | File descriptor / tmpfs secret injection, not environment variables |
| Audit trail tampering | Append-only log file; periodic hash chain integrity check |

---

## 9. Multi-Project Isolation Model

### 9.1 Isolation Dimensions

Projects are isolated along 6 orthogonal dimensions:

| Dimension | Isolation Strategy | Failure Mode Prevented |
|---|---|---|
| **Filesystem** | Each project under `projects/<name>/`. Tools restricted to project directory. | Cross-project file contamination |
| **Database** | Separate Supabase projects per application. MCP connection uses per-project credentials. | Cross-project data leakage |
| **Vector Store** | ChromaDB namespaced per project (see §10). | Cross-project memory contamination |
| **Session** | Session context scoped to single project. Gateway rejects cross-project tool calls. | Cross-project operation leakage |
| **Process** | Tool subprocesses isolated by session. Session groups killed on close. | Zombie cross-project processes |
| **Configuration** | Governance policies per project. Routing policies per project. | Policy bypass via another project |

### 9.2 Project Configuration

Each project declares its AI needs in a minimal configuration file at `projects/<name>/.ai.yaml`:

```yaml
# projects/easyfit-pro/.ai.yaml
project:
  name: "easyfit-pro"
  type: "application"
  language: "dart"

  ai:
    enabled_tools:
      - "opencode"
      - "filesystem"
      - "git"
      - "github"
      - "fetch"
      - "memory"
      - "postgresql"
      - "chromadb"
      - "context7"
      - "langgraph"
      - "crewai"

    supabase:
      project_ref: "ref_abc123"

    memory:
      enabled: true
      namespaces:
        - "sessions"
        - "decisions"
        - "architecture"
```

The project `.ai.yaml` is the ONLY AI-related file inside a project directory. All runtime infrastructure lives in `.ai/` at the workspace root.

### 9.3 Onboarding New Project

```
1. Create projects/<name>/ directory
2. Create projects/<name>/.ai.yaml with tool selections
3. Add governance policy at .ai/governance/policies/<name>.yaml
4. Add routing policy at .ai/routing/policies/<name>.yaml
5. Gateway auto-detects new project via filesystem watch
```

### 9.4 Cross-Project Operations

Cross-project operations are **denied by default**. Explicit policy override required:

```yaml
# .ai/governance/policies/exception-cross-project-ref.yaml
exception:
  id: "XC-001"
  description: "Allow cross-project reference for shared library"
  source_project: "project-a"
  target_project: "project-b"
  allowed_tools: ["filesystem"]
  paths:
    - "projects/project-b/packages/shared/**"
  audit: "full"
  expires: "2027-01-01"
```

### 9.5 Risk

| Risk | Mitigation |
|---|---|
| `.ai.yaml` misconfiguration causes wrong tool set | Gateway validates `.ai.yaml` against schema; rejects invalid configs |
| Shared library cross-project reads create side effects | Read-only access; full audit trail of all cross-project operations |
| Project count grows, management overhead scales | CLI tool `ai project init` auto-generates configs and policies |

---

## 10. ChromaDB Namespace Strategy

### 10.1 Namespace Model

ChromaDB collections are organized in a hierarchical namespace structure that mirrors the workspace's project isolation model.

```
Namespace Tree:

chromadb/
├── global/                          # Platform-wide collections
│   ├── tool-registry                # Tool capability embeddings
│   ├── routing-cache                # Cached routing decisions
│   └── governance-policies          # Governance policy embeddings
│
├── project:<project-id>/            # Per-project collections
│   ├── sessions                     # Session history embeddings
│   ├── decisions                    # Architectural decision embeddings
│   ├── architecture                 # Architecture document embeddings
│   ├── code-context                 # Code understanding embeddings
│   ├── git-history                  # Commit message + diff embeddings
│   ├── agent-memory                 # Agent persistent memory
│   └── custom                       # Project-defined collections
│
└── user:<user-id>/                  # (Future) Per-user collections
    └── preferences
```

### 10.2 Collection Schema

Each collection in the namespace follows a standard schema:

```yaml
collection:
  name: "project:easyfit-pro:sessions"
  metadata:
    namespace: "project:easyfit-pro"
    collection_type: "sessions"
    project_id: "easyfit-pro"
    version: 1
  embedding:
    model: "all-MiniLM-L6-v2"             # Lightweight, fast
    dimension: 384
    distance: "cosine"
  documents:
    - id: "session_trace"
      metadata:
        session_id: "ses_abc123"
        tool: "opencode"
        timestamp: "2026-06-10T10:00:00Z"
      content: "..."
```

### 10.3 Namespace Resolution

```
resolve_namespace(collection_type, project_id):
  if collection_type in GLOBAL_COLLECTIONS:
    return f"global:{collection_type}"
  else:
    return f"project:{project_id}:{collection_type}"
```

### 10.4 Access Boundaries

| Collection Type | Read Scope | Write Scope | Query Scope |
|---|---|---|---|
| `global:*` | All projects | Platform only | All projects |
| `project:*` | Owner project only | Owner project only | Owner project only, filtered at gateway |
| `user:*` | Owner user only | Owner user only | Owner user only |

### 10.5 Collection Lifecycle

```
project:created  →  create default collections for project
session:created   →  (optional) create session sub-collection
session:closed    →  batch-write session embeddings
project:archived  →  freeze collections (read-only)
project:deleted   →  destroy collections (with confirmation)
```

### 10.6 Risk

| Risk | Mitigation |
|---|---|
| Namespace collision (two projects with same name) | Use project_id (UUID/ref) not project name as namespace key |
| Collection proliferation (many small collections) | Merge threshold: sessions older than N days consolidated into monthly snapshot |
| Cross-namespace query by misconfigured agent | Gateway intercepts all ChromaDB requests; enforces namespace scope |
| Embedding model drift over time | Versioned collections; `v1`, `v2` suffixes |

---

## 11. Runtime Governance Model

### 11.1 Governance Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     GOVERNANCE SYSTEM                           │
│                                                                 │
│  ┌──────────────┐   ┌──────────────┐   ┌────────────────────┐  │
│  │ Policy Store  │   │ Enforcer     │   │ Audit Trail        │  │
│  │ (.ai/         │   │ (gateway     │   │ (append-only log)  │  │
│  │  governance/  │   │  middleware)  │   │                    │  │
│  │  policies/)   │   │              │   │                    │  │
│  └──────────────┘   └──────┬───────┘   └────────────────────┘  │
│                            │                                    │
│                   ┌────────┴────────┐                          │
│                   │ Violation       │                          │
│                   │ Handler         │                          │
│                   │ (fail-closed)   │                          │
│                   └─────────────────┘                          │
└─────────────────────────────────────────────────────────────────┘
```

### 11.2 Policy Categories

| Category | Scope | Examples |
|---|---|---|
| **Access** | Which tools/capabilities per project | `postgresql: read-only`, `fetch: denied` |
| **Resource** | CPU, memory, concurrency limits | `max_sessions: 3`, `max_memory_mb: 512` |
| **Data** | Data retention, namespace boundaries | `chroma_retention_days: 90`, `cross_project_read: denied` |
| **Audit** | What to log and at what verbosity | `audit_level: full` for data mutations |
| **Compliance** | Regulatory or organizational rules | (Future) SOC2, HIPAA collection scoping |
| **Operational** | Timeout, retry, circuit-breaker settings | `default_timeout: 30s`, `max_retries: 3` |

### 11.3 Policy Schema

Each policy is a YAML file in `.ai/governance/policies/<project-id>.yaml`:

```yaml
# .ai/governance/policies/easyfit-pro.yaml
governance:
  version: "1.0"
  project: "easyfit-pro"

  access:
    allowed_tools:
      - id: "postgresql"
        operations:
          query: "read_only"                    # read_only, read_write, admin
          schema: "read_only"
          migration: "denied"
      - id: "chromadb"
        operations:
          read: "allow"
          write: "allow"
          delete: "denied"
      - id: "filesystem"
        workspace_root: "/home/asem/workspace/projects/easyfit-pro"
        denied_paths:
          - ".ai/config/secrets/**"
          - ".git/**"

  resources:
    max_concurrent_sessions: 3
    max_tools_per_session: 12
    max_memory_per_session_mb: 2048
    default_timeout: 60s

  data:
    chroma_retention_days: 90
    session_log_retention_days: 365
    vector_store_namespace: "project:easyfit-pro"
    cross_project_read: false

  audit:
    level: "full"                              # none, summary, full
    log_requests: true
    log_responses: true
    log_errors: true
    exclude_health_checks: true

  enforcement:
    fail_closed: true                           # Violation → request denied
    violation_action: "log_and_block"           # log_only, log_and_block, alert

  violations:
    - pattern: "attempt to write to denied path"
      action: "log_and_block"
    - pattern: "query on non-owned collection"
      action: "log_and_block"
    - pattern: "migration without ADR"
      action: "block_with_message"
```

### 11.4 Enforcement Points

| Enforcement Point | What Is Checked | Fail-Closed Behavior |
|---|---|---|
| **Gateway authentication** | Caller identity | 401 Unauthorized |
| **Gateway authorization** | Permission for tool on target project | 403 Forbidden |
| **Gateway routing** | Tool registry lookup | 404 Tool Unknown |
| **Session creation** | Policy loaded, context valid | Session creation rejected |
| **Pre-execution** | Tool operation vs allowed operations | Request rejected before tool invocation |
| **Post-execution** | Response contains no policy violations | Response scrubbed or blocked |
| **Periodic audit** | Session logs vs policies | Alert generated |

### 11.5 Violation Severity

| Severity | Meaning | Response |
|---|---|---|
| **CRITICAL** | Security boundary violation, data leakage | Immediate block, alert, session termination |
| **HIGH** | Policy violation, unauthorized operation | Block request, log, alert |
| **MEDIUM** | Resource limit exceeded | Throttle, log |
| **LOW** | Audit compliance gap | Log, report |

### 11.6 Audit Trail

The audit trail is an append-only log. Each entry:

```yaml
audit_entry:
  id: "aud_20260610_001"
  timestamp: "2026-06-10T10:00:01.123Z"
  session_id: "ses_abc123"
  project_id: "easyfit-pro"
  user_id: "user_default"
  tool_id: "postgresql"
  operation: "query-execution"
  request:
    type: "query"
    target: "SELECT * FROM profiles WHERE id = $1"
    params_count: 1
  response:
    status: "success"
    rows_returned: 1
    duration_ms: 45
  governance:
    policies_applied: ["data-access/read-only"]
    violation: null
```

### 11.7 PERP Compliance

| PERP Principle | Implementation |
|---|---|
| **Precise** | Tool capability declarations are explicit; routing is deterministic |
| **Explicit** | All configuration is declarative YAML. Zero implicit behavior. |
| **Rigorous** | Fail-closed at every enforcement point. No silent fallbacks. |
| **Principled** | All governance policies are auditable. No ad-hoc exceptions. |

### 11.8 Risk

| Risk | Mitigation |
|---|---|
| Policy bypass through misconfigured gateway route | Gateway enforces policy at pre-execution (not just routing); redundant check |
| Policy explosion with many projects | Policy templates with per-project overrides; `ai policy validate` command |
| Circular governance dependency (policy to check policy access) | Governance system reads policies from local filesystem (not through gateway) |
| Policy drift from actual behavior | Scheduled governance audit compares session logs against policy expectations |

---

## 12. Integration Points

### 12.1 Supabase (PostgreSQL)

- Each project manages its own Supabase instance.
- MCP PostgreSQL server connects to the project's Supabase via connection string from `.ai/config/secrets/`.
- Gateway enforces read-only vs read-write per project policy.

### 12.2 ChromaDB

- Single ChromaDB instance with namespace isolation.
- Namespace derived from `project_id` for all project-owned collections.
- Gateway intercepts all ChromaDB MCP requests to enforce namespace scope.

### 12.3 OpenCode

- OpenCode runs as an agent subprocess within the project sandbox.
- Communicates with gateway via Unix socket at `.ai/gateway/gateway.sock`.
- Session lifecycle bound to OpenCode session.

### 12.4 LangGraph / CrewAI

- Custom bridge plugins in `.ai/gateway/plugins/`.
- LangGraph workflows spawn as subprocesses; communicate via MCP stdio/SSE.
- CrewAI crews managed through Python SDK bridge.

### 12.5 Context7

- Persistent daemon on localhost:8237.
- Context retrieval and storage through MCP SSE.
- Context scoped by `project_id` parameter.

---

## 13. MCP Gateway Execution Mapping (v0.4.0)

> **Runtime truth alignment for the adaptive runtime kernel.** Maps architecture to the queue-driven, multi-worker implementation.

### 13.1 Runtime-to-Architecture Mapping

| Architecture Concept | Runtime Implementation | File |
|---|---|---|
| Gateway Pipeline (7-stage) | `Pipeline` with dual STRICT/OPTIMIZED mode | `runtime/mcp_gateway/pipeline.py` |
| Execution Queue | `RequestQueue` — bounded FIFO with backpressure | `runtime/kernel/queue.py` |
| Shared Worker Pool | Persistent worker threads (configurable N) | `runtime/kernel/worker_pool.py` |
| Persistent State | `StateStore` — file-based JSON (sessions, traces, meta) | `runtime/kernel/state_store.py` |
| Tool Registry with Versioning | `ToolRegistry` with version validation | `runtime/tools/registry.py` |
| Governance Enforcement (Composable) | `PolicyEngine` with rule chaining + decision graph | `runtime/governance/policy_engine.py` |
| Tool Isolation | `execute_isolated()` with timeout containment | `runtime/executor/local_executor.py` |
| Request Context Graph | `RequestContext` with queue_wait, worker_id, latency_breakdown | `runtime/mcp_gateway/context.py` |
| Audit Logging | `StructuredLogger` — policy graph, queue timing, worker info | `runtime/auditlog/structured_logger.py` |

### 13.2 Kernel Architecture (v0.4.0)

```
stdin → parse JSON → RequestContext.create() → RequestQueue.put()
                                                  │
                  ┌───────────────────────────────┘
                  ▼
         ┌───────────────────────┐
         │   Worker Pool (N)      │
         │                        │
         │  wrk_000: queue.get()  │──→ Pipeline.process() ──→ result_list
         │  wrk_001: queue.get()  │──→ Pipeline.process() ──→ result_list
         │  wrk_002: queue.get()  │──→ Pipeline.process() ──→ result_list
         │  wrk_003: queue.get()  │──→ Pipeline.process() ──→ result_list
         └───────────────────────┘
                  │
                  ▼
         Result Collector Thread
                  │
                  ▼
         stdout (JSON responses)
```

### 13.3 Dual Pipeline Modes

| Mode | Stages | Use Case |
|---|---|---|
| `strict` | 7 stages (full) + audit_log on all paths | Default. Full governance, full audit. |
| `optimized` | 6 stages (no audit_log in pipeline) | Performance-sensitive. Audit still runs at end. |

Selectable per request via `"pipeline_mode": "optimized"` in the JSON request.

### 13.4 Latency Breakdown

Every response includes `execution.latency_breakdown`:
- `queue_wait_ms` — time in RequestQueue before worker pickup
- `routing_ms` — capability_routing stage duration
- `execution_ms` — tool execution stage duration
- `audit_ms` — audit_log stage duration
- `validation_ms` — pre_validation + post_validation total
- `total_ms` — sum of all stage timings

### 13.5 Persistent State

`.ai/state/` stores:
- `sessions/<session_id>.json` — session metadata
- `traces/<request_id>.json` — full execution trace
- `meta.json` — registry version, policy version, worker count

All writes are atomic (temp file → rename). State survives process restarts.

---

## 14. Expansion Points

| Area | Future Direction | Trigger |
|---|---|---|
| **Remote Gateway** | Gateway exposes TCP endpoint for remote agents | Need for CI/CD integration |
| **Federated Namespace** | Cross-platform ChromaDB namespace federation | Multi-workspace deployment |
| **Plugin System** | Custom MCP backend plugins | Third-party tool integration |
| **UI Dashboard** | Governance dashboard for policy/audit visualization | Operational visibility need |
| **Cost Tracking** | Per-session cost accounting | Resource chargeback requirement |
| **LLM Gateway** | Centralized LLM routing with key management | Multiple LLM provider usage |
| **Workflow Engine** | Visual workflow builder for multi-tool pipelines | Complex automation needs |
| **Compliance Framework** | SOC2/HIPAA/GDPR collection tagging | Regulatory requirements |
| **Secret Rotation** | Automated credential rotation | Security policy requirement |
| **Horizontal Scaling** | Gateway clustering, session distribution | High-throughput multi-project operation |
| **Observability Stack** | OpenTelemetry integration, metrics dashboard | Production monitoring requirement |
| **Agent SDK** | TypeScript/Python SDK for custom agent development | Developer tooling needs |

---

## 15. Key Tradeoffs

| Decision | Tradeoff | Rationale |
|---|---|---|
| **Unix socket gateway** | Not accessible from remote machines | No network attack surface. Local agents only. |
| **YAML-based configuration** | No dynamic reconfiguration without restart | Deterministic configuration. Auditability. |
| **Per-project ChromaDB namespaces** | No cross-project vector search | Strict isolation. Cross-project search can be added via gateway bridge. |
| **Session-scoped tool injection** | Startup latency per session | Isolation > startup time. Pre-warming mitigates. |
| **Fail-closed everywhere** | Higher operational friction on misconfiguration | Safer to deny than to leak. Misconfiguration is detectable. |
| **Project-local `.ai.yaml`** | One file per project in project tree | Minimal intrusion. Only config, no infrastructure. |

---

## 16. Architectural Decision Record Index

Decisions about this platform architecture are recorded in `.ai/ADR_LOG.md` following the same format as project-level ADRs.

| ADR | Title | Status |
|---|---|---|
| ADR-001 | AI Workstation Baseline Architecture | Proposed |
| ADR-002 | Unix Socket Gateway over TCP | Proposed |
| ADR-003 | Per-Project ChromaDB Namespaces | Proposed |
| ADR-004 | Session-Scoped Tool Injection | Proposed |

---

*This architecture document is the authoritative specification for Phase 1 of the AI Engineering Workstation. Implementation must not deviate from this specification without an ADR.*
