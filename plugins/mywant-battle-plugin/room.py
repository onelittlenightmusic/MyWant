"""A room defense's turn: the battle, run by the room itself.

It is the third party that joins the battle's wants. Through the server, as
anyone's change to another want would go, it

  - brings its enemies in, at its way in, one every `spawn_every` seconds;
  - says what each enemy goes for and what each turret is aimed at (their
    `target` param), and lets a fighter go of an enemy gone or too far;
  - wires who can reach whom: each act exposed under its own key, imported by
    what it can land on (battle_common);
  - takes a fallen enemy off the board, and the rest when the battle is over.

What it writes about itself — the phase, the round, how many came in and
fell, who is standing on it — is its own answer. What anything does is that
want's own: shots, blows, steps, mends, a hit taken.
"""

import math
import time

from mywant import MyWantError, current

import battle_common as b

FALL_LINGER_MS = 900
CHARACTER_REACH = 6.0

me = current()
_server = me.server
_failed = []


class _Asking:
    """The server, for what this turn asks of it: a change that fails is
    noted (`failed`) and the turn goes on, so what the room says about itself
    — who is on it, the round, the phase — is always kept."""

    def __getattr__(self, name):
        fn = getattr(_server, name)
        if name in ("wants", "want", "global_state", "cursors", "want_named"):
            return fn

        def asked(*a, **kw):
            try:
                return fn(*a, **kw)
            except MyWantError as e:
                _failed.append(f"{name}: {e}"[:160])
                return None
        return asked


srv = _Asking()
t = b.now_ms()

# Never achieved: a battle cleared waits to be begun again, and only a want
# still running hears someone step onto it.
me.set("achieved", False)

try:
    # Everything this turn reads, read first: a turn that cannot see the
    # board changes nothing at all, rather than half of what it would.
    wants = srv.wants()
    gs = srv.global_state()
    cursor_list = srv.cursors() or []
except MyWantError:
    # The board could not be read this turn: nothing is changed, and the next
    # turn asks again.
    me.finish()
    raise SystemExit(0)
by_name = {b.meta(w).get("name"): w for w in wants}


def is_mine(w):
    m = b.meta(w)
    return m.get("type") in b.ENEMY_TYPES and (m.get("labels") or {}).get(b.ENEMY_OF) == me.id


def running(w):
    return w.get("status") not in ("suspended", "failed", "terminated", "deleting", "archived", "cancelled")


rx, ry = int(me.params.get("room_x", 0)), int(me.params.get("room_y", 0))
rw, rh = int(me.params.get("room_w", 8)), int(me.params.get("room_h", 6))


def in_room():
    out = []
    for w in wants:
        c = b.cell_of(w)
        if c and rx <= c[0] < rx + rw and ry <= c[1] < ry + rh and b.meta(w).get("type") not in b.ENEMY_TYPES:
            out.append(w)
    return out


def is_down(w):
    """Worn down to nothing, and so stopped (or stopping) itself."""
    hp = b.current_of(w).get("hp")
    return w.get("status") == "suspended" or (isinstance(hp, (int, float)) and hp <= 0)


def aim(w, target_name):
    """Have w aimed at `target_name` (or nothing) — its `target` param, and
    `target_id` so it can look that one up by itself — only when that
    changes. Never one that is down: changing its params restarts it, and a
    restart would stand it back up."""
    if is_down(w):
        return
    have = b.params_of(w).get("target") or ""
    if have != (target_name or ""):
        tid = b.meta(by_name[target_name])["id"] if target_name in by_name else None
        srv.patch_want(b.meta(w)["id"], params={"target": target_name or None, "target_id": tid})


fallen = dict(me.state.get("fallen") or {})
enemies = []
for w in wants:
    if not is_mine(w):
        continue
    wid = b.meta(w)["id"]
    if wid in fallen:
        if t - fallen[wid] >= FALL_LINGER_MS:
            srv.delete_want(wid)
            fallen.pop(wid)
        continue
    enemies.append(w)

