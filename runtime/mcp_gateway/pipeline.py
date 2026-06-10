"""AI Workstation — Deterministic Control Plane Pipeline (v0.4.0)

Structured middleware pipeline with dual mode support.
Each stage is explicit, traceable, and independently measurable.

Pipeline modes:
  STRICT_MODE     — full 7-stage pipeline
  OPTIMIZED_MODE  — reduced stages (skip audit_log, simplified post)

v0.4.0 additions:
  - Dual mode: selectable per-request via pipeline_mode field
  - Composable policy evaluation graph tracking
  - Latency breakdown: queue, routing, execution, audit, total
"""

from __future__ import annotations

import time
from datetime import datetime, timezone

from mcp_gateway.context import RequestContext, StageResult, ErrorEnvelope, PIPELINE_STRICT, PIPELINE_OPTIMIZED
from tools.registry import ToolNotFoundError


class Pipeline:
    """Dual-mode deterministic control plane pipeline."""

    STRICT_STAGES = [
        "pre_validation",
        "session_guard",
        "capability_routing",
        "pre_execution",
        "execution",
        "post_validation",
        "audit_log",
    ]

    OPTIMIZED_STAGES = [
        "pre_validation",
        "session_guard",
        "capability_routing",
        "pre_execution",
        "execution",
        "post_validation",
    ]

    def __init__(self, services):
        self._services = services
        self._stages = {}

        self._stages["pre_validation"] = self._stage_pre_validation
        self._stages["session_guard"] = self._stage_session_guard
        self._stages["capability_routing"] = self._stage_capability_routing
        self._stages["pre_execution"] = self._stage_pre_execution
        self._stages["execution"] = self._stage_execution
        self._stages["post_validation"] = self._stage_post_validation
        self._stages["audit_log"] = self._stage_audit_log

    def process(self, context: RequestContext) -> RequestContext:
        """Run pipeline in the mode specified by context.pipeline_mode.

        STRICT_MODE:    7 stages (full audit)
        OPTIMIZED_MODE: 6 stages (skip audit_log)
        """
        stage_order = (
            self.OPTIMIZED_STAGES if context.pipeline_mode == PIPELINE_OPTIMIZED
            else self.STRICT_STAGES
        )

        for stage_name in stage_order:
            handler = self._stages.get(stage_name)
            if handler is None:
                continue

            result = handler(context, self._services)
            context.add_trace(result)

            if result.decision == "deny":
                context.finalize(
                    status="denied",
                    error=result.error or f"Pipeline denied at stage: {stage_name}",
                    error_code=f"DENIED_AT_{stage_name.upper()}",
                )
                self._finalize_latency(context)
                self._run_audit_log(context, self._services)
                return context

        context.finalize(
            status=context.status if context.status != "pending" else "success",
            result=context.result,
            error=context.error,
            error_code=context.error_code,
        )
        self._finalize_latency(context)
        self._run_audit_log(context, self._services)
        return context

    def _finalize_latency(self, context):
        """Compute latency breakdown after pipeline execution."""
        bd = {}
        bd["total_ms"] = round(sum(context.stage_timings.values()), 3) if context.stage_timings else 0
        bd["queue_wait_ms"] = round(context.queue_wait_time_ms, 3)
        bd["routing_ms"] = context.stage_timings.get("capability_routing", 0)
        bd["execution_ms"] = context.stage_timings.get("execution", 0)
        bd["audit_ms"] = context.stage_timings.get("audit_log", 0)
        bd["validation_ms"] = (
            context.stage_timings.get("pre_validation", 0)
            + context.stage_timings.get("post_validation", 0)
        )
        context.latency_breakdown = bd

    def _run_audit_log(self, context, services):
        """Always run audit log stage, even for OPTIMIZED and denied paths."""
        try:
            result = self._stage_audit_log(context, services)
            context.add_trace(result)
        except Exception:
            pass

    @staticmethod
    def _stage_pre_validation(context, services):
        t0 = time.time()
        stage_name = "pre_validation"

        verdict = services.policy_engine.evaluate_stage(stage_name, context)
        if verdict.decision == "deny":
            return StageResult(
                stage=stage_name, decision="deny",
                duration_ms=(time.time() - t0) * 1000,
                error=verdict.reason,
                detail={"policy_id": verdict.policy_id},
            )

        return StageResult(
            stage=stage_name, decision="allow",
            duration_ms=(time.time() - t0) * 1000,
            detail={"fields_present": bool(context.request_id and context.session_id and context.project_id)},
        )

    @staticmethod
    def _stage_session_guard(context, services):
        t0 = time.time()
        tool_def = services.get_tool_def(context.tool_id)
        return services.session_guard.execute(context, tool_def)

    @staticmethod
    def _stage_capability_routing(context, services):
        t0 = time.time()
        stage_name = "capability_routing"

        if context.tool_id:
            try:
                tool_def = services.registry.get_tool(context.tool_id)
                context.capability = context.capability or (
                    tool_def.get("capabilities", [None])[0]
                )
                return StageResult(
                    stage=stage_name, decision="allow",
                    duration_ms=(time.time() - t0) * 1000,
                    detail={"tool_id": context.tool_id, "routing": "direct"},
                )
            except ToolNotFoundError:
                return StageResult(
                    stage=stage_name, decision="deny",
                    duration_ms=(time.time() - t0) * 1000,
                    error=f"Tool not found: {context.tool_id}",
                )

        if context.capability:
            tool = services.registry.find_first_by_capability(context.capability)
            if tool:
                context.tool_id = tool["id"]
                return StageResult(
                    stage=stage_name, decision="allow",
                    duration_ms=(time.time() - t0) * 1000,
                    detail={
                        "tool_id": context.tool_id,
                        "capability": context.capability,
                        "routing": "capability_lookup",
                    },
                )
            return StageResult(
                stage=stage_name, decision="deny",
                duration_ms=(time.time() - t0) * 1000,
                error=f"No tool for capability: {context.capability}",
            )

        return StageResult(
            stage=stage_name, decision="deny",
            duration_ms=(time.time() - t0) * 1000,
            error="Missing tool or capability in request",
        )

    @staticmethod
    def _stage_pre_execution(context, services):
        t0 = time.time()
        stage_name = "pre_execution"

        tool_def = services.get_tool_def(context.tool_id)
        verdict = services.policy_engine.evaluate_stage(stage_name, context, tool_def)
        if verdict.decision == "deny":
            return StageResult(
                stage=stage_name, decision="deny",
                duration_ms=(time.time() - t0) * 1000,
                error=verdict.reason,
                detail={"policy_id": verdict.policy_id},
            )

        return StageResult(
            stage=stage_name, decision="allow",
            duration_ms=(time.time() - t0) * 1000,
            detail={"tool_id": context.tool_id},
        )

    @staticmethod
    def _stage_execution(context, services):
        t0 = time.time()
        stage_name = "execution"

        tool_def = services.get_tool_def(context.tool_id)
        if tool_def is None:
            return StageResult(
                stage=stage_name, decision="deny",
                duration_ms=(time.time() - t0) * 1000,
                error=f"Cannot execute unknown tool: {context.tool_id}",
            )

        session_context = {
            "session_id": context.session_id,
            "project_id": context.project_id,
        }

        timeout_ms = tool_def.get("governance", {}).get("timeout_ms",
            services.policy_engine.evaluate_stage(stage_name, context, tool_def).detail.get("timeout_ms", 30000))

        try:
            exec_result = services.executor.execute_isolated(
                tool_id=context.tool_id,
                arguments=context.arguments,
                session_context=session_context,
                timeout_ms=timeout_ms,
            )
        except Exception as e:
            import traceback
            return StageResult(
                stage=stage_name, decision="deny",
                duration_ms=(time.time() - t0) * 1000,
                error=str(e),
                detail={"error_trace": traceback.format_exc()},
            )

        exec_status = exec_result.get("status", "error")
        if exec_status == "error":
            context.status = "error"
            context.error = exec_result.get("error", "Unknown error")
            context.error_code = exec_result.get("error_code", "EXECUTION_ERROR")
            return StageResult(
                stage=stage_name, decision="deny",
                duration_ms=exec_result.get("duration_ms", (time.time() - t0) * 1000),
                error=context.error,
                detail={
                    "error_code": context.error_code,
                    "tool_id": context.tool_id,
                },
            )

        context.status = "success"
        context.result = exec_result.get("result")
        return StageResult(
            stage=stage_name, decision="allow",
            duration_ms=exec_result.get("duration_ms", (time.time() - t0) * 1000),
            detail={"tool_id": context.tool_id, "result_type": type(context.result).__name__},
        )

    @staticmethod
    def _stage_post_validation(context, services):
        t0 = time.time()
        stage_name = "post_validation"

        if context.status == "error":
            verdict = services.policy_engine.evaluate_stage(stage_name, context)
            if verdict.decision == "deny":
                return StageResult(
                    stage=stage_name, decision="deny",
                    duration_ms=(time.time() - t0) * 1000,
                    error="Execution error contained — cascade prevented",
                    detail={"policy_id": verdict.policy_id},
                )

        return StageResult(
            stage=stage_name, decision="allow",
            duration_ms=(time.time() - t0) * 1000,
            detail={"status": context.status},
        )

    @staticmethod
    def _stage_audit_log(context, services):
        t0 = time.time()
        stage_name = "audit_log"

        log_entry = {
            "request_id": context.request_id,
            "session_id": context.session_id,
            "project_id": context.project_id,
            "tool_id": context.tool_id,
            "capability": context.capability,
            "status": context.status,
            "decision_path": context.decision_path,
            "stage_timings": context.stage_timings,
            "total_ms": round(sum(context.stage_timings.values()), 3) if context.stage_timings else 0,
            "error": context.error,
            "error_code": context.error_code,
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "execution_trace": context.full_trace(),
            "queue_wait_time_ms": round(context.queue_wait_time_ms, 3),
            "worker_id": context.worker_id,
            "pipeline_mode": context.pipeline_mode,
            "latency_breakdown": context.latency_breakdown,
            "policy_decision_graph": context.policy_decision_graph,
        }

        try:
            services.logger.log(log_entry)
        except Exception:
            pass

        return StageResult(
            stage=stage_name, decision="allow",
            duration_ms=(time.time() - t0) * 1000,
            detail={"logged": True},
        )


class PipelineServices:
    """Container for all services the pipeline depends on."""

    def __init__(self, registry, executor, session_guard, policy_engine, logger, pipeline=None):
        self.registry = registry
        self.executor = executor
        self.session_guard = session_guard
        self.policy_engine = policy_engine
        self.logger = logger
        self.pipeline = pipeline

    def get_tool_def(self, tool_id):
        if not tool_id:
            return None
        try:
            return self.registry.get_tool(tool_id)
        except ToolNotFoundError:
            return None
