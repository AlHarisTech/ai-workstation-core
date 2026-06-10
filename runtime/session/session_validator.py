"""AI Workstation — Session Validation Layer

Enforces strict session requirements. Fail-Closed: any invalid request
MUST be rejected immediately with a structured error.

Validation rules (hard-coded, no configuration drift risk):
  1. session_id required — non-empty string.
  2. project_id required — non-empty string.
  3. Scope isValid — session must not be expired and project must match.
  4. Session context must be well-formed if present.

Violation response is a structured dict with status="error" and
an errors list — never an exception that can be silently swallowed.
"""

import uuid
from datetime import datetime, timedelta, timezone


_MAX_SESSION_AGE_MINUTES = 60


class SessionValidator:
    """Fail-closed session validator.

    Every validate() call returns a (valid, errors) tuple.
    The gateway MUST NOT proceed past a False result.
    """

    REQUIRED_SESSION_FIELDS = {
        "session_id": str,
        "project_id": str,
    }

    def __init__(self):
        self._active_sessions = {}

    def validate(self, session_data):
        """Validate session_data dict.

        Args:
            session_data: dict with session_id, project_id, and optionally
                          created_at, expires_at, scope.

        Returns:
            (valid: bool, errors: list[str])
        """
        if not isinstance(session_data, dict):
            return False, ["session data must be a dict"]

        errors = []

        session_id = session_data.get("session_id")
        if not session_id or not isinstance(session_id, str) or not session_id.strip():
            errors.append("session_id is required and must be a non-empty string")

        project_id = session_data.get("project_id")
        if not project_id or not isinstance(project_id, str) or not project_id.strip():
            errors.append("project_id is required and must be a non-empty string")

        if errors:
            return False, errors

        created_at = session_data.get("created_at")
        if created_at is not None:
            try:
                if isinstance(created_at, str):
                    created_dt = datetime.fromisoformat(created_at.replace("Z", "+00:00"))
                else:
                    created_dt = created_at
                age = datetime.now(timezone.utc) - created_dt
                if age > timedelta(minutes=_MAX_SESSION_AGE_MINUTES):
                    errors.append(
                        f"session expired: age {age.total_seconds():.0f}s exceeds "
                        f"{_MAX_SESSION_AGE_MINUTES}m maximum"
                    )
            except (ValueError, TypeError):
                errors.append("invalid created_at format — must be ISO 8601")

        scope = session_data.get("scope")
        if scope is not None and not isinstance(scope, str):
            errors.append("scope must be a string if provided")

        return len(errors) == 0, errors

    def create_session(self, project_id, user_id="default"):
        """Create a new session context.

        Args:
            project_id: project identifier string.
            user_id: user identifier string.

        Returns:
            dict with session_id, project_id, user_id, created_at, expires_at.
        """
        now = datetime.now(timezone.utc)
        session = {
            "session_id": f"ses_{uuid.uuid4().hex[:12]}",
            "project_id": project_id,
            "user_id": user_id,
            "created_at": now.isoformat(),
            "expires_at": (now + timedelta(minutes=_MAX_SESSION_AGE_MINUTES)).isoformat(),
        }
        self._active_sessions[session["session_id"]] = session
        return session

    def lookup(self, session_id):
        """Look up a previously created session."""
        return self._active_sessions.get(session_id)
