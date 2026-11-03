"""Reaction queues (a user's answer to a want): /api/v1/reactions."""

from __future__ import annotations

from typing import Any

from .client import _q


class Reactions:
    def __init__(self, client):
        self._c = client

    def list(self) -> Any:
        return self._c.request("GET", "/api/v1/reactions/")

    def create(self) -> dict:
        return self._c.request("POST", "/api/v1/reactions/")

    def get(self, id: str) -> dict:
        return self._c.request("GET", f"/api/v1/reactions/{_q(id)}")

    def add(self, id: str, approved: bool, comment: str = "") -> dict:
        """Answer the want waiting on this queue."""
        return self._c.request("PUT", f"/api/v1/reactions/{_q(id)}", {"approved": approved, "comment": comment})

    def delete(self, id: str) -> None:
        self._c.request("DELETE", f"/api/v1/reactions/{_q(id)}")
