"""MyWant's Python SDK.

Two halves:

- `Client` — the server's HTTP API (the same one the Go client and the CLI
  speak), for any Python that wants to talk to a MyWant server.
- `current()` — inside a plugin script: the want this call runs for, its
  state and params as they are now, and a `Client` already pointed at the
  server that called it. What a script does to itself (stop, move its own
  tile) it says through this, and the engine does it.
"""

from .client import Client, MyWantError
from .plugin import Want, current

__all__ = ["Client", "MyWantError", "Want", "current"]
__version__ = "0.1.0"