# A reset stood on puts this battle back as it was.
reset_on = dict(me.state.get("reset_stood_on") or {})
phase = me.state.get("phase") or "ready"
rnd = int(me.state.get("round") or 0)
for w in wants:
    m = b.meta(w)
    if m.get("type") != "battle_reset" or b.params_of(w).get("battle") not in (me.name, me.id):
        continue
    on = bool(b.current_of(w).get("characters"))
    if on and not reset_on.get(m["id"]):
        for e in enemies:
            srv.delete_want(b.meta(e)["id"])
        enemies = []
        down = [b.meta(x)["id"] for x in in_room()
                if b.meta(x).get("type") in b.DEFENDABLE_TYPES and x.get("status") == "suspended"]
        if down:
            srv.resume(*down)
        rnd += 1
        phase = "ready"
        me.set("spawned", 0)
        me.set("defeated", 0)
        me.set("targets", [])
        me.set("say", f"元に戻しました（{len(down)} 個を再開）— 乗ると始まります")
    reset_on[m["id"]] = on
me.set("reset_stood_on", reset_on)

# It says which round it is, from the first.
rkey = b.round_key(me.name)
srv.expose(me.id, "round", rkey)

# Someone stepping on it begins a battle that is waiting, or over.
on = bool(me.state.get("characters"))
if on and not me.state.get("stood_on") and phase != "fighting":
    holds = [w for w in in_room() if b.meta(w).get("type") in b.DEFENDABLE_TYPES and running(w)]
    if not holds:
        me.set("say", "守るものがありません — 部屋の want を再開してください")
    else:
        rnd += 1
        phase = "fighting"
        fallen = {}
        me.set("spawned", 0)
        me.set("defeated", 0)
        me.set("targets", [])
        me.set("last_spawn", 0)
        me.set("say", "")
        # For its score when it is over: when it began, how many it had to
        # hold, and who began it.
        me.set("started_at", t)
        me.set("defenders_at_start", len(holds))
        me.set("started_by", list(me.state.get("characters") or []))
me.set("stood_on", on)
me.set("round", rnd)

room = in_room()
shooters = [w for w in room if b.meta(w).get("type") in b.SHOOTER_TYPES]
heals = [w for w in room if b.meta(w).get("type") == "heal"]
defenders = [w for w in room if b.meta(w).get("type") in b.DEFENDABLE_TYPES]
fighters = [w for w in wants if b.meta(w).get("type") == b.FIGHTER_TYPE]

# A fighter for every character, a tile standing in the room near its middle,
# where the way in is in reach — it fights what comes near, a capture's hub it
# is in first, or the one the 攻撃 skill locks on. One still hidden, as they
# were first made, is stood there once.
taken_cells = b.occupied_cells(wants) | b.blocked_cells(wants)


def near_middle():
    """The free cell in the room nearest its middle — taken once given."""
    mx, my = rx + (rw - 1) / 2, ry + (rh - 1) / 2
    free = [(x, y) for x in range(rx + 1, rx + rw - 1) for y in range(ry + 1, ry + rh - 1)
            if (x, y) not in taken_cells]
    if not free:
        return None
    cell = min(free, key=lambda c: (abs(c[0] - mx) + abs(c[1] - my), c[1], c[0]))
    taken_cells.add(cell)
    return cell


have = {b.params_of(f).get("character_id") for f in fighters}
for w in wants:
    if b.meta(w).get("type") != "character_motion":
        continue
    cid = b.params_of(w).get("character_id")
    if cid and cid not in have:
        cell = near_middle()
        labels = {b.CANVAS_X: str(cell[0]), b.CANVAS_Y: str(cell[1])} if cell else {"mywant.io/canvas-hidden": "true"}
        srv.create_want({
            "metadata": {"name": f"fighter-{cid}", "type": b.FIGHTER_TYPE, "labels": labels},
            "spec": {"params": {"character_id": cid},
                     "exposes": [{"currentState": "strike", "as": b.action_key(f"fighter-{cid}", "attack")}]},
        })
for f in fighters:
    if (b.meta(f).get("labels") or {}).get("mywant.io/canvas-hidden") == "true":
        cell = near_middle()
        if cell:
            srv.patch_want(b.meta(f)["id"], labels={
                b.CANVAS_X: str(cell[0]), b.CANVAS_Y: str(cell[1]), "mywant.io/canvas-hidden": None})

