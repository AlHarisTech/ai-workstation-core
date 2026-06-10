"""AI Workstation — Structured Logger (Append-Only JSON)

Provides deterministic, append-only structured logging for the MCP Gateway.
Every log entry is a single JSON line containing the full lifecycle trace.

Properties:
  - Append-only: no truncation, no rotation in MVP.
  - Structured: every entry has request_id, session_id, tool, routing, result, error.
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
    "routing_decision",
    "execution_result",
    "duration_ms",
    "error",
    "error_trace",
    "status",
    "timestamp",
]


class StructuredLogger:
    """Append-only structured JSON logger.

    Thread-safe. Writes one JSON object per line to a log file.
    Log file path defaults to .ai/governance/audit/gateway.log relative
    to the workspace root, determined by the AI_WORKSTATION_ROOT
    environment variable.
    """

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
                   null defaults.

        Returns:
            None. Logging failure is captured but never raises.
        """
        entry.setdefault("timestamp", datetime.now(timezone.utc).isoformat())
        record = {k: entry.get(k) for k in _ENTRY_FIELDS}
        try:
            with self._lock:
                with open(self._log_path, "a", encoding="utf-8") as f:
                    f.write(json.dumps(record, default=str) + "\n")
        except Exception:
            pass

    @property
    def log_path(self):
        return self._log_path
