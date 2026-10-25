"""The want a plugin script runs for.

The engine calls a plugin script for one want at a time. Inside that call,
`current()` is that want: its id and name, its params, its state as it is
now — imported values included — and `server`, a Client pointed at the
server that made the call.

    from mywant import current

    me = current()
    hp = me.state.get("hp", 0)
    me.server.wants(type="enemy")
    if hp <= 0:
        me.suspend()

What a script changes about ITSELF goes back with its answer, and the engine
does it: its state fields (`me.set`, written as the script's output, and
mapped by the agent's state_updates), stopping itself (`me.suspend()`), and
moving its own tile (`me.move_to(x, y)`). What it changes about another want
goes to the server, as anyone's change would (`me.server`).

Where the call comes from:

- the engine's plugin service sets it per call (`_enter` / `_leave`, one per
  thread, so two calls at once do not mix);
- a script started on its own reads it from the environment: MYWANT_WANT_ID,
  MYWANT_WANT_NAME, MYWANT_WANT_TYPE, MYWANT_WANT_STATE and MYWANT_WANT_PARAMS
  (JSON), and the
  server's MYWANT_URL / MYWANT_TOKEN / MYWANT_USERNAME / MYWANT_PASSWORD.
"""

from __future__ import annotations

import json
import os
import sys
import threading
from typing import Any, Optional

from .client import Client

_local = threading.local()


class Want:
    """One want, as a plugin script sees itself."""

    def __init__(self, id: str, name: str = "", *, type: str = "", state: Optional[dict] = None,
                 params: Optional[dict] = None, labels: Optional[dict] = None,
                 server: Optional[Client] = None, out=None):
        self.id = id
        self.name = name
        self.type = type
        self.state = dict(state or {})
        self.params = dict(params or {})
        self.labels = dict(labels or {})
        self.server = server or Client()
        self._out = out
        self._answer: dict = {}

    # What it says about itself goes back with the call.

    def set(self, key: str, value: Any) -> None:
        """One of its own state fields, given back as part of its answer —
        the agent's state_updates say which keys of the answer land where."""
        self.state[key] = value
        self._answer[key] = value

    def answer(self) -> dict:
        """What has been set, as the script's result: print it last (or let
        `finish()` do it)."""
        return dict(self._answer)

    def finish(self) -> None:
        """Print the answer — the last JSON object the script writes is its
        result."""
        self._emit(self.answer())

    def suspend(self, reason: str = "") -> None:
        """Stop itself, until it is resumed. Only itself: the engine applies
        this to the want the call is for and no other."""
        self._emit({"_control": "suspend", "_reason": reason})

    def move_to(self, x: int, y: int) -> None:
        """Move its own tile on the board."""
        self._emit({"_tile": {"x": int(x), "y": int(y)}})

    def progress(self, pct: int, message: str = "") -> None:
        self._emit({"_progress": int(pct), "_message": message})

    def _emit(self, obj: dict) -> None:
        out = self._out or sys.stdout
        out.write(json.dumps(obj, ensure_ascii=False) + "\n")
        out.flush()


def current() -> Want:
    """The want this call runs for."""
    w = getattr(_local, "want", None)
    if w is not None:
        return w
    w = _from_env()
    _local.want = w
    return w


def _enter(context: dict, out=None) -> Want:
    """Set the want for this thread's call — for the engine's plugin service.

    `context` is what the engine sends with the call: want_id, want_name,
    state, params, labels, and server: {url, token, username, password}."""
    srv = context.get("server") or {}
    w = Want(
        context.get("want_id", ""),
        context.get("want_name", ""),
        type=context.get("want_type", ""),
        state=context.get("state"),
        params=context.get("params"),
        labels=context.get("labels"),
        server=Client(srv.get("url"), token=srv.get("token"),
                      username=srv.get("username"), password=srv.get("password")),
        out=out,
    )
    _local.want = w
    return w


def _leave() -> None:
    _local.want = None


def _json_env(name: str) -> dict:
    raw = os.environ.get(name)
    if not raw:
        return {}
    try:
        v = json.loads(raw)
    except ValueError:
        return {}
    return v if isinstance(v, dict) else {}


def _from_env() -> Want:
    return Want(
        os.environ.get("MYWANT_WANT_ID", ""),
        os.environ.get("MYWANT_WANT_NAME", ""),
        type=os.environ.get("MYWANT_WANT_TYPE", ""),
        state=_json_env("MYWANT_WANT_STATE"),
        params=_json_env("MYWANT_WANT_PARAMS"),
        labels=_json_env("MYWANT_WANT_LABELS"),
        server=Client(),
    )
