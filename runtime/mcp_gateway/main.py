"""AI Workstation — MCP Gateway v0.4.0 (Adaptive Runtime Kernel)

Queue-driven, multi-worker, persistent-state runtime kernel.

Architecture:
  stdin → RequestContext.create() → RequestQueue.put()
  Worker pool pulls from queue → Pipeline.process() → ResultQueue
  Main thread reads results → stdout

Features (v0.4.0):
  - Execution queue with backpressure
  - Shared worker pool (configurable N)
  - Persistent state (sessions, traces, meta)
  - Dual pipeline mode (STRICT / OPTIMIZED)
  - Policy evaluation graph per request
  - Latency breakdown per request

Usage:
  echo '{"id":"r1",...}' | python3 main.py

Environment:
  AI_WORKSTATION_ROOT — workspace root path
  AI_GATEWAY_WORKERS — worker count (default: 4)
  AI_QUEUE_SIZE — max queue depth (default: 128)
"""

import json
import os
import sys
import time
import threading
import uuid

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from session.session_validator import SessionValidator
from session.session_guard import SessionGuard
from tools.registry import ToolRegistry, RegistryLoadError
from executor.local_executor import LocalExecutor
from auditlog.structured_logger import StructuredLogger
from governance.policy_engine import PolicyEngine, PolicyLoadError
from mcp_gateway.context import RequestContext
from mcp_gateway.pipeline import Pipeline, PipelineServices
from kernel.queue import RequestQueue, QueueFullError, QueueEmptyError
from kernel.worker_pool import WorkerPool
from kernel.state_store import StateStore


_REQUEST_METHOD_TOOL_CALL = "tool.call"
_REQUEST_METHOD_SESSION_CREATE = "session.create"
_REQUEST_METHOD_TOOL_LIST = "tool.list"

_WORKER_COUNT = int(os.environ.get("AI_GATEWAY_WORKERS", "4"))
_QUEUE_SIZE = int(os.environ.get("AI_QUEUE_SIZE", "128"))


def _process_request(context, services, worker_id):
    """Worker function: enqueue timestamp, run pipeline, set worker_id."""
    context.worker_id = worker_id
    context.timestamp_start = time.time()
    return services.pipeline.process(context)


def _enqueue_timestamp(context):
    """Record enqueue time for queue_wait measurement."""
    context._enqueue_time = time.time()


