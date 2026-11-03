"""MyWant's Python SDK.

Two halves:

- `Client` — the server's HTTP API (the same one the Go client and the CLI
  speak), one object per resource: `Client().wants.list()`,
  `.things.create(...)`, `.constellations.update(...)`. Standard library
  only.
- `current()` — inside a plugin script: the want this call runs for, its
  state and params as they are now, and a `Client` already pointed at the
  server that called it. What a script does to itself (stop, move its own
  tile) it says through this, and the engine does it.
"""

from .client import Client, ConflictError, MyWantError, NotFoundError, ValidationError
from .plugin import Want, current
from .guide import fill_goto, fill_template, goto_template
from .limits import parallel

__all__ = [
    "Client", "MyWantError", "NotFoundError", "ConflictError", "ValidationError",
    "Want", "current", "fill_goto", "fill_template", "goto_template", "parallel",
]
__version__ = "0.2.0"
