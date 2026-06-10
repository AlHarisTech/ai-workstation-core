"""AI Workstation — Local Function Execution Adapter

Provides the ONLY execution backend for v0.2.0.
All tools execute as local Python functions within the gateway process.

No containers. No subprocesses. No orchestration.

Each tool handler:
  - Receives the full tool definition + arguments dict.
  - Returns a dict with at least 'status' and 'result'.
  - Never raises — errors are captured in the result dict.
"""

import json
import os
import subprocess
import time
import urllib.request
import urllib.error

from tools.registry import ToolNotFoundError


_TOOL_MEMORY = {}


class LocalExecutor:
    """Dispatches tool calls to local Python functions.

    The handler mapping tells the executor which function to call for
    each tool id. Handlers map 1:1 to tools in definitions.yaml.
    Drift between registry and handler map is a CRITICAL governance violation.
    """

    HANDLERS = {
        "echo": "_handle_echo",
        "filesystem_read": "_handle_filesystem_read",
        "filesystem_write": "_handle_filesystem_write",
        "git_status": "_handle_git_status",
        "fetch_url": "_handle_fetch_url",
        "memory_store": "_handle_memory_store",
        "memory_retrieve": "_handle_memory_retrieve",
        "session_create": "_handle_session_create",
    }

    def __init__(self, registry, session_validator=None):
        self._registry = registry
        self._session_validator = session_validator

    def execute(self, tool_id, arguments, session_context=None):
        """Execute a tool by id.

        Args:
            tool_id: string tool identifier.
            arguments: dict of tool arguments.
            session_context: dict with session metadata (optional).

        Returns:
            dict with:
              - status: "success" | "error"
              - result: any JSON-serializable value
              - error: error message if status is "error"
              - error_trace: optional traceback string
              - duration_ms: execution time in milliseconds
        """
        start = time.time()

        try:
            tool_def = self._registry.get_tool(tool_id)
        except ToolNotFoundError as e:
            return self._error(str(e), duration_ms=time.time() - start)

        handler_name = self.HANDLERS.get(tool_id)
        if handler_name is None:
            return self._error(
                f"Tool '{tool_id}' is registered but has no runtime handler",
                duration_ms=time.time() - start,
            )

        handler_method = getattr(self, handler_name, None)
        if handler_method is None:
            return self._error(
                f"Handler '{handler_name}' for tool '{tool_id}' is not implemented",
                duration_ms=time.time() - start,
            )

        try:
            result = handler_method(tool_def, arguments, session_context)
            duration_ms = (time.time() - start) * 1000
            tool_status = result.get("status", "success") if isinstance(result, dict) else "success"
            tool_error = result.get("error") if isinstance(result, dict) else None
            return {
                "status": tool_status,
                "result": result,
                "error": tool_error,
                "duration_ms": round(duration_ms, 2),
            }
        except Exception as e:
            import traceback
            duration_ms = (time.time() - start) * 1000
            return self._error(
                str(e),
                error_trace=traceback.format_exc(),
                duration_ms=round(duration_ms, 2),
            )

    @staticmethod
    def _error(message, error_trace=None, duration_ms=0.0):
        return {
            "status": "error",
            "result": None,
            "error": message,
            "error_trace": error_trace,
            "duration_ms": round(float(duration_ms), 2),
        }

    @staticmethod
    def _safe_path(path):
        """Expand and normalize a file path."""
        return os.path.abspath(os.path.expanduser(path))

    def _deny_path(self, path):
        """Check if path matches any deny pattern.

        For v0.2.0: deny absolute paths under /etc/, /proc/,
        and relative paths under .ai/config/secrets/.
        """
        expanded = self._safe_path(path)
        deny_prefixes = [
            "/etc/",
            "/proc/",
            "/sys/",
        ]
        for prefix in deny_prefixes:
            if expanded.startswith(prefix):
                return True
        deny_segments = [
            ".ai/config/secrets",
            ".ai/config/secrets/",
        ]
        for seg in deny_segments:
            if seg in expanded:
                return True
        return False

    def _handle_echo(self, tool_def, arguments, session_context):
        return arguments

    def _handle_filesystem_read(self, tool_def, arguments, session_context):
        path = arguments.get("path")
        if not path:
            return {"status": "error", "error": "Missing required argument: path"}
        if self._deny_path(path):
            return {"status": "error", "error": f"Access denied for path: {path}"}
        try:
            with open(self._safe_path(path), "r", encoding="utf-8") as f:
                content = f.read()
        except FileNotFoundError:
            return {"status": "error", "error": f"File not found: {path}"}
        except PermissionError:
            return {"status": "error", "error": f"Permission denied: {path}"}
        return {"content": content, "path": path, "size_bytes": len(content)}

    def _handle_filesystem_write(self, tool_def, arguments, session_context):
        path = arguments.get("path")
        content = arguments.get("content")
        if not path:
            return {"status": "error", "error": "Missing required argument: path"}
        if content is None:
            return {"status": "error", "error": "Missing required argument: content"}
        if self._deny_path(path):
            return {"status": "error", "error": f"Access denied for path: {path}"}
        try:
            os.makedirs(os.path.dirname(self._safe_path(path)), exist_ok=True)
            with open(self._safe_path(path), "w", encoding="utf-8") as f:
                f.write(str(content))
        except PermissionError:
            return {"status": "error", "error": f"Permission denied: {path}"}
        return {"written": True, "path": path, "size_bytes": len(str(content))}

    def _handle_git_status(self, tool_def, arguments, session_context):
        cwd = arguments.get("cwd", os.getcwd())
        try:
            result = subprocess.run(
                ["git", "status", "--porcelain"],
                capture_output=True,
                text=True,
                timeout=10,
                cwd=cwd,
            )
        except FileNotFoundError:
            return {"status": "error", "error": "git command not found"}
        except subprocess.TimeoutExpired:
            return {"status": "error", "error": "git status timed out"}
        return {
            "output": result.stdout.strip(),
            "returncode": result.returncode,
            "stderr": result.stderr.strip(),
            "cwd": cwd,
        }

    def _handle_fetch_url(self, tool_def, arguments, session_context):
        url = arguments.get("url")
        if not url:
            return {"status": "error", "error": "Missing required argument: url"}
        if not url.startswith(("http://", "https://")):
            return {"status": "error", "error": f"Invalid URL scheme: {url}"}
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "AI-Workstation/0.2.0"})
            with urllib.request.urlopen(req, timeout=10) as resp:
                body = resp.read().decode("utf-8", errors="replace")
            return {
                "status_code": resp.status,
                "headers": dict(resp.headers),
                "body": body[:10000],
                "truncated": len(body) > 10000,
            }
        except urllib.error.URLError as e:
            return {"status": "error", "error": f"Fetch failed: {e.reason}"}
        except Exception as e:
            return {"status": "error", "error": f"Fetch error: {e}"}

    def _handle_memory_store(self, tool_def, arguments, session_context):
        key = arguments.get("key")
        value = arguments.get("value")
        if not key:
            return {"status": "error", "error": "Missing required argument: key"}
        if value is None:
            return {"status": "error", "error": "Missing required argument: value"}
        ns = arguments.get("namespace", "default")
        if ns not in _TOOL_MEMORY:
            _TOOL_MEMORY[ns] = {}
        _TOOL_MEMORY[ns][key] = value
        return {"stored": True, "key": key, "namespace": ns}

    def _handle_memory_retrieve(self, tool_def, arguments, session_context):
        key = arguments.get("key")
        if not key:
            return {"status": "error", "error": "Missing required argument: key"}
        ns = arguments.get("namespace", "default")
        if ns not in _TOOL_MEMORY or key not in _TOOL_MEMORY[ns]:
            return {"status": "error", "error": f"Key not found: {key} (namespace: {ns})"}
        return {"found": True, "key": key, "value": _TOOL_MEMORY[ns][key], "namespace": ns}

    def _handle_session_create(self, tool_def, arguments, session_context):
        if self._session_validator is None:
            return {"status": "error", "error": "Session validator not configured"}
        project_id = arguments.get("project_id", "default-project")
        user_id = arguments.get("user_id", "default")
        session = self._session_validator.create_session(project_id, user_id)
        return {"session": session}
