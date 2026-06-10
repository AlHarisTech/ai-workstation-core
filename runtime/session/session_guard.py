"""AI Workstation — SessionGuard Pipeline Stage (v0.3.0)

Session validation middleware. Runs after CapabilityRouting.
Enforces: tools marked require_session MUST have a valid session.
Fail-closed: invalid session → immediate deny, pipeline stops.
"""

from __future__ import annotations

import time
from mcp_gateway.context import StageResult


class SessionGuard:
    """Pipeline stage: session validation.

    Evaluates governance policy POL-003 (session-gate).
    Uses SessionValidator for actual validation logic.
    """

    STAGE_NAME = "session_guard"

    def __init__(self, session_validator, policy_engine):
        self._validator = session_validator
        self._policy_engine = policy_engine

    def execute(self, context, tool_def=None) -> StageResult:
        t0 = time.time()

        verdict = self._policy_engine.evaluate_stage(self.STAGE_NAME, context, tool_def)
        if verdict.decision == "deny":
            return StageResult(
                stage=self.STAGE_NAME,
                decision="deny",
                duration_ms=(time.time() - t0) * 1000,
                error=verdict.reason,
                detail={
                    "policy_id": verdict.policy_id,
                    "policy_name": verdict.policy_name,
                },
            )

        requires_session = tool_def.get("governance", {}).get("require_session", True) if tool_def else True
        if not requires_session:
            return StageResult(
                stage=self.STAGE_NAME,
                decision="allow",
                duration_ms=(time.time() - t0) * 1000,
                detail={"reason": "Tool does not require session"},
            )

        session_data = {
            "session_id": context.session_id,
            "project_id": context.project_id,
        }
        valid, errors = self._validator.validate(session_data)

        if not valid:
            return StageResult(
                stage=self.STAGE_NAME,
                decision="deny",
                duration_ms=(time.time() - t0) * 1000,
                error="; ".join(errors),
                detail={"validation_errors": errors},
            )

        return StageResult(
            stage=self.STAGE_NAME,
            decision="allow",
            duration_ms=(time.time() - t0) * 1000,
            detail={"session_id": context.session_id},
        )
