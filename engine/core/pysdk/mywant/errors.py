"""Errors the server kept: /api/v1/errors."""

from __future__ import annotations

from typing import Any

from .client import _q


class Errors:
    def __init__(self, client):
        self._c = client

    def list(self) -> Any:
        return self._c.request("GET", "/api/v1/errors")

    def get(self, id: str) -> dict:
        return self._c.request("GET", f"/api/v1/errors/{_q(id)}")

    def resolve(self, id: str) -> dict:
        """Mark it resolved."""
        return self._c.request("PUT", f"/api/v1/errors/{_q(id)}", {})

    def delete(self, id: str) -> None:
        self._c.request("DELETE", f"/api/v1/errors/{_q(id)}")
