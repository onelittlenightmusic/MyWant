"""Characters — who moves on the board: /api/v1/characters."""

from __future__ import annotations

from typing import Any, Iterable, Optional, Union

from .client import _csv, _q, _send


class Characters:
    def __init__(self, client):
        self._c = client

    def list(self) -> list:
        got = self._c.request("GET", "/api/v1/characters")
        return got.get("characters") or [] if isinstance(got, dict) else got or []

    def get(self, id: str) -> dict:
        return self._c.request("GET", f"/api/v1/characters/{_q(id)}")

    def create(self, name: str, *, avatar: str = "", color: str = "", shape: str = "") -> dict:
        body = {"name": name, "avatar": avatar, "color": color}
        if shape:
            body["shape"] = shape
        return self._c.request("POST", "/api/v1/characters", body)

    def update(self, id: str, **fields: Any) -> dict:
        """Who they are: name, avatar, color, shape (their display and labels have their own calls)."""
        return self._c.request("PUT", f"/api/v1/characters/{_q(id)}", fields)

    def delete(self, *ids: str) -> Any:
        """One, or several in one request."""
        if len(ids) == 1:
            return self._c.request("DELETE", f"/api/v1/characters/{_q(ids[0])}")
        return self._c.request("DELETE", "/api/v1/characters", query={"ids": _csv(ids)})

    def set_label(self, id: str, key: str, value: str) -> dict:
        return self._c.request("POST", f"/api/v1/characters/{_q(id)}/labels", {"key": key, "value": value})

    def remove_label(self, id: str, key: str) -> dict:
        return self._c.request("DELETE", f"/api/v1/characters/{_q(id)}/labels/{_q(key)}")

    def export(self, ids: Optional[Iterable[str]] = None) -> str:
        """As YAML; all, or these."""
        query = {"ids": _csv(ids)} if ids else None
        return self._c.request("POST", "/api/v1/characters/export", query=query, text=True)

    def import_(self, data: Union[str, list]) -> list:
        """One of an id already here is updated; any other is added. Their ids."""
        return (_send(self._c, "POST", "/api/v1/characters/import", data) or {}).get("imported") or []
