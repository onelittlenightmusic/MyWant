"""How much a plugin may do at once where it runs.

A plugin that does several things side by side (reads five stores' pages at
once) asks how many it may run together: on a small server — fly's 512MB, both
the GUI and the engine in it — five errands waiting at once were enough to stop
the engine. The server's owner says so in MyWant's config, which reaches every
plugin script as an environment variable:

    # ~/.mywant/config.yaml
    environments:
      MYWANT_PARALLEL: "1"

Nothing set, a plugin runs as it was written to.

    from mywant import parallel

    with ThreadPoolExecutor(max_workers=parallel(len(urls))) as pool: ...
"""

from __future__ import annotations

import os

ENV = "MYWANT_PARALLEL"


def parallel(default: int) -> int:
    """How many at once: the server's MYWANT_PARALLEL when it sets one (at
    least 1, at most `default`), else `default`."""
    raw = os.environ.get(ENV, "").strip()
    if not raw:
        return max(1, default)
    try:
        n = int(raw)
    except ValueError:
        return max(1, default)
    return max(1, min(n, default))
