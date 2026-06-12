# Real MCP Integration Validation Plan

**Phase:** C0 — Real MCP Integration Validation  
**Status:** Planned  
**Version:** v3.1.1-stable  
**Architecture Constraint:** Execution Before Expansion — Evidence Before Evolution

---

## 1. Scope & Rationale

### What This Phase IS
- Real execution against every registered MCP server in the production environment
- Contract verification — does each server's actual behaviour match its code contract?
- Evidence collection — logs, metrics, traces, latency, memory, outcomes
- Gap identification — what breaks, what's slow, what's undocumented

### What This Phase IS NOT
- No new components, engines, or architectural expansion
- No Knowledge Graph, Long-Term Memory, Agent Network, AI Planner, Reasoning Layer, Federation, or Swarm
- No modifications to runtime source files — **zero diff permitted**
- ChromaDB is NOT a core runtime dependency — it is a backend provider behind the Memory MCP layer

### Why Now
The system has been validated against itself (unit tests, benchmarks, metrics). It has NOT been validated against:
- Real filesystem operations with permission boundaries
- Real git repositories with history and branches
- Real HTTP fetch with timeouts, 404s, 500s, large payloads
- Real GitHub API with rate limits and auth failures
- Real in-memory key-value store with persistence
- Real Context7 API or its local fallback

---

## 2. MCP Inventory

| # | Server | Type | Operations | Dependency | Priority |
|---|--------|------|-----------|-----------|----------|
| P1 | Filesystem | Local I/O | read, write, delete, list, search | Workspace path | **P1** |
| P2 | Git | Local exec | status, diff, log, branch, commit, push, tag | Git binary, workspace | **P2** |
| P3 | Fetch | HTTP | get, status, download | Network | **P3** |
| P4 | GitHub | HTTP API | repo, list_issues, create_issue, create_pr, create_release | GitHub token | **P4** |
| P5 | Memory | In-process | store, retrieve, delete, list | None | **P5** |
| P6 | Context7 | Local/Remote | query, store, resolve | API key (optional) | **P6** |

> ChromaDB and Postgres are backend providers, NOT core MCP dependencies.
> ChromaDB-backed Memory (P7) is deferred until in-process Memory passes all gates.
> Postgres SQL (P8) is deferred — no current dependency.

---

## 3. Contract Mapping

For each MCP server, define:
- **Input Contract**: expected payload fields and types
- **Output Contract**: return shape on success
- **Failure Contract**: error conditions and their error messages
- **Timeout Contract**: expected behaviour under latency

### 3.1 Filesystem Server

#### Input Contract

| Operation | Field | Type | Required | Notes |
|-----------|-------|------|----------|-------|
| `read` | `path` | string | yes | relative or absolute within workspace |
| `write` | `path` | string | yes | relative or absolute within workspace |
| `write` | `content` | string | yes | file content to write |
| `delete` | `path` | string | yes | file or directory to remove |
| `list` | `path` | string | no | defaults to workspace root |
| `search` | `pattern` | string | yes | glob pattern, joined to workspace |

#### Output Contract

```jsonc
// read
{"path": "file.go", "content": "...", "size": 1234, "mode": "-rw-r--r--"}

// write
{"path": "file.go", "written": true, "size": 42}

// delete
{"path": "old.go", "deleted": true}

// list
{"path": "", "files": [{"name": "main.go", "dir": false, "size": 100, "mode": "-rw-r--r--"}], "count": 1}

// search
{"matches": ["src/main.go"], "count": 1}
```

#### Failure Contract

| Condition | Error Contains | Recoverable |
|-----------|---------------|-------------|
| Empty path (where required) | `path is required` | yes |
| Path traversal | `path traversal denied` | no |
| Workspace path missing | `workspace path required` | yes |
| File not found | `read failed: no such file` | yes |
| Permission denied | `read failed: permission denied` | no |

#### Timeout Contract
All filesystem operations are synchronous local I/O. Expected latency: < 100ms for typical files, < 1s for large files.

---

### 3.2 Git Server

#### Input Contract

| Operation | Field | Type | Required | Notes |
|-----------|-------|------|----------|-------|
| `commit` | `message` | string | yes | commit message |
| `tag` | `tag` | string | yes | tag name |
| All | — | — | — | workspace path from context is required |

#### Output Contract

