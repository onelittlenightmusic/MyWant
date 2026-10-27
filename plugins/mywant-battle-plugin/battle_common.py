"""A battle, on the board's own wiring — what every role of it shares.

Every want in a battle is in charge of itself. One that acts — a heal's
mend, a turret's or freeze's shot, an enemy's blow, a fighter's attack —
decides how much and how often (its params, its interval) and puts each act
in its own state: one object, numbered (`seq`), saying who it is for (`to`:
a list of names; for a heal, each name's share). That field is exposed under
a key of its own that does not depend on whom it is for — "<name>.attack",
"<name>.heal" — so it can be imported before anyone knows what it will be
aimed at.

One that is acted on imports those keys (under a local key starting
ACTION_PREFIX) and reads them in its own turn: an act numbered past the last
one it took, with its own name in `to`, it takes — its own hp down or up,
never past its max_hp; a freeze's hold, a while. At hp 0 it stops itself.
Nothing writes another want's state.

The wiring — what each exposes, what each imports, what each is aimed at —
is configuration, which the room defense asks the server for as the third
party that joins them (room.py).

Each role's script runs once per turn, so what it must remember between
turns (the last act sent, the acts already taken, when it last stepped) is
kept in its own state too — handed back with its answer.
"""

from __future__ import annotations

import math
import time
from typing import Iterable, Optional

ACTION_PREFIX = "action:"
ROUND_IMPORT = "battle_round"

ENEMY_TYPES = ("enemy", "boss")
DEFENDABLE_TYPES = ("treasure", "turret", "freeze", "heal")
SHOOTER_TYPES = ("turret", "freeze")
FIGHTER_TYPE = "fighter"

CANVAS_X = "mywant.io/canvas-x"
CANVAS_Y = "mywant.io/canvas-y"
ENEMY_OF = "mywant.io/enemy-of"


def action_key(name: str, act: str) -> str:
    return f"{name}.{act}"


def round_key(battle: str) -> str:
    return f"{battle}.round"


def now_ms() -> int:
    return int(time.time() * 1000)


# ── the board ────────────────────────────────────────────────────────────────

def meta(w: dict) -> dict:
    return w.get("metadata") or {}


def current_of(w: dict) -> dict:
    return ((w.get("state") or {}).get("current")) or {}


def params_of(w: dict) -> dict:
    return ((w.get("spec") or {}).get("params")) or {}


def cell_of(w_or_labels) -> Optional[tuple]:
    labels = w_or_labels if "metadata" not in w_or_labels else (meta(w_or_labels).get("labels") or {})
    try:
        return int(labels[CANVAS_X]), int(labels[CANVAS_Y])
    except (KeyError, TypeError, ValueError):
        return None


CONSTELLATION = "constellation/"
CONSTELLATION_CAPTURE = "constellation-capture/"


def tied(f_id: str, f_labels: dict, e_id: str, e_labels: dict) -> bool:
    """Whether a fighter is tied to an enemy by a constellation they are both
    in. An order (tied one after another) ties every fighter in it to every
    enemy in it; a capture ties only to the tile it was caught into — an enemy
    the net was aimed at is hit by each fighter caught after, a fighter it was
    aimed at hits each enemy caught after (the engine's
    constellation_capture.go)."""
    f_labels, e_labels = f_labels or {}, e_labels or {}
    # The enemy a fighter's capture is into is its, whether or not the enemy
    # carries the constellation itself: the hub is not a member (Here and the
    # carried ring move the hub without making it one), and the room labels
    # its enemies afresh.
    for k, v in f_labels.items():
        if k.startswith(CONSTELLATION_CAPTURE) and v == e_id \
                and f_labels.get(CONSTELLATION + k[len(CONSTELLATION_CAPTURE):]) == "true":
            return True
    for k, v in f_labels.items():
        if not k.startswith(CONSTELLATION) or v != "true" or e_labels.get(k) != "true":
            continue
        name = k[len(CONSTELLATION):]
        hub = f_labels.get(CONSTELLATION_CAPTURE + name) or e_labels.get(CONSTELLATION_CAPTURE + name)
        if not hub or hub in (f_id, e_id):
            return True
    return False


