"""AI Workstation — Governance Enforcement Engine (v0.3.0)

Converts governance from documentation to runtime logic.
Evaluates policies BEFORE execution. Deny verdicts stop the pipeline.

Architecture:
  PolicyEngine loads policies from YAML at startup.
  For each pipeline stage, relevant policies are evaluated against
  the current RequestContext and tool definition.
  Each policy returns a PolicyVerdict: ALLOW or DENY.
  DENY is terminal — no further evaluation.
"""

from __future__ import annotations

import os
import yaml
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional


_POLICY_YAML_RELATIVE = os.path.join(
    os.path.dirname(os.path.abspath(__file__)),
    "policies", "runtime.yaml",
)

_POLICY_YAML_FALLBACK = os.path.join(
    os.environ.get("AI_WORKSTATION_ROOT", os.getcwd()),
    "runtime", "governance", "policies", "runtime.yaml",
)


class PolicyLoadError(Exception):
    """Raised when governance policies cannot be loaded."""


@dataclass
class PolicyVerdict:
    """Result of a single policy evaluation."""

    policy_id: str
    policy_name: str
    decision: str  # "allow" | "deny"
    reason: str = ""
    detail: Dict[str, Any] = field(default_factory=dict)


class PolicyEngine:
    """Runtime governance enforcement engine.

    Usage:
        engine = PolicyEngine()
        engine.load()
        verdict = engine.evaluate_stage("pre_validation", context, tool_def)
        if verdict.decision == "deny":
            return deny_response(verdict)
    """

    SUPPORTED_RULE_TYPES = {
        "field_presence",
        "tool_exists",
        "session_required",
        "path_deny_list",
        "timeout",
        "error_isolation",
    }

    def __init__(self, yaml_path=None):
        self._yaml_path = yaml_path or self._resolve_yaml_path()
        self._policies: List[Dict] = []
        self._enforcement = "fail_closed"
        self._version = "0.0.0"

    @staticmethod
    def _resolve_yaml_path():
        for candidate in (_POLICY_YAML_RELATIVE, _POLICY_YAML_FALLBACK):
            if os.path.isfile(candidate):
                return candidate
        raise PolicyLoadError(
            f"Governance policy file not found at {_POLICY_YAML_RELATIVE} "
            f"or {_POLICY_YAML_FALLBACK}"
        )

    def load(self):
        """Load governance policies from YAML."""
        if not os.path.isfile(self._yaml_path):
            raise PolicyLoadError(f"Policy file not found: {self._yaml_path}")

        with open(self._yaml_path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)

        if not data or "governance" not in data:
            raise PolicyLoadError("Invalid policy YAML: missing 'governance' key")

        gov = data["governance"]
        self._version = gov.get("version", "0.0.0")
        self._enforcement = gov.get("enforcement", "fail_closed")
        self._policies = gov.get("policies", [])

        if not isinstance(self._policies, list):
            raise PolicyLoadError("Invalid policy YAML: 'policies' must be a list")

        return len(self._policies)

    def evaluate_stage(self, stage_name: str, context, tool_def=None) -> PolicyVerdict:
        """Evaluate all policies relevant to the given pipeline stage.

        Policies are evaluated in priority order. First DENY stops evaluation.

        Args:
            stage_name: pipeline stage name (e.g. "pre_validation", "session_guard")
            context: RequestContext instance
            tool_def: tool definition dict from registry (optional)

        Returns:
            PolicyVerdict with decision "allow" or "deny"
        """
        stage_policies = [
            p for p in self._policies
            if p.get("stage") == stage_name
        ]
        stage_policies.sort(key=lambda p: p.get("priority", 99))

        for policy in stage_policies:
            verdict = self._evaluate_policy(policy, context, tool_def)
            if verdict.decision == "deny":
                return verdict

        return PolicyVerdict(
            policy_id="-",
            policy_name="default-allow",
            decision="allow",
            reason=f"No policies deny execution at stage '{stage_name}'",
        )

    def _evaluate_policy(self, policy: dict, context, tool_def=None) -> PolicyVerdict:
        """Evaluate a single policy against the current context."""
        pid = policy.get("id", "unknown")
        pname = policy.get("name", "unknown")
        rule = policy.get("rule", {})
        rule_type = rule.get("type", "")

        if rule_type not in self.SUPPORTED_RULE_TYPES:
            return PolicyVerdict(pid, pname, "deny",
                                 f"Unknown rule type: {rule_type}")

        handler = getattr(self, f"_rule_{rule_type}", None)
        if handler is None:
            return PolicyVerdict(pid, pname, "deny",
                                 f"No handler for rule type: {rule_type}")

        return handler(policy, rule, context, tool_def)

    def _rule_field_presence(self, policy, rule, context, tool_def):
        pid = policy["id"]
        pname = policy["name"]
        required = rule.get("required_fields", [])
        missing = []
        for field in required:
            value = getattr(context, field, None)
            if not value:
                missing.append(field)
        if missing:
            return PolicyVerdict(
                pid, pname, "deny",
                reason=f"Missing mandatory fields: {missing}",
                detail={"missing_fields": missing},
            )
        return PolicyVerdict(pid, pname, "allow", reason="All mandatory fields present")

    def _rule_tool_exists(self, policy, rule, context, tool_def):
        pid = policy["id"]
        pname = policy["name"]
        if tool_def is None:
            return PolicyVerdict(
                pid, pname, "deny",
                reason=f"Tool not found: {context.tool_id}",
            )
        return PolicyVerdict(pid, pname, "allow", reason=f"Tool exists: {context.tool_id}")

    def _rule_session_required(self, policy, rule, context, tool_def):
        pid = policy["id"]
        pname = policy["name"]
        if tool_def is None:
            return PolicyVerdict(pid, pname, "allow", reason="No tool definition")
        requires = tool_def.get("governance", {}).get("require_session", True)
        if requires:
            if not context.session_id or not context.project_id:
                return PolicyVerdict(
                    pid, pname, "deny",
                    reason="Tool requires valid session but session_id/project_id missing",
                )
        return PolicyVerdict(pid, pname, "allow", reason="Session check passed")

    def _rule_path_deny_list(self, policy, rule, context, tool_def):
        pid = policy["id"]
        pname = policy["name"]
        path = context.arguments.get("path", "")
        if not path:
            return PolicyVerdict(pid, pname, "allow", reason="No path argument")

        import os as _os
        expanded = _os.path.abspath(_os.path.expanduser(str(path)))

        for prefix in rule.get("deny_prefixes", []):
            if expanded.startswith(prefix):
                return PolicyVerdict(
                    pid, pname, "deny",
                    reason=f"Path in deny list: {path}",
                    detail={"path": path, "matched_prefix": prefix},
                )

        for segment in rule.get("deny_segments", []):
            if segment in expanded:
                return PolicyVerdict(
                    pid, pname, "deny",
                    reason=f"Path contains denied segment: {segment}",
                    detail={"path": path, "matched_segment": segment},
                )

        return PolicyVerdict(pid, pname, "allow", reason="Path allowed")

    def _rule_timeout(self, policy, rule, context, tool_def):
        pid = policy["id"]
        pname = policy["name"]
        timeout_ms = rule.get("default_timeout_ms", 30000)
        return PolicyVerdict(
            pid, pname, "allow",
            reason=f"Timeout boundary: {timeout_ms}ms",
            detail={"timeout_ms": timeout_ms},
        )

    def _rule_error_isolation(self, policy, rule, context, tool_def):
        pid = policy["id"]
        pname = policy["name"]
        if context.status == "error":
            if context.error_code == "EXECUTION_TIMEOUT":
                return PolicyVerdict(
                    pid, pname, "deny",
                    reason="Tool execution timed out — cascade prevented",
                    detail={"error_code": context.error_code},
                )
            elif context.error_code == "EXECUTION_ERROR":
                return PolicyVerdict(
                    pid, pname, "deny",
                    reason="Tool execution error — cascade prevented",
                    detail={"error_code": context.error_code},
                )
        return PolicyVerdict(pid, pname, "allow", reason="No isolation violation")

    @property
    def policy_count(self):
        return len(self._policies)

    @property
    def version(self):
        return self._version

    @property
    def enforcement_mode(self):
        return self._enforcement
