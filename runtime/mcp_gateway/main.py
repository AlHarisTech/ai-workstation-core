"""AI Workstation — MCP Gateway v0.3.0 (Control Plane Kernel)

Stdio-based deterministic control plane runtime. Reads JSON requests from
stdin, runs the 7-stage middleware pipeline, writes JSON responses to stdout.

Pipeline:
  PreValidation → SessionGuard → CapabilityRouting → PreExecution
  → Execution → PostValidation → AuditLog

Each stage is explicit and independently measurable. Deny verdicts stop the
pipeline immediately (fail-closed).

Usage:
  echo '{"id":"r1","method":"tool.call",...}' | python3 main.py

Environment:
  AI_WORKSTATION_ROOT — workspace root path (defaults to cwd)
"""

import json
import os
import sys
import traceback

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from session.session_validator import SessionValidator
from session.session_guard import SessionGuard
from tools.registry import ToolRegistry, RegistryLoadError
from executor.local_executor import LocalExecutor
from auditlog.structured_logger import StructuredLogger
from governance.policy_engine import PolicyEngine, PolicyLoadError
from mcp_gateway.context import RequestContext
from mcp_gateway.pipeline import Pipeline, PipelineServices


_REQUEST_METHOD_TOOL_CALL = "tool.call"
_REQUEST_METHOD_SESSION_CREATE = "session.create"
_REQUEST_METHOD_TOOL_LIST = "tool.list"


def main():
    """Entry point. Initializes services, builds pipeline, processes stdin."""
    root = os.environ.get("AI_WORKSTATION_ROOT", os.getcwd())

    logger = StructuredLogger()
    policy_engine = PolicyEngine()
    registry = ToolRegistry()
    validator = SessionValidator()
    executor = LocalExecutor(registry=registry, session_validator=validator)
    session_guard = SessionGuard(session_validator=validator, policy_engine=policy_engine)

    # Load governance policies
    try:
        policy_count = policy_engine.load()
    except PolicyLoadError as e:
        print(json.dumps({"status": "fatal", "error": f"Policy load failed: {e}"}), file=sys.stderr)
        sys.exit(1)

    # Load tool registry
    try:
        tool_count = registry.load()
    except RegistryLoadError as e:
        print(json.dumps({"status": "fatal", "error": f"Registry load failed: {e}"}), file=sys.stderr)
        sys.exit(1)

    services = PipelineServices(
        registry=registry,
        executor=executor,
        session_guard=session_guard,
        policy_engine=policy_engine,
        logger=logger,
    )

    pipeline = Pipeline(services)

    logger.log({
        "status": "gateway_started",
        "policy_count": policy_count,
        "policy_version": policy_engine.version,
        "tool_count": tool_count,
        "registry_version": registry.version,
        "enforcement": policy_engine.enforcement_mode,
    })

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        try:
            raw_request = json.loads(line)
        except json.JSONDecodeError as e:
            response = {
                "id": None,
                "status": "error",
                "error": {"code": "PARSE_ERROR", "message": f"Invalid JSON: {e}"},
            }
            print(json.dumps(response))
            sys.stdout.flush()
            continue

        method = raw_request.get("method", _REQUEST_METHOD_TOOL_CALL)

        if method == _REQUEST_METHOD_SESSION_CREATE:
            response = _handle_session_create(raw_request, validator)
        elif method == _REQUEST_METHOD_TOOL_LIST:
            response = _handle_tool_list(raw_request, registry)
        else:
            try:
                context = RequestContext.create(raw_request)
                context = pipeline.process(context)
                response = context.as_response()
            except Exception:
                response = {
                    "id": raw_request.get("id", "unknown"),
                    "status": "error",
                    "error": {
                        "code": "INTERNAL_ERROR",
                        "message": "Gateway internal error",
                        "trace": traceback.format_exc(),
                    },
                }

        print(json.dumps(response, default=str))
        sys.stdout.flush()


def _handle_session_create(raw_request, validator):
    request_id = raw_request.get("id", f"auto_ses")
    params = raw_request.get("params", {})
    project_id = params.get("project_id", "default-project")
    user_id = params.get("user_id", "default")
    session = validator.create_session(project_id, user_id)
    return {
        "id": request_id,
        "status": "success",
        "result": session,
    }


def _handle_tool_list(raw_request, registry):
    request_id = raw_request.get("id", f"auto_tl")
    tools = []
    for tid in registry.tool_ids:
        try:
            tool = registry.get_tool(tid)
            tools.append({
                "id": tool["id"],
                "name": tool["name"],
                "capabilities": tool.get("capabilities", []),
                "require_session": tool.get("governance", {}).get("require_session", True),
            })
        except Exception:
            continue
    return {
        "id": request_id,
        "status": "success",
        "result": {"tools": tools, "count": len(tools), "registry_version": registry.version},
    }


if __name__ == "__main__":
    main()