```jsonc
// status
{"output": " M src/main.go\n?? new.go"}

// diff
{"output": "diff --git a/main.go b/main.go\n..."}

// log
{"output": "abc1234 feat: add feature\ndef5678 fix: resolve bug"}

// branch
{"output": "* main\n  feature"}

// commit
{"output": "[main abc1234] feat: add feature"}

// push
{"output": "To github.com:user/repo.git\n   abc1234..def5678  main -> main"}

// tag
{"tag": "v1.0.0"}
```

#### Failure Contract

| Condition | Error Contains | Recoverable |
|-----------|---------------|-------------|
| No workspace path | `workspace path required` | yes |
| Not a git repository | `git status failed` + `fatal: not a git repository` | yes |
| No commit message | `commit message is required` | yes |
| No tag name | `tag name is required` | yes |
| Git binary not found | `exec: "git": executable file not found` | no |

#### Timeout Contract
Git operations are local `exec.Command` calls. No explicit timeout in the server — relies on OS process timeout. Expected latency: < 500ms for typical operations.

---

### 3.3 GitHub Server

#### Input Contract

| Operation | Field | Type | Required | Notes |
|-----------|-------|------|----------|-------|
| All | `owner` | string | yes | repo owner |
| All | `repo` | string | yes | repo name |
| `repo`/`read` | `path` | string | yes | file path in repo |
| `list_issues` | `state` | string | no | open, closed, all |
| `create_issue` | `title` | string | yes | — |
| `create_issue` | `body` | string | no | — |
| `create_pr` | `title` | string | yes | — |
| `create_pr` | `head` | string | yes | head branch |
| `create_pr` | `base` | string | yes | base branch |
| `create_release` | `tag` | string | yes | tag name |
| `create_release` | `name` | string | no | release name |
| Auth | — | — | — | Token via `ctx.TenantID` |

#### Output Contract

```jsonc
// repo/read
{"status": 200, "data": {"name": "main.go", "content": "base64..."}}

// list_issues
{"status": 200, "data": [{"number": 1, "title": "bug", "state": "open"}]}

// create_issue, create_pr, create_release
{"status": 201, "data": {"number": 2, "title": "new issue", "state": "open"}}
```

#### Failure Contract

| Condition | Error Contains | Recoverable |
|-----------|---------------|-------------|
| Missing owner/repo | `owner, repo, and path are required` | yes |
| No token | `GitHub token required` | yes |
| 401 Unauthorized | `github api error: 401` | no |
| 403 Rate limited | `github api error: 403` | yes (wait) |
| 404 Not found | `github api error: 404` | no |
| Network failure | `github api error: connection refused` | yes |

#### Timeout Contract
HTTP client timeout: 30s (hardcoded in `NewGitHubServer`). External API — expected latency: 200ms–2s.

---

### 3.4 Fetch Server

#### Input Contract

| Operation | Field | Type | Required | Notes |
|-----------|-------|------|----------|-------|
| `get`/`url` | `url` | string | yes | target URL |
| `status` | `url` | string | yes | URL to HEAD |
| `download` | `url` | string | yes | URL to GET |

#### Output Contract

```jsonc
// get/url
{"url": "https://example.com", "status": 200, "content_type": "text/html", "body": "...", "size": 1234}

// status
{"url": "https://example.com", "status": 200, "alive": true}

// download
{"url": "https://example.com/file.bin", "size": 5678, "data": "<binary>"}
```

#### Failure Contract

| Condition | Error Contains | Recoverable |
|-----------|---------------|-------------|
| Empty URL | `url is required` | yes |
| DNS failure | `http get failed: no such host` | yes |
| Timeout | `http get failed: timeout` | yes |
| 404/500 | No error — returns status in response | N/A |

#### Timeout Contract
Per-request timeout from `ctx.TimeoutMs` (default 30s in `httpClient()`). Expected latency varies by target:
- Fast API: 50–500ms
- Large payload: 1–10s
- Timeout scenario: fails after configured timeout

---

### 3.5 Context7 Server

#### Input Contract

| Operation | Field | Type | Required | Notes |
|-----------|-------|------|----------|-------|
| `query` | `key` | string | no | defaults to "default" |
| `query` | `service_url` | string | no | custom service URL |
| `store` | `key` | string | yes | storage key |
| `store` | `value` | any | no | arbitrary value |
| `resolve` | `key` | string | no | defaults to "session" |

