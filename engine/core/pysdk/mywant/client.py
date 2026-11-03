"""The server's HTTP API, as Python.

Standard library only (urllib), so a plugin can use it with nothing
installed. One object per resource, as the API has them:

    c = Client()
    c.wants.list(type="enemy")
    c.things.create("stations", "国分寺")
    c.constellations.update("通勤", members=[...])

Every call raises MyWantError (NotFoundError on 404, ConflictError on 409,
ValidationError on 400) on a status outside 2xx, carrying the status and
what the server said.
"""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request
from base64 import b64encode
from typing import Any, Mapping, Optional

DEFAULT_URL = "http://localhost:8080"


class MyWantError(Exception):
    """A request the server refused or could not be reached for."""

    def __init__(self, message: str, status: int = 0, body: str = ""):
        super().__init__(message)
        self.status = status
        self.body = body


class ValidationError(MyWantError):
    """400: what was sent does not hold."""


class NotFoundError(MyWantError):
    """404: no such one."""


class ConflictError(MyWantError):
    """409: taken, or in a state that forbids it."""


_ERRORS = {400: ValidationError, 404: NotFoundError, 409: ConflictError}


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

        # Imported here: each resource module imports this one for _q.
        from .agents import Agents
        from .capabilities import Capabilities
        from .character_skills import CharacterSkills
        from .characters import Characters
        from .constellations import Constellations
        from .cursors import Cursors
        from .devices import Devices
        from .errors import Errors
        from .global_state import GlobalState
        from .labels import Labels
        from .llm import LLM
        from .logs import Logs
        from .reactions import Reactions
        from .recipes import Recipes
        from .states import States
        from .system import System
        from .things import Things
        from .want_types import WantTypes
        from .wants import Wants
        from .webhooks import Webhooks

        self.wants = Wants(self)
        self.states = States(self)
        self.global_state = GlobalState(self)
        self.want_types = WantTypes(self)
        self.recipes = Recipes(self)
        self.agents = Agents(self)
        self.capabilities = Capabilities(self)
        self.labels = Labels(self)
        self.things = Things(self)
        self.constellations = Constellations(self)
        self.characters = Characters(self)
        self.character_skills = CharacterSkills(self)
        self.devices = Devices(self)
        self.cursors = Cursors(self)
        self.system = System(self)
        self.logs = Logs(self)
        self.errors = Errors(self)
        self.llm = LLM(self)
        self.webhooks = Webhooks(self)
        self.reactions = Reactions(self)

    # ── the one way every call goes ─────────────────────────────────────────

    def request(self, method: str, path: str, body: Any = None, query: Optional[Mapping[str, Any]] = None,
                *, yaml: Optional[str] = None, text: bool = False) -> Any:
        """Any endpoint: `path` from /api/v1/…, `body` sent as JSON, or
        `yaml` sent as it is. The decoded JSON answer — or, with `text` or
        when it is not JSON, the text — or None."""
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
        headers = {"Accept": "application/json", "User-Agent": "mywant-py/0.2.0"}
        if yaml is not None:
            data = yaml.encode("utf-8")
            headers["Content-Type"] = "application/yaml"
        elif body is not None:
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
                msg = e.read().decode("utf-8", "replace")
                cls = _ERRORS.get(e.code, MyWantError)
                raise cls(f"{method.upper()} {path}: {e.code} {msg.strip()}", e.code, msg) from None
            except (urllib.error.URLError, TimeoutError, OSError) as e:
                if attempt + 1 < tries:
                    continue
                reason = getattr(e, "reason", e)
                raise MyWantError(f"{method.upper()} {path}: cannot reach {self.url}: {reason}") from None
        if not raw:
            return None
        decoded = raw.decode("utf-8", "replace")
        if text:
            return decoded
        try:
            return json.loads(decoded)
        except ValueError:
            return decoded


def _query_value(v: Any) -> str:
    if isinstance(v, bool):
        return "true" if v else "false"
    return str(v)


def _q(s: str) -> str:
    return urllib.parse.quote(str(s), safe="")


def _csv(items) -> str:
    """Items for an ?ids= / ?names= list."""
    return ",".join(items)


def _send(c: "Client", method: str, path: str, data: Any, query: Optional[Mapping[str, Any]] = None) -> Any:
    """`data` as the server reads it either way: a string is YAML, sent as it
    is; anything else is JSON (which the server's YAML reader takes too)."""
    if isinstance(data, str):
        return c.request(method, path, query=query, yaml=data)
    return c.request(method, path, data, query)
