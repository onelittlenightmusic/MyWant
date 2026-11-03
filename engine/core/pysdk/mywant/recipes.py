"""Recipes: /api/v1/recipes."""

from __future__ import annotations

from typing import Any, Mapping, Union

from .client import _q, _send


class Recipes:
    def __init__(self, client):
        self._c = client

    def list(self) -> Any:
        """Every recipe, by name."""
        return self._c.request("GET", "/api/v1/recipes")

    def get(self, id: str) -> dict:
        return self._c.request("GET", f"/api/v1/recipes/{_q(id)}")

    def examples(self, id: str) -> Any:
        return self._c.request("GET", f"/api/v1/recipes/{_q(id)}/examples")

    def create(self, recipe: Union[str, Mapping[str, Any]]) -> Any:
        """A recipe (with its top-level `recipe` key), or its YAML."""
        return _send(self._c, "POST", "/api/v1/recipes", recipe)

    def update(self, id: str, recipe: Union[str, Mapping[str, Any]]) -> Any:
        return _send(self._c, "PUT", f"/api/v1/recipes/{_q(id)}", recipe)

    def delete(self, id: str) -> None:
        self._c.request("DELETE", f"/api/v1/recipes/{_q(id)}")

    def from_want(self, want_id: str, name: str, description: str = "", version: str = "1.0.0") -> Any:
        """Save a running want (and its children) as a recipe."""
        return self._c.request("POST", "/api/v1/recipes/from-want", {
            "wantId": want_id,
            "metadata": {"name": name, "description": description, "version": version},
        })