#### Output Contract

```jsonc
// query (local fallback)
{"key": "default", "data": {"key": "default", "description": "deterministic context response for default", "type": "local", "resolved": true}}

// query (remote/cloud)
{"key": "default", "data": {...}, "source": "cloud"}

// store
{"key": "mykey", "stored": true}

// resolve
{"key": "session", "context": {"session_id": "...", "trace_id": "...", "workspace": "...", "resolved": true}}
```

#### Failure Contract

| Condition | Error Contains | Recoverable |
|-----------|---------------|-------------|
| Empty key (store) | `key is required` | yes |
| Remote service unreachable | `context7 service unreachable` | yes |
| Remote auth failure | `context7 service unreachable: 401` | yes |

#### Timeout Contract
Local operations: < 1ms. Remote: HTTP client timeout 10s (hardcoded in `NewContext7Server`).

---

### 3.6 Memory Server

#### Input Contract

| Operation | Field | Type | Required | Notes |
|-----------|-------|------|----------|-------|
| `store` | `key` | string | yes | storage key |
| `store` | `value` | string | yes | storage value |
| `retrieve` | `key` | string | yes | lookup key |
| `delete` | `key` | string | yes | key to remove |

#### Output Contract

```jsonc
// store
{"key": "mykey", "stored": true}

// retrieve
{"key": "mykey", "value": "myvalue"}

// delete
{"key": "mykey", "deleted": true}

// list
{"keys": ["key1", "key2"], "count": 2}
```

#### Failure Contract

| Condition | Error Contains | Recoverable |
|-----------|---------------|-------------|
| Empty key | `key is required` | yes |
| Key not found (retrieve) | `key not found: mykey` | yes |

#### Timeout Contract
In-process map operations: < 10µs. Persistence writes (if file configured): < 10ms.

---

### 3.7 Postgres Server

#### Input Contract

| Operation | Field | Type | Required | Notes |
|-----------|-------|------|----------|-------|
| `query` | `sql` | string | yes | SELECT query |
| `execute` | `sql` | string | yes | INSERT/UPDATE/DELETE |

#### Output Contract

```jsonc
// With connection string
{"status": 200, "result": [{"column": "value"}]}

// Without connection string (logged mode)
{"status": "logged", "sql": "SELECT 1", "notice": "no database connection configured — query was logged, not executed"}
```

#### Failure Contract

| Condition | Error Contains | Recoverable |
|-----------|---------------|-------------|
| Empty SQL | `sql is required` | yes |
| Connection failure | `postgres query failed: connection refused` | yes |

#### Timeout Contract
External HTTP query endpoint: depends on configuration. Logged mode: < 1ms.

---

### 3.8 ChromaDB Server

#### Input Contract

| Operation | Field | Type | Required | Notes |
|-----------|-------|------|----------|-------|
| `store` | `id` | string | yes | document ID |
| `store` | `document` | string | yes | document text |
| `store` | `collection` | string | no | defaults to `mcp_execution_memory` |
| `query` | `query` | string | yes | search query |
| `query` | `collection` | string | no | collection name |
| `delete` | `id` | string | yes | document ID |
| `delete` | `collection` | string | no | collection name |

#### Output Contract

```jsonc
// store (with credentials)
{"id": "doc-1", "status": 200, "collection": "mcp_execution_memory"}

// store (no credentials — logged mode)
{"id": "doc-1", "status": "logged", "notice": "no chroma cloud connection configured — document was logged, not stored"}

// query (with credentials)
{"query": "git status", "results": {"documents": [...], "metadatas": [...], "distances": [...]}}

// query (no credentials — fallback)
{}

// delete
{"id": "doc-1", "status": 200}

// list_collections (no credentials)
{"collections": ["mcp_execution_memory"], "status": "simulated"}
```

#### Failure Contract

| Condition | Error Contains | Recoverable |
|-----------|---------------|-------------|
| Missing id or document (store) | `id and document are required` | yes |
| Missing query (query) | `query is required` | yes |
| Cloud API error | `chroma store failed: 5xx` | yes |
| Network failure | `chroma query failed: connection refused` | yes |

#### Timeout Contract
HTTP client timeout: 10s (hardcoded in `NewChromaAdapter`). Without credentials: < 1ms (immediate fallback).

