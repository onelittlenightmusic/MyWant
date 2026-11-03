"""Capabilities: /api/v1/capabilities."""

from __future__ import annotations

from typing import Any, Iterable

from .client import _q


class Capabilities:
    def __init__(self, client):
        self._c = client

    def list(self) -> Any:
        return self._c.request("GET", "/api/v1/capabilities")

    def get(self, name: str) -> dict:
        return self._c.request("GET", f"/api/v1/capabilities/{_q(name)}")

    def create(self, name: str, gives: Iterable[str]) -> Any:
        return self._c.request("POST", "/api/v1/capabilities", {"name": name, "gives": list(gives)})

    def delete(self, name: str) -> None:
        self._c.request("DELETE", f"/api/v1/capabilities/{_q(name)}")

    def agents(self, name: str) -> Any:
        """The agents that give it."""
        return self._c.request("GET", f"/api/v1/capabilities/{_q(name)}/agents")
