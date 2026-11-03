"""Agents: /api/v1/agents."""

from __future__ import annotations

from typing import Any, Iterable

from .client import _q


class Agents:
    def __init__(self, client):
        self._c = client

    def list(self) -> Any:
        return self._c.request("GET", "/api/v1/agents")

    def get(self, name: str) -> dict:
        return self._c.request("GET", f"/api/v1/agents/{_q(name)}")

    def create(self, name: str, type: str, capabilities: Iterable[str]) -> Any:
        return self._c.request("POST", "/api/v1/agents",
                               {"name": name, "type": type, "capabilities": list(capabilities)})

    def delete(self, name: str) -> None:
        self._c.request("DELETE", f"/api/v1/agents/{_q(name)}")

    def register_yaml(self, yaml: str) -> Any:
        """Load an agent from its YAML definition, without a restart."""
        return self._c.request("POST", "/api/v1/agents/register-yaml", yaml=yaml)