if phase != "fighting":
    # Not under way: its turrets aimed at nothing, its enemies off the board.
    for w in shooters:
        aim(w, None)
    for e in enemies:
        srv.delete_want(b.meta(e)["id"])
    me.set("phase", phase)
    me.set("fallen", {})
    me.set("failed", _failed[-5:])
    me.finish()
    raise SystemExit(0)

# ── a battle under way ───────────────────────────────────────────────────────

# What acts says so, each under its own key.
for w in shooters + heals:
    act = "heal" if b.meta(w).get("type") == "heal" else "attack"
    field = "pulse" if act == "heal" else "strike"
    srv.expose(b.meta(w)["id"], field, b.action_key(b.meta(w)["name"], act))

# What is acted on reads what can reach it — each only once there is
# something behind it, so it is never held back waiting.
live = lambda keys: [k for k in keys if gs.get(k) is not None]
blows = live([b.action_key(b.meta(e)["name"], "attack") for e in enemies]
             + [b.action_key(b.meta(h)["name"], "heal") for h in heals])
shots = live([b.action_key(b.meta(w)["name"], "attack") for w in shooters])


# Every fighter, as every turret: which enemy a blow is for is the blow's own
# `to` (fighter.py picks — its hub first), and only that one takes it.
blades = live([b.action_key(b.meta(f)["name"], "attack") for f in fighters])


def wire(w, keys, also=None):
    """w imports exactly these acts (and `also`); its other imports are its
    own. Asked of the server only when that changes (set_imports reads the
    want itself — a listing can lag behind it)."""
    imports = {k: v for k, v in ((w.get("spec") or {}).get("imports") or {}).items()
               if not v.startswith(b.ACTION_PREFIX) and v != b.ROUND_IMPORT}
    for k in keys:
        imports[k] = b.ACTION_PREFIX + k
    for k, v in (also or {}).items():
        imports[k] = v
    srv.set_imports(b.meta(w)["id"], imports)


round_also = {rkey: b.ROUND_IMPORT} if gs.get(rkey) is not None else {}
for w in defenders:
    wire(w, blows, round_also)
for e in enemies:
    wire(e, shots + blades)

targets = {b.meta(w)["name"]: w for w in defenders if running(w)}
names = sorted(targets)
if names != sorted(me.state.get("targets") or []):
    me.set("targets", names)

spawned = int(me.state.get("spawned") or 0)
defeated = int(me.state.get("defeated") or 0)
total = int(me.params.get("enemies", 5))

# An enemy brought down says so itself: counted, and left a moment for its
# fall to be seen.
standing = []
for e in enemies:
    if b.current_of(e).get("defeated"):
        fallen[b.meta(e)["id"]] = t
        defeated += 1
        aim(e, None)
    else:
        standing.append(e)
enemies = standing

# Coming in, one after another — when the way in is clear.
spawn_at = (int(me.params.get("spawn_x", 0)), int(me.params.get("spawn_y", 0)))
taken = b.occupied_cells(wants)
due = (spawned < total and t - int(me.state.get("last_spawn") or 0)
       >= float(me.params.get("spawn_every", 4)) * 1000)
if due:
    n = spawned + 1
    boss = n == total and me.params.get("boss", True) not in (False, "false")
    name = f"{me.name}-boss" if boss else f"{me.name}-enemy-{n}"
    if name in by_name:
        # Already there — a turn that brought it in did not get to say so.
        spawned = n
        me.set("last_spawn", t)
    elif spawn_at not in taken:
        labels = {b.CANVAS_X: str(spawn_at[0]), b.CANVAS_Y: str(spawn_at[1]), b.ENEMY_OF: me.id}
        if not boss:
            labels["mywant.io/canvas-size"] = "0.5"
        else:
            me.set("say", "ボスが現れた！")
        srv.create_want({
            # The round in its id: an enemy of an earlier round is another
            # want, and what was aimed at that one (a capture's hub)
            # is not this one's.
            "metadata": {"id": f"enemy-{me.id}-r{rnd}-{n}", "name": name, "type": "boss" if boss else "enemy",
                         "labels": labels},
            "spec": {
                "params": {
                    "max_hp": float(me.params.get("boss_hp" if boss else "enemy_hp", 30 if boss else 3)),
                    "damage": float(me.params.get("boss_damage" if boss else "enemy_damage", 3 if boss else 1)),
                    "interval": 1.0,
                    "speed": float(me.params.get("enemy_speed", 0.6)),
                },
                "exposes": [{"currentState": "strike", "as": b.action_key(name, "attack")}],
                "imports": {k: b.ACTION_PREFIX + k for k in shots},
            },
        })
        spawned = n
        me.set("last_spawn", t)

