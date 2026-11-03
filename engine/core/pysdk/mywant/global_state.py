"""What wants publish for one another: /api/v1/global-state."""

from __future__ import annotations

from typing import Any

from .client import _q


class GlobalState:
    def __init__(self, client):
        self._c = client

    def get(self) -> dict:
        got = self._c.request("GET", "/api/v1/global-state")
        if isinstance(got, dict) and isinstance(got.get("state"), dict):
            return got["state"]
        return got or {}

    def set(self, key: str, value: Any) -> None:
        self._c.request("PUT", f"/api/v1/global-state/{_q(key)}", value)

    def delete(self, key: str) -> None:
        self._c.request("DELETE", f"/api/v1/global-state/{_q(key)}")

    def clear(self) -> None:
        self._c.request("DELETE", "/api/v1/global-state")
