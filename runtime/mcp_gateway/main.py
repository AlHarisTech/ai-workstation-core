"""AI Workstation — MCP Gateway (v0.2.0)

Stdio-based MCP Gateway runtime. Reads JSON-encoded requests from stdin,
processes through the full governance pipeline, writes JSON responses to stdout.

Pipeline:
  Parse → Validate Session → Route → Execute → Log → Respond

Usage:
  echo '{"id":"req_001","method":"tool.call","params":{...},"session":{...}}' | python3 main.py

  Or interactive mode:
  python3 main.py

Environment:
  AI_WORKSTATION_ROOT — workspace root path (defaults to cwd)
"""

import json
import os
import sys
import time
import traceback
import uuid

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from session.session_validator import SessionValidator
from tools.registry import ToolRegistry, RegistryLoadError, ToolNotFoundError
from executor.local_executor import LocalExecutor
from logging.structured_logger import StructuredLogger


_REQUEST_METHOD_TOOL_CALL = "tool.call"
_REQUEST_METHOD_SESSION_CREATE = "session.create"
_REQUEST_METHOD_TOOL_LIST = "tool.list"


class Gateway:
    """MCP Gateway runtime.

    Orchestrates the full request pipeline. Stateless between requests.
    Maintains references to registry, validator, executor, and logger.
    """

    def __init__(self, workspace_root=None):
        self._workspace_root = workspace_root or os.environ.get(
            "AI_WORKSTATION_ROOT", os.getcwd()
        )
        self._logger = StructuredLogger()
        self._validator = SessionValidator()
        self._registry = ToolRegistry()
        self._executor = LocalExecutor(
            registry=self._registry,
            session_validator=self._validator,
        )

    def start(self):
        """Load the registry and signal readiness."""
        count = self._registry.load()
        self._log("gateway.start", {
            "status": "gateway_started",
            "message": f"Registry loaded: {count} tools",
            "tool_count": count,
            "tool_ids": self._registry.tool_ids,
        })

    def handle_request(self, raw_request):
        """Process a single JSON request through the full pipeline.

        Args:
            raw_request: dict with id, method, params, session.

        Returns:
            dict: unified response.
        """
        request_id = raw_request.get("id", f"auto_{uuid.uuid4().hex[:8]}")
        method = raw_request.get("method", _REQUEST_METHOD_TOOL_CALL)
        params = raw_request.get("params", {})
        session_data = raw_request.get("session", {})

        t_start = time.time()

        # === ROUTING (phase 1: resolve tool_id) ===
        tool_id = params.get("tool")
        capability = params.get("capability")

        if not tool_id and capability:
            tool = self._registry.find_first_by_capability(capability)
            if tool:
                tool_id = tool["id"]
            else:
                return self._reject(
                    request_id=request_id,
                    code="ROUTING_FAILED",
                    message=f"No tool found for capability: {capability}",
                    errors=[f"Unknown capability: {capability}"],
                    session_data=session_data,
                )

        if not tool_id:
            return self._reject(
                request_id=request_id,
                code="MISSING_TOOL",
                message="Request must specify 'tool' or 'capability' in params",
                errors=["Missing tool or capability in params"],
                session_data=session_data,
            )

        # === TOOL EXISTENCE CHECK ===
        try:
            requires_session = self._registry.requires_session(tool_id)
        except ToolNotFoundError:
            return self._reject(
                request_id=request_id,
                code="TOOL_NOT_FOUND",
                message=f"Tool not found in registry: {tool_id}",
                errors=[f"Unknown tool: {tool_id}"],
                session_data=session_data,
            )

        # === SESSION VALIDATION (only if tool requires it) ===
        valid = True
        errors = []
        if requires_session:
            valid, errors = self._validator.validate(session_data)
            if not valid:
                return self._reject(
                    request_id=request_id,
                    code="SESSION_INVALID",
                    message="Session validation failed",
                    errors=errors,
                    session_data=session_data,
                )

        # === EXECUTION ===
        arguments = params.get("arguments", {})
        exec_result = self._executor.execute(
            tool_id=tool_id,
            arguments=arguments,
            session_context=session_data if valid else None,
        )

        # === RESPONSE ASSEMBLY ===
        total_ms = round((time.time() - t_start) * 1000, 2)

        response = {
            "id": request_id,
            "status": exec_result.get("status", "error"),
            "tool_id": tool_id,
            "routing": {
                "tool_id": tool_id,
                "capability": capability,
                "resolved": True,
            },
            "session": {
                "valid": True,
                "session_id": session_data.get("session_id"),
                "project_id": session_data.get("project_id"),
            },
            "execution": {
                "duration_ms": exec_result.get("duration_ms", 0),
                "total_ms": total_ms,
            },
            "result": exec_result.get("result"),
            "error": exec_result.get("error"),
            "error_trace": exec_result.get("error_trace"),
        }

        # === AUDIT LOGGING ===
        self._log("gateway.request", {
            "request_id": request_id,
            "session_id": session_data.get("session_id"),
            "project_id": session_data.get("project_id"),
            "tool_id": tool_id,
            "capability": capability,
            "routing_decision": tool_id,
            "execution_result": exec_result.get("status"),
            "duration_ms": exec_result.get("duration_ms"),
            "error": exec_result.get("error"),
            "error_trace": exec_result.get("error_trace"),
            "status": response["status"],
        })

        return response

    def handle_method(self, raw_request):
        """Route method-level requests before pipeline processing.

        Supported methods:
          - tool.call: full pipeline processing
          - session.create: create a new session
          - tool.list: list available tools
        """
        method = raw_request.get("method", _REQUEST_METHOD_TOOL_CALL)

        if method == _REQUEST_METHOD_SESSION_CREATE:
            return self._handle_session_create(raw_request)
        elif method == _REQUEST_METHOD_TOOL_LIST:
            return self._handle_tool_list(raw_request)
        else:
            return self.handle_request(raw_request)

    def _handle_session_create(self, raw_request):
        request_id = raw_request.get("id", f"auto_{uuid.uuid4().hex[:8]}")
        params = raw_request.get("params", {})
        project_id = params.get("project_id", "default-project")
        user_id = params.get("user_id", "default")
        session = self._validator.create_session(project_id, user_id)
        return {
            "id": request_id,
            "status": "success",
            "result": session,
        }

    def _handle_tool_list(self, raw_request):
        request_id = raw_request.get("id", f"auto_{uuid.uuid4().hex[:8]}")
        tools = []
        for tid in self._registry.tool_ids:
            try:
                tool = self._registry.get_tool(tid)
                tools.append({
                    "id": tool["id"],
                    "name": tool["name"],
                    "capabilities": tool.get("capabilities", []),
                    "require_session": tool.get("governance", {}).get("require_session", True),
                })
            except ToolNotFoundError:
                continue
        return {
            "id": request_id,
            "status": "success",
            "result": {"tools": tools, "count": len(tools)},
        }

    def _reject(self, request_id, code, message, errors, session_data=None):
        """Build a fail-closed rejection response."""
        return {
            "id": request_id,
            "status": "error",
            "error": {
                "code": code,
                "message": message,
                "details": errors,
            },
            "session": {
                "valid": False,
                "session_id": session_data.get("session_id") if session_data else None,
                "project_id": session_data.get("project_id") if session_data else None,
                "errors": errors,
            },
        }

    def _log(self, event_type, data):
        try:
            self._logger.log(data)
        except Exception:
            pass


def main():
    """Entry point. Reads JSON lines from stdin, writes JSON lines to stdout."""
    gw = Gateway()

    try:
        gw.start()
    except RegistryLoadError as e:
        print(json.dumps({"status": "fatal", "error": str(e)}), file=sys.stderr)
        sys.exit(1)

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        try:
            request = json.loads(line)
        except json.JSONDecodeError as e:
            response = {
                "id": None,
                "status": "error",
                "error": {
                    "code": "PARSE_ERROR",
                    "message": f"Invalid JSON: {e}",
                },
            }
            print(json.dumps(response))
            sys.stdout.flush()
            continue

        try:
            response = gw.handle_method(request)
        except Exception:
            response = {
                "id": request.get("id", "unknown"),
                "status": "error",
                "error": {
                    "code": "INTERNAL_ERROR",
                    "message": "Gateway internal error",
                    "trace": traceback.format_exc(),
                },
            }

        print(json.dumps(response, default=str))
        sys.stdout.flush()


if __name__ == "__main__":
    main()
