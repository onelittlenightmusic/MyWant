# mywant (Python SDK)

Python for MyWant, in two halves:

- **`Client`** — the server's HTTP API, the one the Go client (`client/`) and
  the CLI speak.
- **`current()`** — inside a plugin script: the want the call runs for.

Standard library only, so a plugin can use it with nothing installed.

The package's code lives in `engine/core/pysdk/mywant/`: the engine embeds
it and puts it on every plugin script's path, and this directory packages
the same files for pip (`pip install ./client-py`).

## The API

One object per resource, as the API has them.

```python
from mywant import Client

c = Client()                                   # MYWANT_URL, or http://localhost:8080
c = Client("https://x.fly.dev", username="mywant", password="…")   # or token="…"

# wants
c.wants.list(type="treasure")                  # every want, or by type / labels
c.wants.get(id); c.wants.named("帳簿")
c.wants.create({"metadata": {"name": "e", "type": "enemy"}, "spec": {"params": {}}})  # or a list, or YAML text
c.wants.patch(id, params={"target": "帳簿"})    # only these params / labels; None removes
c.wants.set_label(id, "mywant.io/canvas-x", "3")
c.wants.suspend(id, id2); c.wants.resume(id); c.wants.restart(id)
c.wants.delete(id)                             # several ids: one request
c.wants.expose(id, "strike", "西の砲台.attack")  # publish a state field under a global key
c.wants.set_imports(id, {"西の砲台.attack": "action:西の砲台.attack"})
c.wants.export(); c.wants.import_(yaml_text)

c.states.get(id); c.states.set(id, "content", "…")
c.global_state.get(); c.global_state.set("k", 1)
c.want_types.list(); c.want_types.get("treasure")
# c.recipes, c.agents, c.capabilities, c.labels alike

# the board
c.things.create("stations", "国分寺"); c.things.update(tid, value="国立")
c.things.patch_labels(tid, set={"a": "1"}, remove=["b"]); c.things.delete(tid, tid2)
c.constellations.create("通勤", [tid, tid2]); c.constellations.update("通勤", capture=tid)
c.characters.list(); c.character_skills.list(); c.devices.list(); c.cursors.list()
c.things.export()                              # YAML text; import_ takes it, or the list it holds
                                               # (constellations, characters alike)

c.system.health(); c.logs.get(); c.errors.list(); c.llm.query("…")

c.request("GET", "/api/v1/…")                  # anything else
```

Every call raises `MyWantError` (with `.status` and `.body`) when the server
refuses it or cannot be reached — `NotFoundError` on 404, `ConflictError` on
409, `ValidationError` on 400. Credentials come from the arguments, or
`MYWANT_TOKEN`, or `MYWANT_USERNAME` / `MYWANT_PASSWORD`.

What is sent as YAML may be a string (sent as it is) or the lists and dicts
it holds (sent as JSON, which the server reads the same); what comes back as
YAML (an export) is a string. Nothing beyond the standard library.

## In a plugin script

```python
from mywant import current

me = current()                 # the want this call runs for
hp = me.state.get("hp", 0)     # its state as it is now, imports included
dmg = me.params.get("damage")  # its params
me.server.wants.list(type="enemy")  # the server that called, already connected

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
