"""A heal's turn.

Standing in a room it is gone for too: what is sent to it, taken. Then it
mends: every `interval` seconds one pulse, for each want it mends its share
(`to`) — `amount` per second × `interval` in all, shared among them, so
mending more is never more mending. What it mends is what it is connected to
(its `mend` param, set by the 接続 skill); connected to nothing, it is a
button, and while someone or something is on it, the hurt wants in its
`range`."""

from mywant import current

import battle_common as b

me = current()
if not b.take_hits(me):
    b.start(me, "pulse")
    names = [n for n in (me.params.get("mend") or []) if n]
    if not names and (me.state.get("characters") or me.state.get("things")):
        here = b.cell_of(me.labels)
        reach = float(me.params.get("range", 2))
        if here is not None:
            for w in me.server.wants():
                m = b.meta(w)
                c = b.cell_of(w)
                cur = b.current_of(w)
                hp, mx = cur.get("hp"), cur.get("max_hp")
                if (m.get("id") != me.id and c is not None and b.dist(here, c) <= reach
                        and isinstance(hp, (int, float)) and isinstance(mx, (int, float)) and 0 < hp < mx):
                    names.append(m.get("name"))
    if names:
        interval = float(me.params.get("interval", 1))
        share = float(me.params.get("amount", 2)) * interval / len(names)
        if b.send(me, "pulse", interval, {"amount": share * len(names)}, {n: share for n in names}, "mend", names):
            me.set("healing", names)
    elif me.state.get("healing"):
        me.set("healing", [])
me.finish()
