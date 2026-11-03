"""Things — the values you have named (a station, a city…): /api/v1/things.

A thing is addressed by its id (thg-…). Schema: want-spec's
spec/thing-spec.yaml.
"""

from __future__ import annotations

from typing import Iterable, Mapping, Optional, Union

from .client import _csv, _q, _send


class Things:
    def __init__(self, client):
        self._c = client

    def list(self) -> list:
        """Every thing, whole (labels, the wants naming it, …)."""
        return (self._c.request("GET", "/api/v1/things") or {}).get("things") or []

    def get(self, id: str) -> dict:
        return self._c.request("GET", f"/api/v1/things/{_q(id)}")

    def create(self, catalog: str, value: str) -> dict:
        """Remember a value under a kind ("stations", or the subtype "station")."""
        return self._c.request("POST", "/api/v1/things", {"catalog": catalog, "value": value})

    def update(self, id: str, *, catalog: Optional[str] = None, value: Optional[str] = None) -> dict:
        """Rename it, or file it under another kind — the same thing, kept."""
        body = {k: v for k, v in (("catalog", catalog), ("value", value)) if v}
        return self._c.request("PATCH", f"/api/v1/things/{_q(id)}", body)

    def delete(self, *ids: str) -> dict:
        """One, or several in one request: {deleted: [...], missing: [...]}."""
        if len(ids) == 1:
            return self._c.request("DELETE", f"/api/v1/things/{_q(ids[0])}")
        return self._c.request("DELETE", "/api/v1/things", query={"ids": _csv(ids)})

    def set_label(self, id: str, key: str, value: str) -> dict:
        return self._c.request("POST", f"/api/v1/things/{_q(id)}/labels", {"key": key, "value": value})

    def remove_label(self, id: str, key: str) -> dict:
        return self._c.request("DELETE", f"/api/v1/things/{_q(id)}/labels/{_q(key)}")

    def patch_labels(self, id: str, set: Optional[Mapping[str, str]] = None,
                     remove: Optional[Iterable[str]] = None) -> dict:
        """Set and remove several labels in one request; the labels after: {id, labels}."""
        return self._c.request("PATCH", f"/api/v1/things/{_q(id)}/labels",
                               {"set": dict(set or {}), "remove": list(remove or [])})

    def export(self, ids: Optional[Iterable[str]] = None) -> str:
        """Things as YAML (id, catalog, value, createdAt, labels); all, or these."""
        query = {"ids": _csv(ids)} if ids else None
        return self._c.request("POST", "/api/v1/things/export", query=query, text=True)

    def import_(self, data: Union[str, list]) -> list:
        """Read things in (YAML, or the list it holds): matched by kind and
        value, their labels laid on. The ids they have here."""
        return (_send(self._c, "POST", "/api/v1/things/import", data) or {}).get("imported") or []
