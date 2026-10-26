"""A fighter's turn.

A fighter is a tile in its room, a character's sword stood where it guards.
What it goes for is what it is tied to: the enemies in a constellation with it
(Y on the board ties them — before they are near, too: it waits for them; a
capture, only to the tile the net was aimed at — b.tied), and the one its
character locked on to with the 攻撃 skill (its `target` param).
Of those within `reach` of where it stands, the nearest: one hit every
`attack_interval` seconds, its `attack_damage`. Its room imports that hit into
the enemies it is tied to (room.py), and only those.

A fighter still hidden (as the room first made them, before they stood in it)
strikes from where its character stands instead."""

from mywant import current

import battle_common as b

me = current()
b.start(me, "strike")

mine = any(k.startswith(b.CONSTELLATION) for k in me.labels)
locked = me.params.get("target") or ""

# Where it strikes from: its tile, or — hidden — its character.
at = None
if me.labels.get("mywant.io/canvas-hidden") != "true":
    at = b.cell_of(me.labels)
if at is None:
    char = me.params.get("character_id") or ""
    for c in me.server.cursors() or []:
        if (c.get("character_id") or c.get("characterId") or c.get("id")) == char:
            at = (float(c.get("x", 0)), float(c.get("y", 0)))

if at is not None and (mine or locked):
    reach = float(me.params.get("reach", 6))
    tied = []
    for e in me.server.wants():
        m = b.meta(e)
        if m.get("type") not in b.ENEMY_TYPES or b.current_of(e).get("defeated"):
            continue
        if m.get("name") == locked or b.tied(me.id, me.labels, m.get("id"), m.get("labels")):
            c = b.cell_of(e)
            if c is not None and b.dist(at, c) <= reach:
                tied.append((b.dist(at, c), m.get("name")))
    if tied:
        on = min(tied)[1]
        b.send(me, "strike", float(me.params.get("attack_interval", 1)),
               {"damage": float(me.params.get("attack_damage", 1))}, [on], "attack", [on])
me.finish()
