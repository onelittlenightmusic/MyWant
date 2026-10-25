"""An enemy's (or the boss's) turn.

What is left of it is its own: the shots sent to it come off its hp, a
freeze's hold keeps it `frozen` a while, and at 0 it is `defeated` (its room
takes it off the board). What it goes for is its `target` param (its room
says); it walks there itself, a cell at a time at its `speed`, round what
stands in the way and the walls — and next to it, one blow every `interval`
seconds, for that one: its `damage`."""

from mywant import current

import battle_common as b

me = current()
if me.state.get("defeated"):
    me.finish()
    raise SystemExit(0)

hp = me.state.get("hp")
if not isinstance(hp, (int, float)):
    hp = float(me.params.get("max_hp", 3))
    me.set("max_hp", hp)
t = b.now_ms()
for a in b.take(me):
    hp = max(0.0, hp - a["damage"])
    if a["hold"] > 0:
        me.set("frozen", True)
        me.set("frozen_until", t + int(a["hold"] * 1000))
hp = round(hp, 2)
if hp != me.state.get("hp"):
    me.set("hp", hp)
b.start(me, "strike")

if hp <= 0:
    me.set("defeated", True)
else:
    frozen = bool(me.state.get("frozen"))
    if frozen and t >= int(me.state.get("frozen_until") or 0):
        frozen = False
        me.set("frozen", False)
    on = me.params.get("target") or ""
    if me.state.get("target") != on:
        me.set("target", on)
    if on and not frozen:
        # Only the one it goes for, by id, every turn; the whole board only
        # when it is time to take a step through it.
        tid = me.params.get("target_id")
        target = me.server.want(tid) if tid else me.server.want_named(on)
        here, there = b.cell_of(me.labels), b.cell_of(target) if target else None
        if here is not None and there is not None:
            if b.dist(here, there) <= 1.5:
                b.send(me, "strike", float(me.params.get("interval", 1)),
                       {"damage": float(me.params.get("damage", 1))}, [on], "attack", [on])
            else:
                last = me.state.get("last_step")
                if not isinstance(last, (int, float)):
                    me.set("last_step", t)  # just come in: a step's time first
                elif t - last >= 1000 / max(float(me.params.get("speed", 0.6)), 0.05):
                    wants = me.server.wants()
                    taken = b.occupied_cells(wants) | b.blocked_cells(wants)
                    nxt = b.step_toward(here, there, lambda c: c in taken)
                    if nxt is not None:
                        me.set("last_step", t)
                        me.move_to(*nxt)
me.finish()
