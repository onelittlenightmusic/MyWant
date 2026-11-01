"""The way to one thing on a web page, for a plugin whose answers are things
someone may want to go to (a booking site's open slot, a shop's item).

An optional convention — a plugin with nothing on the web has no use for it.
A plugin that has one keeps a template beside its script, `goto.json`:

    {
      "guide":    {...},          the facts its voice.py says the way from
      "steps":    [...],          what to do on the page first (browser_run's
                                  steps: click, waitForElement, …)
      "elements": [...],          the things to bring the person to, web want
                                  elements ({role, name, selector}); the first
                                  is lit and stood on, the rest are jumped to
      "then":     [{"url_contains": "...", "elements": [...]}]
                                  pages the way goes on to in the same tab
    }

with `{{name}}` where an answer's own values go (a room, a time). Each step
and element may carry a `say_key`: the moment it is, for which the type's
voice.py says a line (its "guide" event); the words are never in the template.
Keys starting with "_" are notes and are left out.

    from mywant import fill_goto

    slot["goto"] = fill_goto({"room": room, "value": iso})

An answer that is a list of many places keeps the template once instead — the
want's `goto_template` state (goto_template()), each place only its values
(`goto_values`) — so its history does not hold the same way a hundred times:

    result["goto_template"] = goto_template()
    slot["goto_values"] = {"room": room, "value": iso}

The GUI opens an answer's `url` with its `goto` through the MyWant extension
(POST /web-wants/open): the steps run, the robot says each line, and the
CursorMan is left on the first element.
"""

from __future__ import annotations

import inspect
import json
import os
from typing import Any, Dict, Optional

TEMPLATE_FILE = "goto.json"


def fill_template(template: Any, values: Dict[str, Any]) -> Any:
    """`template` with every `{{name}}` in its strings replaced by values[name],
    lists and objects walked through; keys starting with "_" left out."""
    if isinstance(template, str):
        for k, v in values.items():
            template = template.replace("{{" + k + "}}", str(v))
        return template
    if isinstance(template, list):
        return [fill_template(t, values) for t in template]
    if isinstance(template, dict):
        return {k: fill_template(v, values) for k, v in template.items() if not k.startswith("_")}
    return template


_cache: Dict[str, Any] = {}


def _template_path(template: Optional[str], depth: int) -> str:
    if template is not None:
        return template
    caller = inspect.stack()[depth].filename
    return os.path.join(os.path.dirname(os.path.abspath(caller)), TEMPLATE_FILE)


def _load(path: str) -> Any:
    if path not in _cache:
        with open(path, encoding="utf-8") as f:
            _cache[path] = json.load(f)
    return _cache[path]


def goto_template(template: Optional[str] = None) -> Dict[str, Any]:
    """The plugin's goto.json as it is, its notes left out — to keep once on
    the want (a state field, `goto_template`) while each answer carries only
    its own values (`goto_values`): a list of many places then holds the way's
    shape once, not once per place. The GUI fills it when one is opened
    (lib/webGuide)."""
    return fill_template(_load(_template_path(template, 2)), {})


def fill_goto(values: Dict[str, Any], template: Optional[str] = None) -> Dict[str, Any]:
    """The plugin's `goto.json`, filled with one answer's values.

    `template` is the template's path; by default goto.json beside the script
    that calls this. Read once per process.
    """
    return fill_template(_load(_template_path(template, 2)), values)