---

## 4. Validation Matrix

Every operation in the inventory must be tested against each cell in the matrix.

### Matrix Dimensions

| Dimension | Description | Acceptance Criteria |
|-----------|-------------|-------------------|
| **Success** | Normal operation with valid inputs | Returns expected output shape; no error |
| **Failure** | Operation with invalid/missing inputs | Returns expected error; system remains operational |
| **Timeout** | Operation that exceeds latency bound | Returns error; trace captures timeout; no goroutine leak |
| **Malformed** | Operation with unexpected payload types | Server rejects or handles gracefully; no panic |
| **Rate Limited** | High-frequency calls to same operation | Enforcement blocks or rate limiter triggers; logged |
| **Large Payload** | Operation with oversized inputs/outputs | Server handles without OOM; truncation if applicable |

### Priority P1: Filesystem

| Test Case | Operation | Input | Expected Outcome |
|-----------|-----------|-------|-----------------|
| FS-001: List root | `list` | `{}` | Returns workspace files |
| FS-002: List subdir | `list` | `{"path": "src"}` | Returns subdirectory contents |
| FS-003: Read existing file | `read` | `{"path": "main.go"}` | Returns file content, size, mode |
| FS-004: Read non-existent file | `read` | `{"path": "nonexistent.go"}` | Error: "read failed" |
| FS-005: Write new file | `write` | `{"path": "test.txt", "content": "hello"}` | Returns written: true |
| FS-006: Write to nested path | `write` | `{"path": "a/b/c/test.txt", "content": "nested"}` | Creates directories; returns success |
| FS-007: Delete file | `delete` | `{"path": "test.txt"}` | Returns deleted: true |
| FS-008: Search matching | `search` | `{"pattern": "*.go"}` | Returns matching files |
| FS-009: Search no match | `search` | `{"pattern": "*.xyz"}` | Returns empty matches |
| FS-010: Path traversal read | `read` | `{"path": "/etc/passwd"}` | Error: "path traversal denied" |
| FS-011: Path traversal write | `write` | `{"path": "../../etc/pwned", "content": "..."}` | Error: "path traversal denied" |
| FS-012: Large file write | `write` | `{"path": "large.bin", "content": "<10MB>"}` | Succeeds; size recorded |
| FS-013: Empty path | `read` | `{}` | Error: "path is required" |
| FS-014: Permission denied | `read` | `{"path": "/root/secret"}` | Error (O/S-level) |
| FS-015: Malformed payload | `read` | `{"path": 123}` | Go type assertion fails → error (recoverable) |

### Priority P2: Git

| Test Case | Operation | Input | Expected Outcome |
|-----------|-----------|-------|-----------------|
| GT-001: Status in repo | `status` | `{}` | Returns porcelain output |
| GT-002: Status outside repo | `status` | `{}` (temp dir) | Error: "git status failed" |
| GT-003: Diff with changes | `diff` | `{}` | Returns diff output |
| GT-004: Log | `log` | `{}` | Returns 10 most recent commits |
| GT-005: Branch list | `branch` | `{}` | Returns branch list with * marker |
| GT-006: Commit | `commit` | `{"message": "test commit"}` | Performs add + commit; returns output |
| GT-007: Commit without message | `commit` | `{}` | Error: "commit message is required" |
| GT-008: Tag | `tag` | `{"tag": "test-tag"}"` | Creates tag |
| GT-009: Tag without name | `tag` | `{}` | Error: "tag name is required" |
| GT-010: No workspace path | `status` | `{}` (empty context) | Error: "workspace path required" |

### Priority P3: GitHub

| Test Case | Operation | Input | Expected Outcome |
|-----------|-----------|-------|-----------------|
| GH-001: Read repo file | `repo` | `{"owner": "user", "repo": "repo", "path": "README.md"}` | Returns file data |
| GH-002: Read without auth | `repo` | same, no token | Error: "GitHub token required" |
| GH-003: List issues | `list_issues` | `{"owner": "user", "repo": "repo"}` | Returns issue list |
| GH-004: List issues with state | `list_issues` | `{"owner": "user", "repo": "repo", "state": "open"}` | Returns filtered list |
| GH-005: Create issue | `create_issue` | `{"owner": "user", "repo": "repo", "title": "test"}` | Returns created issue |
| GH-006: Create issue no title | `create_issue` | `{"owner": "user", "repo": "repo"}` | Error: "title is required" |
| GH-007: Missing owner/repo | `repo` | `{"path": "README.md"}` | Error: "owner, repo, and path are required" |
| GH-008: Rate limit response | `list_issues` | valid, rapid calls | Returns 403 status (not error) — verify trace captures this |

