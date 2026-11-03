"""The labels in use: /api/v1/labels."""

from __future__ import annotations

from typing import Any


class Labels:
    def __init__(self, client):
        self._c = client

    def list(self) -> Any:
        return self._c.request("GET", "/api/v1/labels")

    def add(self, key: str, value: str) -> Any:
        return self._c.request("POST", "/api/v1/labels", {"key": key, "value": value})
