"""Where each character stands: /api/v1/cursors."""

from __future__ import annotations

from typing import Any, Mapping

from .client import _q


class Cursors:
    def __init__(self, client):
        self._c = client

    def list(self) -> Any:
        return self._c.request("GET", "/api/v1/cursors")

    def put(self, character_id: str, cursor: Mapping[str, Any]) -> Any:
        return self._c.request("PUT", f"/api/v1/cursors/{_q(character_id)}", dict(cursor))

    def delete(self, character_id: str) -> None:
        self._c.request("DELETE", f"/api/v1/cursors/{_q(character_id)}")