### Priority P4: Fetch

| Test Case | Operation | Input | Expected Outcome |
|-----------|-----------|-------|-----------------|
| FT-001: Successful GET | `get` | `{"url": "https://example.com"}` | Returns body, status 200, content type, size |
| FT-002: 404 | `get` | `{"url": "https://example.com/nonexistent"}` | Returns status 404 (not an error) |
| FT-003: DNS failure | `get` | `{"url": "https://nonexistent.invalid"}` | Error: "http get failed" |
| FT-004: Timeout | `get` | `{"url": "http://10.255.255.1:1"}` (with short timeout) | Error: "http get failed: timeout" |
| FT-005: Large payload | `get` | `{"url": "https://httpbin.org/bytes/10000"}` | Returns body with correct size |
| FT-006: URL status check | `status` | `{"url": "https://example.com"}` | Returns alive: true |
| FT-007: Empty URL | `get` | `{}` | Error: "url is required" |
| FT-008: Malformed URL | `get` | `{"url": "not-a-url"}` | Error: "http get failed" |

### Priority P5: Context7

| Test Case | Operation | Input | Expected Outcome |
|-----------|-----------|-------|-----------------|
| C7-001: Query (local) | `query` | `{"key": "test"}` | Returns local context data |
| C7-002: Query default key | `query` | `{}` | Returns default context |
| C7-003: Store value | `store` | `{"key": "k1", "value": "v1"}` | Returns stored: true |
| C7-004: Store without key | `store` | `{"value": "v1"}` | Error: "key is required" |
| C7-005: Resolve session | `resolve` | `{}` | Returns session context |
| C7-006: Resolve with key | `resolve` | `{"key": "mykey"}` | Returns stored or auto-created context |
| C7-007: Remote query (no API key) | `query` | `{"service_url": "https://localhost:9999"}` | Error: "context7 service unreachable" |

### Priority P6: Memory

| Test Case | Operation | Input | Expected Outcome |
|-----------|-----------|-------|-----------------|
| MEM-001: Store | `store` | `{"key": "k1", "value": "v1"}` | Returns stored: true |
| MEM-002: Retrieve | `retrieve` | `{"key": "k1"}` | Returns value: "v1" |
| MEM-003: Retrieve missing | `retrieve` | `{"key": "nonexistent"}` | Error: "key not found" |
| MEM-004: Delete | `delete` | `{"key": "k1"}` | Returns deleted: true |
| MEM-005: Delete missing | `delete` | `{"key": "k1"}` | Succeeds (no-op) |
| MEM-006: List | `list` | `{}` | Returns keys array |
| MEM-007: Store empty key | `store` | `{"value": "v1"}` | Error: "key is required" |
| MEM-008: Retrieve empty key | `retrieve` | `{}` | Error: "key is required" |
| MEM-009: Large value store | `store` | `{"key": "large", "value": "<1MB string>"}` | Succeeds |

### Priority P7: Postgres

| Test Case | Operation | Input | Expected Outcome |
|-----------|-----------|-------|-----------------|
| PG-001: List tables | `list_tables` | `{}` | Returns tables (logged mode: notice) |
| PG-002: Query | `query` | `{"sql": "SELECT 1"}` | Returns result |
| PG-003: Execute | `execute` | `{"sql": "CREATE TABLE test(id int)"}` | Returns status |
| PG-004: Empty SQL | `query` | `{}` | Error: "sql is required" |

### Priority P8: ChromaDB

| Test Case | Operation | Input | Expected Outcome |
|-----------|-----------|-------|-----------------|
| CH-001: Store document | `store` | `{"id": "test-1", "document": "test content"}` | Returns status (200 or "logged") |
| CH-002: Store without id | `store` | `{"document": "test"}` | Error: "id and document are required" |
| CH-003: Query | `query` | `{"query": "test"}` | Returns results or empty fallback |
| CH-004: Query without query | `query` | `{}` | Error: "query is required" |
| CH-005: Delete | `delete` | `{"id": "test-1"}` | Returns status |
| CH-006: List collections | `list_collections` | `{}` | Returns collection list |

