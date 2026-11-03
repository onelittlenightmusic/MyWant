"""The server's log: /api/v1/logs."""

from __future__ import annotations


class Logs:
    def __init__(self, client):
        self._c = client

    def get(self) -> str:
        return self._c.request("GET", "/api/v1/logs", text=True)

    def clear(self) -> None:
        self._c.request("DELETE", "/api/v1/logs")
