"""AI Workstation — Persistent State Layer (v0.4.0)

File-based persistent storage for sessions, registry versions, and
execution traces. No external database required.

Storage layout:
  .ai/state/
  ├── sessions/          # One JSON file per session_id
  ├── traces/            # One JSON file per request_id
  └── meta.json          # Registry version, platform metadata

Properties:
  - Atomic writes: write to temp file, then os.rename
  - JSON: human-readable, diffable
  - Namespaced: sessions by session_id, traces by request_id
"""

from __future__ import annotations

import json
import os
import tempfile
import threading
from datetime import datetime, timezone


class StateStore:
    """File-based persistent state store.

    Thread-safe. Uses atomic file writes (write temp → rename).
    State survives process restarts.
    """

    def __init__(self, root_dir=None):
        if root_dir is None:
            root_dir = os.path.join(
                os.environ.get("AI_WORKSTATION_ROOT", os.getcwd()),
                ".ai", "state",
            )
        self._root = root_dir
        self._lock = threading.Lock()
        self._ensure_directories()

    def _ensure_directories(self):
        os.makedirs(os.path.join(self._root, "sessions"), exist_ok=True)
        os.makedirs(os.path.join(self._root, "traces"), exist_ok=True)

    def _atomic_write(self, filepath, data):
        """Write JSON data atomically using temp file + rename."""
        dir_path = os.path.dirname(filepath)
        os.makedirs(dir_path, exist_ok=True)
        fd, tmp_path = tempfile.mkstemp(dir=dir_path, suffix=".tmp")
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as f:
                json.dump(data, f, default=str, indent=2)
            os.rename(tmp_path, filepath)
        except Exception:
            if os.path.exists(tmp_path):
                os.unlink(tmp_path)
            raise

    def save_session(self, session):
        """Persist a session dict by session_id."""
        sid = session.get("session_id")
        if not sid:
            return
        filepath = os.path.join(self._root, "sessions", f"{sid}.json")
        with self._lock:
            self._atomic_write(filepath, session)

    def load_session(self, session_id):
        """Load a persisted session by session_id."""
        filepath = os.path.join(self._root, "sessions", f"{session_id}.json")
        if not os.path.isfile(filepath):
            return None
        with self._lock:
            with open(filepath, "r", encoding="utf-8") as f:
                return json.load(f)

    def list_sessions(self):
        """List all persisted session IDs."""
        sdir = os.path.join(self._root, "sessions")
        if not os.path.isdir(sdir):
            return []
        return [f.replace(".json", "") for f in os.listdir(sdir) if f.endswith(".json")]

    def save_trace(self, request_id, trace_data):
        """Persist an execution trace by request_id."""
        filepath = os.path.join(self._root, "traces", f"{request_id}.json")
        with self._lock:
            self._atomic_write(filepath, trace_data)

    def load_trace(self, request_id):
        """Load a persisted execution trace by request_id."""
        filepath = os.path.join(self._root, "traces", f"{request_id}.json")
        if not os.path.isfile(filepath):
            return None
        with self._lock:
            with open(filepath, "r", encoding="utf-8") as f:
                return json.load(f)

    def save_meta(self, meta_dict):
        """Persist platform metadata (registry version, etc.)."""
        filepath = os.path.join(self._root, "meta.json")
        meta_dict["updated_at"] = datetime.now(timezone.utc).isoformat()
        with self._lock:
            self._atomic_write(filepath, meta_dict)

    def load_meta(self):
        """Load platform metadata."""
        filepath = os.path.join(self._root, "meta.json")
        if not os.path.isfile(filepath):
            return {"version": "0.4.0", "state": "fresh"}
        with self._lock:
            with open(filepath, "r", encoding="utf-8") as f:
                return json.load(f)

    def store_count(self):
        """Return counts of stored sessions and traces."""
        sdir = os.path.join(self._root, "sessions")
        tdir = os.path.join(self._root, "traces")
        sc = len([f for f in os.listdir(sdir) if f.endswith(".json")]) if os.path.isdir(sdir) else 0
        tc = len([f for f in os.listdir(tdir) if f.endswith(".json")]) if os.path.isdir(tdir) else 0
        return {"sessions": sc, "traces": tc}

    @property
    def root(self):
        return self._root
