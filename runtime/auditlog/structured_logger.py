"""AI Workstation — Structured Logger v0.3.0 (Enhanced Observability)

Append-only JSON audit log. Every entry captures the full execution
trace graph, decision path, and stage-level timing metrics.

New in v0.3.0:
  - execution_trace: ordered array of stage results
  - decision_path: ordered list of stage names visited
  - stage_timings: dict mapping stage name → duration in ms
  - error_code: structured error classification

Properties:
  - Append-only: no truncation, no rotation in MVP.
  - Structured: every entry has request_id, session_id, tool_id, etc.
  - Traceable: full execution trace reconstructable from log alone.
  - Fail-safe: logging failure never blocks gateway execution.
"""

import json
import os
import threading
from datetime import datetime, timezone


_ENTRY_FIELDS = [
    "request_id",
    "session_id",
    "project_id",
    "tool_id",
    "capability",
    "status",
    "decision_path",
    "stage_timings",
    "total_ms",
    "execution_trace",
    "error",
    "error_code",
    "timestamp",
]


class StructuredLogger:
    """Append-only structured JSON logger with execution trace support."""

    def __init__(self, log_path=None):
        if log_path is None:
            root = os.environ.get("AI_WORKSTATION_ROOT", os.getcwd())
            log_path = os.path.join(root, ".ai", "governance", "audit", "gateway.log")
        self._log_path = log_path
        self._lock = threading.Lock()
        self._ensure_log_directory()

    def _ensure_log_directory(self):
        dir_path = os.path.dirname(self._log_path)
        os.makedirs(dir_path, exist_ok=True)

    def log(self, entry):
        """Append a structured log entry.

        Args:
            entry: dict with keys from _ENTRY_FIELDS. Unknown keys are
                   silently dropped. Missing fields are populated with
                   null defaults. execution_trace, stage_timings, and
                   decision_path are preserved as complex structures.
        """
        entry.setdefault("timestamp", datetime.now(timezone.utc).isoformat())
        record = {}
        for k in _ENTRY_FIELDS:
            v = entry.get(k)
            record[k] = v if v is not None else None
        try:
            with self._lock:
                with open(self._log_path, "a", encoding="utf-8") as f:
                    f.write(json.dumps(record, default=str) + "\n")
        except Exception:
            pass

    @property
    def log_path(self):
        return self._log_path