# Each enemy goes for the nearest want in the room.
going_for = {}
for e in enemies:
    at = b.cell_of(e)
    best, best_d = None, math.inf
    for name, w in targets.items():
        c = b.cell_of(w)
        if at and c and b.dist(at, c) < best_d:
            best, best_d = name, b.dist(at, c)
    going_for[b.meta(e)["name"]] = best
    aim(e, best)

# Each turret is aimed: at the enemy it is already on while it is in reach,
# or one its strategy picks (nearest, weakest, guard).
for w in shooters:
    if not running(w):
        continue
    at = b.cell_of(w)
    p = b.params_of(w)
    reach = float(p.get("range", 3))
    pool = [e for e in enemies if at and b.cell_of(e) and b.dist(at, b.cell_of(e)) <= reach]
    if b.meta(w).get("type") == "freeze":
        free = [e for e in pool if not b.current_of(e).get("frozen")]
        pool = free or pool
    pick = None
    cur = p.get("target")
    if any(b.meta(e)["name"] == cur for e in pool):
        pick = cur
    elif pool:
        strategy = p.get("strategy", "nearest")
        if strategy == "weakest":
            key = lambda e: float(b.current_of(e).get("hp") or 0)
        elif strategy == "guard":
            key = lambda e: float(b.current_of(targets[going_for[b.meta(e)["name"]]]).get("hp") or 0) \
                if going_for.get(b.meta(e)["name"]) in targets else math.inf
        else:
            key = lambda e: b.dist(at, b.cell_of(e))
        pick = b.meta(min(pool, key=key))["name"]
    aim(w, pick)

# A fighter locked on to one of these enemies holds while near enough; one
# gone, or too far, it lets go.
alive = {b.meta(e)["name"]: b.cell_of(e) for e in enemies}
cursors = {c.get("characterId"): (float(c.get("x", 0)), float(c.get("y", 0))) for c in cursor_list}
for f in fighters:
    on = b.params_of(f).get("target") or ""
    if not on.startswith(me.name + "-"):
        continue
    at = cursors.get(b.params_of(f).get("character_id"))
    if on not in alive or (at and alive[on] and b.dist(at, alive[on]) > CHARACTER_REACH):
        aim(f, None)

# Over: every enemy in and down, or nothing left to defend.
if spawned >= total and not enemies:
    phase = "cleared"
elif not targets and spawned > 0:
    phase = "lost"
if phase != "fighting":
    for w in shooters:
        aim(w, None)
    # Its score, and its place among the battles before it — kept in its own
    # state, which outlives a restart.
    cleared = phase == "cleared"
    survivors = len(targets)
    began = int(me.state.get("started_at") or t)
    seconds = round((t - began) / 1000, 1)
    entry = {
        "score": b.score_of(cleared, survivors, defeated, seconds, me.params),
        "result": phase,
        "survivors": survivors,
        "of": int(me.state.get("defenders_at_start") or survivors),
        "defeated": defeated,
        "seconds": seconds,
        "round": rnd,
        "by": list(me.state.get("started_by") or []),
        # UTC, said so: the script may run where the clock is set to any zone.
        "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    ranking, place = b.ranked(me.state.get("ranking"), entry, int(me.params.get("ranking_size", 10)))
    me.set("last_score", entry)
    me.set("ranking", ranking)
    head = f"守りきった！ {defeated} 体倒しました" if cleared \
        else f"全部やられた… {defeated} 体倒しました。 want を再開してもう一度"
    rank = f"{place} 位" if place else "ランク外"
    me.set("say", f"{head} — スコア {entry['score']}（{survivors}/{entry['of']} 生存・{seconds:g} 秒）{rank}")

me.set("phase", phase)
me.set("spawned", spawned)
me.set("defeated", defeated)
me.set("fallen", fallen)
me.set("failed", _failed[-5:])
me.finish()
