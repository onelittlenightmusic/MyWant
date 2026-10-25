# mywant (Python SDK)

Python for MyWant, in two halves:

- **`Client`** — the server's HTTP API, the one the Go client (`client/`) and
  the CLI speak.
- **`current()`** — inside a plugin script: the want the call runs for.

Standard library only, so a plugin can use it with nothing installed.

## The API

```python
from mywant import Client

c = Client()                                   # MYWANT_URL, or http://localhost:8080
c = Client("https://x.fly.dev", username="mywant", password="…")   # or token="…"

c.wants(type="treasure")                       # every want, or by type / labels
c.want(id); c.want_named("帳簿")
c.create_want({"metadata": {"name": "e", "type": "enemy"}, "spec": {"params": {}}})
c.patch_want(id, params={"target": "帳簿"})     # only these params / labels; None removes
c.set_label(id, "mywant.io/canvas-x", "3")
c.suspend(id); c.resume(id); c.restart(id); c.delete_want(id)

c.expose(id, "strike", "西の砲台.attack")       # publish a state field under a global key
c.set_imports(id, {"西の砲台.attack": "action:西の砲台.attack"})

c.state(id); c.set_state(id, "content", "…")
c.global_state(); c.set_global_state("k", 1)
c.cursors(); c.things(); c.want_types(); c.want_type("treasure")

c.request("GET", "/api/v1/…")                  # anything else
```

Every call raises `MyWantError` (with `.status` and `.body`) when the server
refuses it or cannot be reached. Credentials come from the arguments, or
`MYWANT_TOKEN`, or `MYWANT_USERNAME` / `MYWANT_PASSWORD`.

## In a plugin script

```python
from mywant import current

me = current()                 # the want this call runs for
hp = me.state.get("hp", 0)     # its state as it is now, imports included
dmg = me.params.get("damage")  # its params
me.server.wants(type="enemy")  # the server that called, already connected

me.set("hp", hp - 1)           # its own state fields, as its answer
if hp <= 1:
    me.suspend("worn down")    # stop itself, until resumed
me.move_to(4, 7)               # move its own tile
me.finish()                    # print the answer (the last JSON object is the result)
```

What a script changes about **itself** — its state fields, stopping itself,
moving its own tile — goes back with its answer, and the engine does it, to
that want and no other. What it changes about **another** want goes to the
server, through `me.server`, as anyone's change would.

The engine sets `current()` for each call. A script started on its own reads
it from the environment: `MYWANT_WANT_ID`, `MYWANT_WANT_NAME`, and
`MYWANT_WANT_STATE` / `MYWANT_WANT_PARAMS` / `MYWANT_WANT_LABELS` (JSON).

## Tests

```sh
cd client-py && python3 -m unittest discover -s tests -v
```
