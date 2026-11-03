"""A want's state: /api/v1/states."""

from __future__ import annotations

from typing import Any

from .client import _q


class States:
    def __init__(self, client):
        self._c = client

    def get(self, id: str) -> dict:
        return self._c.request("GET", f"/api/v1/states/{_q(id)}")

    def set(self, id: str, key: str, value: Any) -> None:
        self._c.request("PUT", f"/api/v1/states/{_q(id)}/{_q(key)}", value)

    def delete(self, id: str, key: str) -> None:
        self._c.request("DELETE", f"/api/v1/states/{_q(id)}/{_q(key)}")
