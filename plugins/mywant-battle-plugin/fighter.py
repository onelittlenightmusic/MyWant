"""A fighter's turn.

A fighter is a tile in its room, a character's sword stood where it guards.
It goes for any enemy within `reach` of where it stands, as a turret does —
but first for the hub of a capture it is in (b.hubs), and for the one its
character locked on to with the 攻撃 skill (its `target` param). Of those in
reach, the nearest; none of them in reach, the nearest of the rest: one hit
every `attack_interval` seconds, its `attack_damage`. Its room imports its hits into every enemy (room.py); only
the one a hit names takes it.

A fighter still hidden (as the room first made them, before they stood in it)
strikes from where its character stands instead."""

from mywant import current

import battle_common as b

me = current()
b.start(me, "strike")

locked = me.params.get("target") or ""
first = b.hubs(me.labels)

# Where it strikes from: its tile, or — hidden — its character.
at = None
if me.labels.get("mywant.io/canvas-hidden") != "true":
    at = b.cell_of(me.labels)
if at is None:
    char = me.params.get("character_id") or ""
    for c in me.server.cursors() or []:
        if (c.get("character_id") or c.get("characterId") or c.get("id")) == char:
            at = (float(c.get("x", 0)), float(c.get("y", 0)))

on = None
if at is not None:
    reach = float(me.params.get("reach", 6))
    told, rest = [], []
    for e in me.server.wants():
        m = b.meta(e)
        if m.get("type") not in b.ENEMY_TYPES or b.current_of(e).get("defeated"):
            continue
        c = b.cell_of(e)
        if c is None or b.dist(at, c) > reach:
            continue
        mine = m.get("name") == locked or m.get("id") in first
        (told if mine else rest).append((b.dist(at, c), m.get("name")))
    if told or rest:
        on = min(told or rest)[1]
        b.send(me, "strike", float(me.params.get("attack_interval", 1)),
               {"damage": float(me.params.get("attack_damage", 1))}, [on], "attack", [on])
# Nothing to hit: its last blow is not left standing as if it were still
# going for something.
if on is None and (me.state.get("strike") or {}).get("to"):
    me.set("strike", {"seq": int((me.state.get("strike") or {}).get("seq") or 0), "to": []})
me.finish()
