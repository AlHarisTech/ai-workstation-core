"""AI Workstation — Deterministic Control Plane Pipeline (v0.3.0)

Replaces linear execution with a structured middleware pipeline.
Each stage is explicit, traceable, and independently measurable.

Pipeline stages (in execution order):
  1. PreValidation   — mandatory field presence, JSON validity
  2. SessionGuard     — session validation (fail-closed)
  3. CapabilityRouting — tool_id resolution from registry
  4. PreExecution     — governance policy checks, path deny-list
  5. Execution        — tool dispatch with timeout isolation
  6. PostValidation   — error isolation, result sanitization
  7. AuditLog         — structured log emission

Every stage returns a StageResult. DENY stops the pipeline.
The pipeline is the SINGLE path for all tool.call requests.
"""

from __future__ import annotations

import time
from datetime import datetime, timezone

from mcp_gateway.context import RequestContext, StageResult, ErrorEnvelope
from tools.registry import ToolNotFoundError


class Pipeline:
    """Deterministic control plane pipeline.

    Executes middleware stages in strict order. Each stage receives
    the accumulating RequestContext and produces a StageResult.

    Stages can be added via `add_stage()`. The pipeline evaluates
    each stage in registration order. First DENY stops execution
    immediately (fail-closed).
    """

    def __init__(self, services):
        """Initialize pipeline with service dependencies.

        Args:
            services: PipelineServices with registry, executor, etc.
        """
        self._services = services
        self._stages = []
        self._register_default_stages()

    def _register_default_stages(self):
        """Register the 7 standard pipeline stages in order."""
        self.add_stage("pre_validation", self._stage_pre_validation)
        self.add_stage("session_guard", self._stage_session_guard)
        self.add_stage("capability_routing", self._stage_capability_routing)
        self.add_stage("pre_execution", self._stage_pre_execution)
        self.add_stage("execution", self._stage_execution)
        self.add_stage("post_validation", self._stage_post_validation)
        self.add_stage("audit_log", self._stage_audit_log)

    def add_stage(self, name, handler):
        self._stages.append((name, handler))

    def process(self, context: RequestContext) -> RequestContext:
        """Run the full pipeline on a RequestContext.

        Args:
            context: initial RequestContext with mandatory fields set

        Returns:
            RequestContext with execution_trace populated, status finalized
        """
        for stage_name, handler in self._stages:
            result = handler(context, self._services)
            context.add_trace(result)

            if result.decision == "deny":
                context.finalize(
                    status="denied",
                    error=result.error or f"Pipeline denied at stage: {stage_name}",
                    error_code=f"DENIED_AT_{stage_name.upper()}",
                )
                return context

        context.finalize(
            status=context.status if context.status != "pending" else "success",
            result=context.result,
            error=context.error,
            error_code=context.error_code,
        )
        return context

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

    def __init__(self, registry, executor, session_guard, policy_engine, logger):
        self.registry = registry
        self.executor = executor
        self.session_guard = session_guard
        self.policy_engine = policy_engine
        self.logger = logger

    def get_tool_def(self, tool_id):
        if not tool_id:
            return None
        try:
            return self.registry.get_tool(tool_id)
        except ToolNotFoundError:
            return None