def dist(a: tuple, b: tuple) -> float:
    return math.hypot(a[0] - b[0], a[1] - b[1])


def footprint(labels: dict) -> list:
    """The cells a tile covers: its anchor and, stretched, the run of cells
    its rotation and length say — the server's tileFootprint."""
    c = cell_of(labels)
    if c is None:
        return []
    try:
        rot = int(labels.get("mywant.io/canvas-rotation") or 0)
    except ValueError:
        rot = 0
    try:
        length = int(labels.get("mywant.io/canvas-length") or 0)
    except ValueError:
        length = 0
    ax, ay = c
    step = {90: (0, 1), 180: (-1, 0), 270: (0, -1)}.get(rot, (1, 0))
    return [(ax + step[0] * i, ay + step[1] * i) for i in range(length + 1)]


def blocked_cells(wants: Iterable[dict]) -> set:
    """Where nothing may stand: walls, and doors while locked."""
    cells = set()
    for w in wants:
        m = meta(w)
        t = m.get("type")
        labels = m.get("labels") or {}
        if labels.get("mywant.io/canvas-hidden") == "true":
            continue
        if t == "wall" or (t in ("door", "rpg_door") and current_of(w).get("locked") is True):
            cells.update(footprint(labels))
    return cells


def occupied_cells(wants: Iterable[dict]) -> set:
    """Where a tile stands — its anchor cell; a wall's whole run is
    blocked_cells'."""
    cells = set()
    for w in wants:
        labels = meta(w).get("labels") or {}
        if labels.get("mywant.io/canvas-hidden") == "true":
            continue
        c = cell_of(labels)
        if c is not None:
            cells.add(c)
    return cells


def step_toward(frm: tuple, to: tuple, blocked, reach: float = 1.5, margin: int = 12) -> Optional[tuple]:
    """The next cell on a shortest way from `frm` to somewhere within `reach`
    of `to` — round walls and other tiles, through a room's way in — or None
    when there is no way (or `frm` is already there). A search of the board
    within `margin` cells of the two, so it stays small."""
    from collections import deque
    if dist(frm, to) <= reach:
        return None
    lo_x, hi_x = min(frm[0], to[0]) - margin, max(frm[0], to[0]) + margin
    lo_y, hi_y = min(frm[1], to[1]) - margin, max(frm[1], to[1]) + margin
    first = {frm: None}
    q = deque([frm])
    while q:
        c = q.popleft()
        if c != frm and dist(c, to) <= reach:
            # Walk back to the step out of frm.
            while first[c] != frm and first[c] is not None:
                c = first[c]
            return c
        for d in ((1, 0), (-1, 0), (0, 1), (0, -1)):
            n = (c[0] + d[0], c[1] + d[1])
            if n in first or not (lo_x <= n[0] <= hi_x and lo_y <= n[1] <= hi_y) or blocked(n):
                continue
            first[n] = c
            q.append(n)
    return None


# ── the score ────────────────────────────────────────────────────────────────

def score_of(cleared: bool, survivors: int, defeated: int, seconds: float, params: dict) -> int:
    """A battle's score, once it is over: each want in the room still standing,
    each enemy brought down, and — only for one held to the end — each second
    it was over before its par."""
    per_survivor = float(params.get("score_per_survivor", 1000))
    per_defeat = float(params.get("score_per_defeat", 100))
    par = float(params.get("score_par_seconds", 120))
    per_second = float(params.get("score_per_second", 10))
    score = survivors * per_survivor + defeated * per_defeat
    if cleared:
        score += max(0.0, par - seconds) * per_second
    return int(round(score))