---

## 5. Evidence Requirements

Each test case MUST produce the following evidence:

### Required Evidence Per Test

| Evidence | Source | Format | Retention |
|----------|--------|--------|-----------|
| **Logs** | `LogAudit()` output (structured JSON) | JSON object with timestamp, request_id, status, duration_ms, execution_allowed | Append to `REAL_INTEGRATION_EVIDENCE.md` |
| **Metrics** | `MetricsRegistry` counters | Snapshot before/after | Append counters delta |
| **Trace** | `DecisionTrace` from response meta | JSON trace object | Attach to test record |
| **Latency** | `time.Since(start)` per request | Duration in ms | Per-test latency record |
| **Memory** | `runtime.ReadMemStats` before/after | Alloc, TotalAlloc, Mallocs, Frees | Per-test memory delta |
| **Outcome** | Test pass/fail | PASS / FAIL with reason | Summary table |

### Evidence Collection Script (Pseudo)

```
for each test_case in PLAN:
    1. Snapshot MetricsRegistry
    2. Record runtime.ReadMemStats
    3. Execute MCPRequest through Gateway.Process()
    4. Capture DecisionTrace from resp.Meta.DecisionTrace
    5. Record latency = time.Since(start)
    6. Snapshot MetricsRegistry again → delta
    7. Record runtime.ReadMemStats again → delta
    8. Append to evidence document:
       - Test ID, Operation, Input
       - Outcome (PASS/FAIL)
       - Latency, Memory delta
       - Trace summary (selected_server, stages count)
       - Audit log entry
       - Any error details
```

---

## 6. Evidence Document Template

```markdown
## P1-FS-001: Filesystem List Root

**Date:** 2026-06-11
**Gateway Commit:** 3906c21

### Input
```json
{"action": {"type": "filesystem", "operation": "list"}, "payload": {}}
```

### Outcome
PASS

### Latency
Total: 2.3ms  
Stage Breakdown: validate=12µs policy=8µs knowledge=512µs enforcement=2µs execute=1.5ms

### Memory Delta
+45KB alloc, 0 mallocs

### DecisionTrace
- SelectedServer: filesystem
- Stages: [validate, policy, resolve, knowledge, route, enforcement, execute]
- KnowledgeUsed: ["mcp_execution_memory:filesystem.list"]

### Audit Log
```json
{"timestamp":"...","request_id":"...","action":"filesystem.list","server":"filesystem","status":"success","duration_ms":2,"execution_allowed":true}
```

### Notes
SafeJoin correctly prevented traversal. All 42 workspace files returned.
```

---

## 7. Reality Gates

Each gate must be COMPLETELY SATISFIED before the next begins. No partial gate crossings.

### Gate 1: Filesystem

| Criterion | Target | Evidence |
|-----------|--------|----------|
| List directory | Scenarios pass | Test IDs FS-001, FS-002 |
| Read file (existing + missing) | Scenarios pass | FS-003, FS-004 |
| Write file (new + nested) | Scenarios pass | FS-005, FS-006 |
| Delete file | Scenarios pass | FS-007 |
| Search (match + no match) | Scenarios pass | FS-008, FS-009 |
| Path traversal prevention | Blocked correctly | FS-010, FS-011 |
| Large file handling | Completes without OOM | FS-012 |
| Edge cases (empty path, malformed) | Graceful error | FS-013, FS-015 |
| Permission boundary | O/S error returned, not panic | FS-014 |
| **Gate pass** | **15/15 scenarios pass** | |

### Gate 2: Git

| Criterion | Target | Evidence |
|-----------|--------|----------|
| Status in repo | Works | GT-001 |
| Status outside repo | Error, not panic | GT-002 |
| Diff + Log + Branch | Works | GT-003, GT-004, GT-005 |
| Commit (with message) | Works | GT-006 |
| Commit without message | Error | GT-007 |
| Tag (with + without name) | Works + Error | GT-008, GT-009 |
| No workspace path | Error | GT-010 |
| **Gate pass** | **10/10 scenarios pass** | |

### Gate 3: Fetch

