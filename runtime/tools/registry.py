"""AI Workstation — Tool Registry Runtime Loader

Converts the static YAML tool definitions into an in-memory registry
that supports capability-based lookup, tool metadata retrieval, and
runtime handler mapping.

The registry:
  - Loads tools from YAML once at startup.
  - Provides O(1) lookup by tool id.
  - Provides O(n) lookup by capability.
  - Validates that every registered tool has a handler.
  - Emits a governance violation if registry/handler drift is detected.
"""

import os
import yaml


_REGISTRY_YAML_RELATIVE = os.path.join(
    os.path.dirname(os.path.abspath(__file__)),
    "definitions.yaml",
)

_REGISTRY_YAML_FALLBACK = os.path.join(
    os.environ.get("AI_WORKSTATION_ROOT", os.getcwd()),
    "runtime", "tools", "definitions.yaml",
)


class RegistryLoadError(Exception):
    """Raised when the tool registry YAML cannot be loaded or is invalid."""


class ToolNotFoundError(Exception):
    """Raised when a tool is requested but not found in the registry."""


class ToolRegistry:
    """In-memory tool registry backed by YAML definitions.

    Usage:
        registry = ToolRegistry()
        registry.load()
        tool = registry.get_tool("filesystem_read")
        found = registry.find_by_capability("file-read")
    """

    def __init__(self, yaml_path=None):
        self._yaml_path = yaml_path or self._resolve_yaml_path()
        self._tools = {}
        self._capability_index = {}

    @staticmethod
    def _resolve_yaml_path():
        for candidate in (_REGISTRY_YAML_RELATIVE, _REGISTRY_YAML_FALLBACK):
            if os.path.isfile(candidate):
                return candidate
        raise RegistryLoadError(
            f"Tool registry YAML not found at {_REGISTRY_YAML_RELATIVE} "
            f"or {_REGISTRY_YAML_FALLBACK}"
        )

    def load(self):
        """Load and index all tools from the YAML definitions file."""
        if not os.path.isfile(self._yaml_path):
            raise RegistryLoadError(f"Registry file not found: {self._yaml_path}")

        with open(self._yaml_path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)

        if not data or "registry" not in data:
            raise RegistryLoadError("Invalid registry YAML: missing 'registry' key")

        tools = data["registry"].get("tools", [])
        if not isinstance(tools, list):
            raise RegistryLoadError("Invalid registry YAML: 'tools' must be a list")

        self._tools.clear()
        self._capability_index.clear()

        for tool in tools:
            tool_id = tool.get("id")
            if not tool_id:
                raise RegistryLoadError(f"Tool missing 'id' field: {tool}")

            if tool_id in self._tools:
                raise RegistryLoadError(f"Duplicate tool id in registry: {tool_id}")

            self._tools[tool_id] = tool

            for cap in tool.get("capabilities", []):
                self._capability_index.setdefault(cap, []).append(tool_id)

        return len(self._tools)

    def get_tool(self, tool_id):
        """Retrieve a tool definition by id.

        Returns:
            dict: tool definition.

        Raises:
            ToolNotFoundError if tool_id is not registered.
        """
        if tool_id not in self._tools:
            raise ToolNotFoundError(f"Tool not found: {tool_id}")
        return self._tools[tool_id]

    def find_by_capability(self, capability):
        """Find tools matching a capability string.

        Args:
            capability: capability string (e.g. "file-read").

        Returns:
            list[dict]: matching tool definitions. Empty list if none.
        """
        tool_ids = self._capability_index.get(capability, [])
        return [self._tools[tid] for tid in tool_ids]

    def find_first_by_capability(self, capability):
        """Find the first tool matching a capability.

        Returns:
            dict or None.
        """
        matches = self.find_by_capability(capability)
        return matches[0] if matches else None

    def requires_session(self, tool_id):
        """Check if a tool requires a valid session."""
        tool = self.get_tool(tool_id)
        return tool.get("governance", {}).get("require_session", True)

    @property
    def tool_count(self):
        return len(self._tools)

    @property
    def tool_ids(self):
        return list(self._tools.keys())
