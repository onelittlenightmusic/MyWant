"""A turret's (or a freeze's) turn.

Standing in a room it is gone for too: what is sent to it, taken. Then, while
it is aimed at something (its `target` param — the room aims it), one shot
every `interval` seconds, for that one: its `damage`, and a freeze's `hold`
(its `duration`)."""

from mywant import current

import battle_common as b

me = current()
if not b.take_hits(me):
    b.start(me, "strike")
    on = me.params.get("target") or ""
    if on:
        freeze = me.type == "freeze"
        body = {"damage": float(me.params.get("damage", 0 if freeze else 1))}
        if freeze:
            body["hold"] = float(me.params.get("duration", 2.5))
        if b.send(me, "strike", float(me.params.get("interval", 1.5)), body, [on],
                  "freeze" if freeze else "attack", [on]):
            me.set("shots", int(me.state.get("strike_clock", {}).get("seq", 0)))
me.finish()