| Criterion | Target | Evidence |
|-----------|--------|----------|
| Successful GET | Returns body + status + size | FT-001 |
| 404 handling | Returns 404 (not error) | FT-002 |
| DNS failure | Error, not panic | FT-003 |
| Timeout handling | Error, goroutine-safe | FT-004 |
| Large payload | Returns correctly | FT-005 |
| URL status check | Alive check works | FT-006 |
| Empty/malformed URL | Error | FT-007, FT-008 |
| **Gate pass** | **Timeout handling verified, 8/8 pass** | |

### Gate 4: GitHub

| Criterion | Target | Evidence |
|-----------|--------|----------|
| Read repo file | Works with auth | GH-001 |
| Auth failure | Error, not panic | GH-002 |
| List issues | Works | GH-003, GH-004 |
| Create issue (with + without title) | Works + Error | GH-005, GH-006 |
| Missing owner/repo | Error | GH-007 |
| Rate limit handling | 403 captured, not crash | GH-008 |
| **Gate pass** | **8/8 scenarios pass** | |

### Gate 5: Memory

| Criterion | Target | Evidence |
|-----------|--------|----------|
| Store + Retrieve | Round-trip works | MEM-001, MEM-002 |
| Retrieve missing key | Error, not panic | MEM-003 |
| Delete (existing + missing) | Works | MEM-004, MEM-005 |
| List keys | Works | MEM-006 |
| Empty key validation | Error | MEM-007, MEM-008 |
| Large value store | Completes | MEM-009 |
| **Gate pass** | **Persistence verified, 9/9 pass** | |

### Gate 6: Context7

| Criterion | Target | Evidence |
|-----------|--------|----------|
| Local query | Returns context | C7-001, C7-002 |
| Store value | Works | C7-003 |
| Store without key | Error | C7-004 |
| Resolve session | Works | C7-005, C7-006 |
| Remote unavailable | Error, not panic | C7-007 |
| **Gate pass** | **Large context retrieval verified, 7/7 pass** | |

### Phase C0 Completion Gate

| Criterion | Target |
|-----------|--------|
| All 6 Reality Gates passed | 6/6 |
| No panics across all tests | 0 panics |
| Enforcement accuracy | 100% |
| Metrics capture coverage | 100% |
| Trace capture coverage | 100% |
| Evidence document complete | `REAL_INTEGRATION_EVIDENCE.md` |

---

## 8. Implementation Rules

### Golden Rule
**NO NEW CODE UNLESS REQUIRED FOR TEST EXECUTION.**

### Strictly Forbidden (Out of Scope)
- Memory Engine, Knowledge Layer, Semantic Search, Agent Planner, Reasoning Layer, Federation, Agent Swarm
- Any new Go package beyond `runtime/mcp/v2/integration_test.go`
- Modifications to existing runtime source files: `runtime/mcp/v2/*.go`, `runtime/metrics/*.go`
- Changes to existing test files: `gateway_test.go`, `benchmark_test.go`, `metrics_test.go`
- New dependencies in `go.mod`
- Bypassing Gateway to call MCP servers directly

### Allowed
- Small test harness or adapter in `runtime/mcp/v2/integration_test.go` if needed for execution
- `REAL_INTEGRATION_EVIDENCE.md` (evidence output only — created during execution)
- Using `Gateway.Process()` for all test execution
- Using `MetricsRegistry.Global()` for metrics capture
- Using `resp.Meta.DecisionTrace` for trace capture
- Using `testing.T` standard Go test framework
- Recording logs, metrics, traces, latency, memory, outcomes per test

### Execution Hierarchy
```
test script → Gateway.Process(req) → [Validate → Policy → Resolve → Knowledge → Score → Route → Enforcement → Execute → Trace → Audit → Learn → PolicyIntel]
                                                                                              ↑
                                                                                    Real MCP Server
                                                                                    (filesystem, git, etc.)
```

### Known Constraints
- GitHub tests require a valid token (stored in env or context.TenantID)
- ChromaDB tests require `CHROMA_API_KEY`, `CHROMA_TENANT`, `CHROMA_DATABASE` env vars
- Git tests require `git` binary in PATH
- Filesystem tests operate within the workspace path
- Context7 remote tests require `CONTEXT7_API_KEY` env var
- Postgres tests require connection string (or fall back to logged mode)
- Fetch tests require network access
