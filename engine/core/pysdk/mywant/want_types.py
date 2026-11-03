"""Want types: /api/v1/want-types."""

from __future__ import annotations

from typing import Any, Mapping, Union

from .client import _q, _send


class WantTypes:
    def __init__(self, client):
        self._c = client

    def list(self) -> list:
        got = self._c.request("GET", "/api/v1/want-types")
        return got.get("wantTypes") or [] if isinstance(got, dict) else got or []

    def get(self, name: str) -> dict:
        return self._c.request("GET", f"/api/v1/want-types/{_q(name)}")

    def examples(self, name: str) -> Any:
        return self._c.request("GET", f"/api/v1/want-types/{_q(name)}/examples")

    def create(self, definition: Union[str, Mapping[str, Any]]) -> Any:
        return _send(self._c, "POST", "/api/v1/want-types", definition)

    def update(self, name: str, definition: Union[str, Mapping[str, Any]]) -> Any:
        return _send(self._c, "PUT", f"/api/v1/want-types/{_q(name)}", definition)

    def delete(self, name: str) -> None:
        self._c.request("DELETE", f"/api/v1/want-types/{_q(name)}")

    def reload(self) -> Any:
        """Read the type definitions from disk again."""
        return self._c.request("POST", "/api/v1/want-types/reload")
