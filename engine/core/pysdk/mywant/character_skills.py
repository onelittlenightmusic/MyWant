"""A character's skills on the board (the Y slot's): /api/v1/character-skills.

Defined by the GUI extension that gives the board them, which puts them when
it starts. Schema: want-spec's spec/character-skill-spec.yaml.
"""

from __future__ import annotations

from typing import Any, Mapping, Union

from .client import _q, _send


class CharacterSkills:
    def __init__(self, client):
        self._c = client

    def list(self) -> list:
        """In their digits' order."""
        return (self._c.request("GET", "/api/v1/character-skills") or {}).get("skills") or []

    def get(self, name: str) -> dict:
        return self._c.request("GET", f"/api/v1/character-skills/{_q(name)}")

    def create(self, skill: Union[str, Mapping[str, Any]]) -> dict:
        """A new one (fails when its name is taken)."""
        return _send(self._c, "POST", "/api/v1/character-skills", skill)

    def put(self, name: str, skill: Union[str, Mapping[str, Any]]) -> dict:
        """Made or replaced."""
        return _send(self._c, "PUT", f"/api/v1/character-skills/{_q(name)}", skill)

    def delete(self, name: str) -> None:
        self._c.request("DELETE", f"/api/v1/character-skills/{_q(name)}")