def main():
    root = os.environ.get("AI_WORKSTATION_ROOT", os.getcwd())
    logger = StructuredLogger()
    state_store = StateStore(root_dir=os.path.join(root, ".ai", "state"))
    policy_engine = PolicyEngine()
    registry = ToolRegistry()
    validator = SessionValidator()
    executor = LocalExecutor(registry=registry, session_validator=validator)
    session_guard = SessionGuard(session_validator=validator, policy_engine=policy_engine)

    # Load governance
    try:
        policy_count = policy_engine.load()
    except PolicyLoadError as e:
        print(json.dumps({"status": "fatal", "error": f"Policy load: {e}"}), file=sys.stderr)
        sys.exit(1)

    # Load registry
    try:
        tool_count = registry.load()
    except RegistryLoadError as e:
        print(json.dumps({"status": "fatal", "error": f"Registry load: {e}"}), file=sys.stderr)
        sys.exit(1)

    # Persist meta
    state_store.save_meta({
        "version": "0.4.0",
        "registry_version": registry.version,
        "policy_version": policy_engine.version,
        "workers": _WORKER_COUNT,
        "queue_size": _QUEUE_SIZE,
    })

    services = PipelineServices(
        registry=registry,
        executor=executor,
        session_guard=session_guard,
        policy_engine=policy_engine,
        logger=logger,
    )
    services.pipeline = Pipeline(services)

    logger.log({
        "status": "kernel_started",
        "workers": _WORKER_COUNT,
        "queue_size": _QUEUE_SIZE,
        "tool_count": tool_count,
        "policy_count": policy_count,
        "pipeline_modes": ["strict", "optimized"],
        "state_root": state_store.root,
    })

    # Initialize queue and worker pool
    request_queue = RequestQueue(max_size=_QUEUE_SIZE)
    result_list = []
    result_lock = threading.Lock()
    shutdown_event = threading.Event()

    # Start worker pool
    for i in range(_WORKER_COUNT):
        worker_id = f"wrk_{i:03d}"
        t = threading.Thread(
            target=_worker_loop,
            args=(worker_id, request_queue, result_list, result_lock,
                  shutdown_event, services, state_store),
            name=f"gateway-{worker_id}",
            daemon=True,
        )
        t.start()

    # Result collector thread
    output_lock = threading.Lock()

    def _result_collector():
        while not shutdown_event.is_set() or result_list:
            with result_lock:
                while result_list:
                    ctx = result_list.pop(0)
                    try:
                        response = ctx.as_response()
                        with output_lock:
                            print(json.dumps(response, default=str))
                            sys.stdout.flush()
                    except Exception:
                        pass
            if shutdown_event.is_set() and not result_list:
                break
            time.sleep(0.005)

    collector = threading.Thread(target=_result_collector, daemon=True)
    collector.start()

    # Stdin reader: parse, enqueue
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        try:
            raw = json.loads(line)
        except json.JSONDecodeError as e:
            with output_lock:
                print(json.dumps({"id": None, "status": "error",
                    "error": {"code": "PARSE_ERROR", "message": str(e)}}))
                sys.stdout.flush()
            continue

        method = raw.get("method", _REQUEST_METHOD_TOOL_CALL)

        if method == _REQUEST_METHOD_SESSION_CREATE:
            response = _handle_session_create(raw, validator, state_store)
            with output_lock:
                print(json.dumps(response, default=str))
                sys.stdout.flush()
            continue

        if method == _REQUEST_METHOD_TOOL_LIST:
            response = _handle_tool_list(raw, registry)
            with output_lock:
                print(json.dumps(response, default=str))
                sys.stdout.flush()
            continue

        context = RequestContext.create(raw)
        _enqueue_timestamp(context)

        try:
            request_queue.put(context, block=True, timeout=5.0)
        except QueueFullError:
            response = {"id": context.request_id, "status": "error",
                "error": {"code": "QUEUE_FULL", "message": "Server busy — rejected by backpressure"}}
            with output_lock:
                print(json.dumps(response))
                sys.stdout.flush()

    # Shutdown sequence
    # Wait briefly for workers to finish processing queued items
    time.sleep(0.3)
    shutdown_event.set()
    collector.join(timeout=5)

    logger.log({"status": "kernel_stopped", "queue_metrics": request_queue.metrics})


def _worker_loop(worker_id, request_queue, result_list, result_lock,
                 shutdown_event, services, state_store):
    """Worker thread loop: pull from queue, run pipeline, push result."""
    while not shutdown_event.is_set():
        try:
            context = request_queue.get(timeout=1.0)
        except QueueEmptyError:
            continue

        if hasattr(context, '_enqueue_time'):
            context.queue_wait_time_ms = round(
                (time.time() - context._enqueue_time) * 1000, 3
            )

        context.timestamp_start = time.time()
        context.worker_id = worker_id

        try:
            context = services.pipeline.process(context)
        except Exception:
            import traceback
            context.finalize(
                status="error",
                error=traceback.format_exc(),
                error_code="WORKER_FATAL",
            )

        # Persist trace
        try:
            state_store.save_trace(context.request_id, {
                "request_id": context.request_id,
                "status": context.status,
                "execution_trace": context.full_trace(),
                "decision_path": context.decision_path,
                "stage_timings": context.stage_timings,
                "worker_id": worker_id,
                "pipeline_mode": context.pipeline_mode,
                "timestamp": time.time(),
            })
        except Exception:
            pass

        with result_lock:
            result_list.append(context)


def _handle_session_create(raw_request, validator, state_store):
    request_id = raw_request.get("id", f"auto_ses")
    params = raw_request.get("params", {})
    project_id = params.get("project_id", "default-project")
    user_id = params.get("user_id", "default")
    session = validator.create_session(project_id, user_id)
    state_store.save_session(session)
    return {"id": request_id, "status": "success", "result": session}


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
    return {"id": request_id, "status": "success",
            "result": {"tools": tools, "count": len(tools), "registry_version": registry.version}}


if __name__ == "__main__":
    main()
