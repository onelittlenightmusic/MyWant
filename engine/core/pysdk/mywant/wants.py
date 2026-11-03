"""Wants: /api/v1/wants."""

from __future__ import annotations

from typing import Any, Iterable, Mapping, Optional, Union

from .client import _csv, _q, _send


class Wants:
    def __init__(self, client):
        self._c = client

    def list(self, *, type: Optional[str] = None, labels: Optional[Mapping[str, str]] = None,
             include_system: bool = False) -> list:
        """Every want (or those of one type, or carrying these labels)."""
        query: dict = {}
        if type:
            query["type"] = type
        if labels:
            query["label"] = [f"{k}={v}" for k, v in labels.items()]
        if include_system:
            query["includeSystemWants"] = True
        got = self._c.request("GET", "/api/v1/wants", query=query)
        if isinstance(got, dict):
            return got.get("wants") or []
        return got or []

    def get(self, id: str) -> dict:
        return self._c.request("GET", f"/api/v1/wants/{_q(id)}")

    def named(self, name: str) -> Optional[dict]:
        """One want, by name — None when there is none."""
        for w in self.list():
            if (w.get("metadata") or {}).get("name") == name:
                return w
        return None

    def create(self, wants: Union[str, Mapping[str, Any], Iterable[Mapping[str, Any]]]) -> Any:
        """Add one want ({metadata, spec}), several, or a YAML text of them."""
        if isinstance(wants, Mapping):
            wants = [wants]
        elif not isinstance(wants, str):
            wants = list(wants)
        return _send(self._c, "POST", "/api/v1/wants", wants)

    def validate(self, wants: Union[str, Mapping[str, Any], Iterable[Mapping[str, Any]]]) -> Any:
        """What create would say, without deploying."""
        if isinstance(wants, Mapping):
            wants = [wants]
        elif not isinstance(wants, str):
            wants = list(wants)
        return _send(self._c, "POST", "/api/v1/wants/validate", wants)

    def update(self, id: str, want: Union[str, Mapping[str, Any]]) -> Any:
        """Replace a want's metadata and spec (PUT)."""
        return _send(self._c, "PUT", f"/api/v1/wants/{_q(id)}", want if isinstance(want, str) else dict(want))

    def patch(self, id: str, *, params: Optional[Mapping[str, Any]] = None,
              labels: Optional[Mapping[str, Optional[str]]] = None) -> Any:
        """Change only these params and labels; a value of None takes it away."""
        body: dict = {}
        if labels is not None:
            body["metadata"] = {"labels": dict(labels)}
        if params is not None:
            body["spec"] = {"params": dict(params)}
        return self._c.request("PATCH", f"/api/v1/wants/{_q(id)}", body)

    def delete(self, *ids: str) -> Any:
        """One want, or several in one request."""
        if len(ids) == 1:
            return self._c.request("DELETE", f"/api/v1/wants/{_q(ids[0])}")
        return self._c.request("DELETE", "/api/v1/wants", query={"ids": _csv(ids)})

    # ── running ─────────────────────────────────────────────────────────────

    def _control(self, op: str, ids: Iterable[str]) -> None:
        self._c.request("POST", f"/api/v1/wants/{op}", {"ids": list(ids)})

    def suspend(self, *ids: str) -> None:
        self._control("suspend", ids)

    def resume(self, *ids: str) -> None:
        self._control("resume", ids)

    def restart(self, *ids: str) -> None:
        self._control("restart", ids)

    def stop(self, *ids: str) -> None:
        self._control("stop", ids)

    def start(self, *ids: str) -> None:
        self._control("start", ids)

    def status(self, id: str) -> dict:
        return self._c.request("GET", f"/api/v1/wants/{_q(id)}/status")

    def results(self, id: str) -> Any:
        return self._c.request("GET", f"/api/v1/wants/{_q(id)}/results")

    # ── labels, order, wiring ───────────────────────────────────────────────

    def set_label(self, id: str, key: str, value: str) -> None:
        self._c.request("POST", f"/api/v1/wants/{_q(id)}/labels", {"key": key, "value": value})

    def remove_label(self, id: str, key: str) -> None:
        self._c.request("DELETE", f"/api/v1/wants/{_q(id)}/labels/{_q(key)}")

    def set_order(self, id: str, new_order: int) -> None:
        self._c.request("PUT", f"/api/v1/wants/{_q(id)}/order", {"newOrder": new_order})

    def add_using(self, id: str, key: str, value: str) -> None:
        self._c.request("POST", f"/api/v1/wants/{_q(id)}/using", {"key": key, "value": value})

    def remove_using(self, id: str, key: str) -> None:
        self._c.request("DELETE", f"/api/v1/wants/{_q(id)}/using/{_q(key)}")

    def expose(self, id: str, current_state: str, as_key: str) -> bool:
        """Have a want publish one of its state fields under a global key —
        unless it already does. Whether anything changed."""
        w = self.get(id)
        for e in ((w.get("spec") or {}).get("exposes") or []):
            if e.get("currentState") == current_state and e.get("as") == as_key:
                return False
        self._c.request("POST", f"/api/v1/wants/{_q(id)}/relations",
                        {"currentState": current_state, "as": as_key})
        return True

    def set_imports(self, id: str, imports: Mapping[str, str]) -> bool:
        """Make a want's imports exactly these (global key → its local key) —
        unless they already are. Read from the want itself, not a listing,
        which can lag behind it. Whether anything changed."""
        w = self.get(id)
        if ((w.get("spec") or {}).get("imports") or {}) == dict(imports):
            return False
        self.update(id, {"metadata": w["metadata"],
                         "spec": {**(w.get("spec") or {}), "imports": dict(imports)}})
        return True

    # ── in and out ──────────────────────────────────────────────────────────

    def export(self, *, include_system: bool = False) -> str:
        """Every want, as YAML."""
        query = {"includeSystemWants": True} if include_system else None
        return self._c.request("POST", "/api/v1/wants/export", query=query, text=True)

    def import_(self, data: Union[str, list]) -> Any:
        """Read wants in: YAML, or the list it holds."""
        return _send(self._c, "POST", "/api/v1/wants/import", data)
