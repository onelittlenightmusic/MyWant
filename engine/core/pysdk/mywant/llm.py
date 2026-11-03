"""A question to the server's model: /api/v1/llm."""

from __future__ import annotations

from typing import Optional


class LLM:
    def __init__(self, client):
        self._c = client

    def query(self, message: str, model: Optional[str] = None) -> dict:
        body = {"message": message}
        if model:
            body["model"] = model
        return self._c.request("POST", "/api/v1/llm/query", body)
