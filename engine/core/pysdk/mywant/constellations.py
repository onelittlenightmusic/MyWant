"""Constellations (星座) — things and wants named as one: /api/v1/constellations.

An order is tied one after another; a hub (`capture`) is a capture into one
tile. Schema: want-spec's spec/constellation-spec.yaml.
"""

from __future__ import annotations

from typing import Iterable, Optional, Union

from .client import _csv, _q, _send


class Constellations:
    def __init__(self, client):
        self._c = client

    def list(self, kind: Optional[str] = None) -> list:
        """All, or only "thing" / "want" ones."""
        got = self._c.request("GET", "/api/v1/constellations", query={"kind": kind})
        return (got or {}).get("groups") or []

    def get(self, name: str) -> dict:
        return self._c.request("GET", f"/api/v1/constellations/{_q(name)}")

    def create(self, name: str, members: Iterable[str], *, color: Optional[str] = None,
               capture: Optional[str] = None) -> dict:
        """Members are thing or want ids; `capture` (a member's id) makes it a hub."""
        body: dict = {"name": name, "members": list(members)}
        if color:
            body["color"] = color
        if capture:
            body["capture"] = capture
        return self._c.request("POST", "/api/v1/constellations", body)

    def update(self, name: str, *, new_name: Optional[str] = None, members: Optional[Iterable[str]] = None,
               color: Optional[str] = None, capture: Optional[str] = None) -> dict:
        """Change what is given; members are replaced; `capture=""` makes it an order."""
        body: dict = {}
        for k, v in (("name", new_name), ("color", color), ("capture", capture)):
            if v is not None:
                body[k] = v
        if members is not None:
            body["members"] = list(members)
        return self._c.request("PATCH", f"/api/v1/constellations/{_q(name)}", body)

    def delete(self, *names: str) -> dict:
        """One, or several in one request. Their members are kept."""
        if len(names) == 1:
            return self._c.request("DELETE", f"/api/v1/constellations/{_q(names[0])}")
        return self._c.request("DELETE", "/api/v1/constellations", query={"names": _csv(names)})

    def export(self, names: Optional[Iterable[str]] = None) -> str:
        """As YAML; all, or these."""
        query = {"names": _csv(names)} if names else None
        return self._c.request("POST", "/api/v1/constellations/export", query=query, text=True)

    def import_(self, data: Union[str, list]) -> list:
        """Made or joined by name, with colour, art and hub. Their names."""
        return (_send(self._c, "POST", "/api/v1/constellations/import", data) or {}).get("imported") or []
