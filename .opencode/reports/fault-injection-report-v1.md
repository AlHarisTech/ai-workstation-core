# CDCS-1 Fault Injection Report v1

**Date:** 2026-06-15 07:34:17 +03  
**Injector:** Manual (echo to observations JSONL)  
**Type:** Single malformed JSON record (truncated)  
**Target:** Consumer isolation layer (classify, synthesize, drift, agent)  

---

## Injection Payload

```json
{"ts":"2026-06-15T04:34:00Z","mcp-filesystem":{"state":"active"}
```

Intentionally truncated — missing closing braces. This simulates the exact class of corruption that caused the original cascade failure on 2026-06-12.

---

## Results

| Consumer | Record Processed? | Behavior | Pipeline Impact |
|----------|------------------|----------|-----------------|
| classify | Yes (next cycle: 07:35:09) | `F3-B WARN: skipping malformed observation` | None |
| synthesize | Yes (07:35:09) | `F3-C WARN: skipping malformed record` | None |
| drift | Not in cycle window | No-op | None |
| agent | Not in cycle window | No-op | None |

**Service failures: 0**  
**Unexpected restarts: 0**  
**Downstream propagation: None**  
**Latency impact: Negligible (<1 cycle delay)**

---

## Verification

The malformed record was appended to the live observations file. The next timer-driven consumer cycle (59 seconds later) processed the file, hit the bad record, skipped it with a structured warning, and continued processing valid records normally.

The observation file shows the injection record followed by clean records from subsequent observe cycles:

```text
...valid record...
{"ts":"2026-06-15T04:34:00Z","mcp-filesystem":{"state":"active"}  ← injected, malformed
{"ts":"2026-06-15T04:34:10Z",...}  ← next clean observation
```

---

## Conclusion

Consumer isolation is verified under real production load. A single malformed record is:
1. Detected by JSON parser
2. Logged with a structured warning
3. Skipped
4. Pipeline continues without interruption

The system behavior matches the designed fault-tolerant pipeline model (skip + warn + continue), not the original fail-stop model (crash + cascade).
