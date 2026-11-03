"""The server itself: /health, /api/v1/config, /api/v1/system, /api/v1/spec."""

from __future__ import annotations

from typing import Any, Mapping


class System:
    def __init__(self, client):
        self._c = client

    def health(self) -> dict:
        return self._c.request("GET", "/health")

    def config(self) -> dict:
        return self._c.request("GET", "/api/v1/config")

    def update_config(self, config: Mapping[str, Any]) -> dict:
        """Replace the config (PUT)."""
        return self._c.request("PUT", "/api/v1/config", dict(config))

    def patch_config(self, changes: Mapping[str, Any]) -> dict:
        """Change only these keys."""
        return self._c.request("PATCH", "/api/v1/config", dict(changes))

    def spec(self) -> str:
        """The OpenAPI spec, as YAML."""
        return self._c.request("GET", "/api/v1/spec", text=True)

    def stop(self) -> None:
        self._c.request("POST", "/api/v1/system/stop")

    def restart(self) -> None:
        self._c.request("POST", "/api/v1/system/restart")
