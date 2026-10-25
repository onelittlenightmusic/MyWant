"""A fighter's turn: one character's 攻撃 skill.

Locked on to an enemy (its `target` param — the 攻撃 skill sets it), while
its character stands within `reach` of that enemy, one hit every
`attack_interval` seconds, for that enemy: its `attack_damage`."""

from mywant import current

import battle_common as b

me = current()
b.start(me, "strike")
on = me.params.get("target") or ""
if on:
    char = me.params.get("character_id") or ""
    at = None
    for c in me.server.cursors() or []:
        if (c.get("character_id") or c.get("characterId") or c.get("id")) == char:
            at = (float(c.get("x", 0)), float(c.get("y", 0)))
    tid = me.params.get("target_id")
    enemy = me.server.want(tid) if tid else me.server.want_named(on)
    ec = b.cell_of(enemy) if enemy else None
    if at is not None and ec is not None and b.dist(at, ec) <= float(me.params.get("reach", 6)):
        b.send(me, "strike", float(me.params.get("attack_interval", 1)),
               {"damage": float(me.params.get("attack_damage", 1))}, [on], "attack", [on])
me.finish()