def ranked(ranking, entry: dict, size: int) -> tuple:
    """The ranking with `entry` in its place — highest score first, the
    quicker of two the same — kept to `size`; and entry's place (1 is the top),
    or 0 when it did not make it."""
    rows = [r for r in (ranking or []) if isinstance(r, dict)] + [entry]
    rows.sort(key=lambda r: (-int(r.get("score", 0)), float(r.get("seconds", 0))))
    rows = rows[:max(size, 1)]
    place = next((i + 1 for i, r in enumerate(rows) if r is entry), 0)
    return rows, place


# ── acts ─────────────────────────────────────────────────────────────────────

def send(me, field: str, interval: float, body: dict, to, impact_kind: str = "", on=()):
    """The next act in `field` when `interval` seconds have passed since the
    last (its clock kept in `<field>_clock`). Whether it went."""
    clock = me.state.get(field + "_clock") or {}
    last = clock.get("at", 0)
    seq = int(clock.get("seq", 0))
    t = now_ms()
    if t - last < max(interval, 0.1) * 1000:
        return False
    seq += 1
    act = dict(body)
    act["seq"] = seq
    act["to"] = to
    me.set(field, act)
    me.set(field + "_clock", {"at": t, "seq": seq})
    if impact_kind and on:
        me.set("impact", {"kind": impact_kind, "to": list(on), "n": t})
    return True


def start(me, field: str):
    """A sender's field in place from the first — numbered 0, for no one —
    so what imports it has a value to read."""
    if not isinstance(me.state.get(field), dict):
        me.set(field, {"seq": 0, "to": []})


def take(me):
    """Every new act imported for this want: a list of
    {from, damage, amount, hold}. Remembered in `inbox` (sender key → the
    last number taken); what was there before it listened is not for it."""
    seen = dict(me.state.get("inbox") or {})
    name = me.name
    got = []
    changed = False
    for local, value in list(me.state.items()):
        if not local.startswith(ACTION_PREFIX) or not isinstance(value, dict):
            continue
        key = local[len(ACTION_PREFIX):]
        seq = int(value.get("seq") or 0)
        if key in seen and seen[key] == seq:
            continue
        first = key not in seen
        seen[key] = seq
        changed = True
        if first or seq == 0:
            continue
        to = value.get("to")
        act = {"from": key.rsplit(".", 1)[0], "damage": 0.0, "amount": 0.0, "hold": 0.0}
        if isinstance(to, list):
            if name not in to:
                continue
            act["damage"] = float(value.get("damage") or 0)
            act["hold"] = float(value.get("hold") or 0)
        elif isinstance(to, dict):
            if name not in to:
                continue
            act["amount"] = float(to.get(name) or 0)
        else:
            continue
        got.append(act)
    if changed:
        me.set("inbox", seen)
    return got


def keep_hp(me, fallback: float = 10.0) -> tuple:
    """Its hp and max_hp — put in place, whole, when it has none."""
    hp = me.state.get("hp")
    mx = me.state.get("max_hp")
    if not isinstance(hp, (int, float)):
        mx = float(me.params.get("max_hp", fallback))
        hp = mx
        me.set("hp", hp)
        me.set("max_hp", mx)
    if not isinstance(mx, (int, float)):
        mx = float(hp)
        me.set("max_hp", mx)
    return float(hp), float(mx)


def take_hits(me) -> bool:
    """What has been sent to this want: blows off its hp, mends onto it,
    never past its max_hp, nothing at all once it is down — and a new round
    of its battle makes it whole. At 0 it stops itself. Whether it is down."""
    hp, mx = keep_hp(me)
    rnd = me.state.get(ROUND_IMPORT)
    if isinstance(rnd, (int, float)):
        had = me.state.get("round")
        if had != rnd:
            me.set("round", int(rnd))
            if had is not None:
                mx = float(me.params.get("max_hp", mx))
                hp = mx
                me.set("max_hp", mx)
    for a in take(me):
        if hp <= 0:
            break
        hp = max(0.0, min(mx, hp - a["damage"] + a["amount"]))
    hp = round(hp, 2)
    if hp != me.state.get("hp"):
        me.set("hp", hp)
    if hp <= 0:
        me.suspend("worn down")
        return True
    return False
