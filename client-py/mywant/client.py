"""The server's HTTP API, as Python.

Standard library only (urllib), so a plugin can use it with nothing
installed. Every call raises MyWantError on a status outside 2xx, carrying
the status and what the server said.
"""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request
from base64 import b64encode
from typing import Any, Iterable, Mapping, Optional

DEFAULT_URL = "http://localhost:8080"


class MyWantError(Exception):
    """A request the server refused or could not be reached for."""

    def __init__(self, message: str, status: int = 0, body: str = ""):
        super().__init__(message)
        self.status = status
        self.body = body


class Client:
    """A MyWant server.

    `url` defaults to MYWANT_URL, then http://localhost:8080. Authentication
    is a bearer token (`token`, or MYWANT_TOKEN) or basic auth (`username` /
    `password`, or MYWANT_USERNAME / MYWANT_PASSWORD), as the Go client's.
    """

    def __init__(
        self,
        url: Optional[str] = None,
        *,
        token: Optional[str] = None,
        username: Optional[str] = None,
        password: Optional[str] = None,
        timeout: float = 4.0,
        retries: int = 1,
    ):
        self.url = (url or os.environ.get("MYWANT_URL") or DEFAULT_URL).rstrip("/")
        self.token = token if token is not None else os.environ.get("MYWANT_TOKEN") or None
        self.username = username if username is not None else os.environ.get("MYWANT_USERNAME") or None
        self.password = password if password is not None else os.environ.get("MYWANT_PASSWORD") or None
        self.timeout = timeout
        # A GET that timed out is asked once more: it changes nothing, and a
        # server busy for a moment usually answers the second time.
        self.retries = retries

    # ── the one way every call goes ─────────────────────────────────────────

    def request(self, method: str, path: str, body: Any = None, query: Optional[Mapping[str, Any]] = None) -> Any:
        """Any endpoint: `path` from /api/v1/…, `body` sent as JSON. The
        decoded JSON answer, or the text when it is not JSON, or None."""
        url = self.url + path
        if query:
            pairs = []
            for k, v in query.items():
                if v is None:
                    continue
                for item in v if isinstance(v, (list, tuple)) else [v]:
                    pairs.append((k, _query_value(item)))
            if pairs:
                url += "?" + urllib.parse.urlencode(pairs)
        data = None
        headers = {"Accept": "application/json", "User-Agent": "mywant-py/0.1.0"}
        if body is not None:
            data = json.dumps(body, ensure_ascii=False).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if self.token:
            headers["Authorization"] = "Bearer " + self.token
        elif self.username or self.password:
            pair = f"{self.username or ''}:{self.password or ''}".encode("utf-8")
            headers["Authorization"] = "Basic " + b64encode(pair).decode("ascii")
        tries = 1 + (self.retries if method.upper() == "GET" else 0)
        for attempt in range(tries):
            req = urllib.request.Request(url, data=data, method=method.upper(), headers=headers)
            try:
                with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                    raw = resp.read()
                break
            except urllib.error.HTTPError as e:
                text = e.read().decode("utf-8", "replace")
                raise MyWantError(f"{method.upper()} {path}: {e.code} {text.strip()}", e.code, text) from None
            except (urllib.error.URLError, TimeoutError, OSError) as e:
                if attempt + 1 < tries:
                    continue
                reason = getattr(e, "reason", e)
                raise MyWantError(f"{method.upper()} {path}: cannot reach {self.url}: {reason}") from None
        if not raw:
            return None
        text = raw.decode("utf-8", "replace")
        try:
            return json.loads(text)
        except ValueError:
            return text

    # ── wants ───────────────────────────────────────────────────────────────

    def wants(self, *, type: Optional[str] = None, labels: Optional[Mapping[str, str]] = None,
              include_system: bool = False) -> list:
        """Every want (or those of one type, or carrying these labels)."""
        query: dict = {}
        if type:
            query["type"] = type
        if labels:
            query["label"] = [f"{k}={v}" for k, v in labels.items()]
        if include_system:
            query["includeSystemWants"] = True
        got = self.request("GET", "/api/v1/wants", query=query)
        if isinstance(got, dict):
            return got.get("wants") or []
        return got or []

    def want(self, id: str) -> dict:
        """One want, by id."""
        return self.request("GET", f"/api/v1/wants/{_q(id)}")

    def want_named(self, name: str) -> Optional[dict]:
        """One want, by name — None when there is none."""
        for w in self.wants():
            if (w.get("metadata") or {}).get("name") == name:
                return w
        return None

    def create_wants(self, wants: Iterable[Mapping[str, Any]]) -> Any:
        """Add wants (each {metadata, spec})."""
        return self.request("POST", "/api/v1/wants", list(wants))

    def create_want(self, want: Mapping[str, Any]) -> Any:
        return self.create_wants([want])

    def delete_want(self, id: str) -> None:
        self.request("DELETE", f"/api/v1/wants/{_q(id)}")

    def update_want(self, id: str, want: Mapping[str, Any]) -> Any:
        """Replace a want's metadata and spec (PUT)."""
        return self.request("PUT", f"/api/v1/wants/{_q(id)}", dict(want))

    def patch_want(self, id: str, *, params: Optional[Mapping[str, Any]] = None,
                   labels: Optional[Mapping[str, Optional[str]]] = None) -> Any:
        """Change only these params and labels; a value of None takes it away."""
        body: dict = {}
        if labels is not None:
            body["metadata"] = {"labels": dict(labels)}
        if params is not None:
            body["spec"] = {"params": dict(params)}
        return self.request("PATCH", f"/api/v1/wants/{_q(id)}", body)

    def set_label(self, id: str, key: str, value: str) -> None:
        self.request("POST", f"/api/v1/wants/{_q(id)}/labels", {"key": key, "value": value})

    def remove_label(self, id: str, key: str) -> None:
        self.request("DELETE", f"/api/v1/wants/{_q(id)}/labels/{_q(key)}")

    def _control(self, op: str, ids: Iterable[str]) -> None:
        self.request("POST", f"/api/v1/wants/{op}", {"ids": list(ids)})

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

    # ── a want's wiring (its spec) ──────────────────────────────────────────

    def expose(self, id: str, current_state: str, as_key: str) -> bool:
        """Have a want publish one of its state fields under a global key —
        unless it already does. Whether anything changed."""
        w = self.want(id)
        for e in ((w.get("spec") or {}).get("exposes") or []):
            if e.get("currentState") == current_state and e.get("as") == as_key:
                return False
        self.request("POST", f"/api/v1/wants/{_q(id)}/relations",
                     {"currentState": current_state, "as": as_key})
        return True

    def set_imports(self, id: str, imports: Mapping[str, str]) -> bool:
        """Make a want's imports exactly these (global key → its local key) —
        unless they already are. Read from the want itself, not a listing,
        which can lag behind it. Whether anything changed."""
        w = self.want(id)
        if ((w.get("spec") or {}).get("imports") or {}) == dict(imports):
            return False
        self.update_want(id, {"metadata": w["metadata"],
                              "spec": {**(w.get("spec") or {}), "imports": dict(imports)}})
        return True

    # ── state ───────────────────────────────────────────────────────────────

    def state(self, id: str) -> dict:
        """A want's state, as /states gives it."""
        return self.request("GET", f"/api/v1/states/{_q(id)}")

    def set_state(self, id: str, key: str, value: Any) -> None:
        self.request("PUT", f"/api/v1/states/{_q(id)}/{_q(key)}", value)

    def global_state(self) -> dict:
        got = self.request("GET", "/api/v1/global-state")
        if isinstance(got, dict) and "state" in got and isinstance(got["state"], dict):
            return got["state"]
        return got or {}

    def set_global_state(self, key: str, value: Any) -> None:
        self.request("PUT", f"/api/v1/global-state/{_q(key)}", value)

    # ── the board ───────────────────────────────────────────────────────────

    def cursors(self) -> Any:
        """Where each character stands."""
        return self.request("GET", "/api/v1/cursors")

    def things(self) -> Any:
        return self.request("GET", "/api/v1/things")

    def want_types(self) -> Any:
        return self.request("GET", "/api/v1/want-types")

    def want_type(self, name: str) -> Any:
        return self.request("GET", f"/api/v1/want-types/{_q(name)}")


def _query_value(v: Any) -> str:
    if isinstance(v, bool):
        return "true" if v else "false"
    return str(v)


def _q(s: str) -> str:
    return urllib.parse.quote(str(s), safe="")
