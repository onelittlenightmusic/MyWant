"""Webhook endpoints wants listen on: /api/v1/webhooks."""

from __future__ import annotations

from typing import Any, Mapping

from .client import _q


class Webhooks:
    def __init__(self, client):
        self._c = client

    def list(self) -> Any:
        return self._c.request("GET", "/api/v1/webhooks")

    def send(self, id: str, payload: Mapping[str, Any]) -> Any:
        """Deliver a payload to one, as its caller would."""
        return self._c.request("POST", f"/api/v1/webhooks/{_q(id)}", dict(payload))
