# Kata Notation

型 (kata) are what the board recognises you as having done. A kata is a named
combination of 所作 (waza) — conditions on the wants and values that are
actually there — and when they all hold at once, the kata is 極まった: credited
once, and credited again each time you hold it somewhere new.

Nothing here gates a feature. Every want type stays usable from day one. What a
kata grants is 手数 (shortcuts), 権限 (autonomy) and 語彙 (vocabulary), and what
it leaves behind is a mark on the board.

Seeds live in `engine/bundled/kata/*.yaml`. One file may carry both `levels:`
and `kata:`.

**"Recipe" is not part of this vocabulary.** `WantRecipe` / `ChildRecipe` are a
different mechanism (reproducing a previous arrangement). A kata is a
*discovery*, not a factory: holding one produces nothing.

---

## A file

```yaml
levels:
  - id: level-3
    name: 緑帯
    grade: 3rd kyu
    order: 3
    theme: Overlapping
    subtitle: Two wants about the same place
    color: "#22c55e"       # the belt's own colour
    accent: "#15803d"      # ink for marks against a pale card
    unlocked: false        # only the first belt starts open
    kata: [kata-kasa, kata-koku, kata-zeni]
    promotion:
      requiredKata: 3      # hold this many to open the next belt

kata:
  - id: kata-koku
    name: 刻
    reading: koku
    level: level-3
    order: 2
    intent: Overlap a route and a reminder on the same destination
    yields: A departure time worked backwards from when you want to arrive
    contains: [kata-ate]
    join: { kind: thing_group }
    waza:
      - { kind: thing, subtype: station }
      - { kind: want_type, type: transit_search, join: to }
      - { kind: want_type, type: reminder,
          paramFrom: { type: transit_search, state: departure, into: event_time } }
    mark: { label: 刻, icon: AlarmClock }
    mastery: { shoden: 1, kaiden: 3 }
    unlocks:
      shoden: { vocabulary: ["The pair earns a name"] }
      kaiden: { shortcuts: ["Placing the route brings the departure alert with it"] }
```

## Kata fields

| field | meaning |
|:---|:---|
| `id` | stable identifier, `kata-<reading>` by convention |
| `name` / `reading` | the kanji it is called by, and how to say it |
| `level` | which belt it belongs to |
| `order` | position within the belt |
| `intent` / `yields` | what it is for, and what holding it hands you |
| `contains` | lower kata this one subsumes (shown as 前提の型 in the sidebar) |
| `join` | `{kind: thing_group}` — all 所作 must resolve inside ONE constellation |
| `waza` | the 所作 (below) |
| `henka` | variations: alternative 所作 lists, any one of which counts |
| `mark` | what it leaves on the board (below) |
| `hidden` / `veiled` | the two kinds of secret (below) |
| `mastery` | `{shoden: N, kaiden: M}` — practice counts for 初伝 / 皆伝 |
| `unlocks` | per rank: `shortcuts` / `autonomy` / `vocabulary` |

### `join`

Without it, each 所作 is measured against the whole board. With
`join: {kind: thing_group}`, they must all hold inside one **constellation** —
the group of values the user has named as one thing. That is what makes 傘 mean
"a route and a forecast about the same place" rather than "a route somewhere
and a forecast somewhere".

A value that belongs to no constellation stands as a scope of its own, so a
kata that needs one value and one want does not require the user to make a
group first.

## Waza

Every waza is one condition. `kind` picks which.

### `kind: thing`

```yaml
- { kind: thing, subtype: station }
- { kind: thing, subtype: station, minCount: 2 }   # TWO stations, one 所作
```

Inside a join this asks "does this constellation hold a value of this subtype".
Ungrouped, it counts the values remembered in the thing store.

**Write "two of the same" as one waza with `minCount`, never as the waza
twice.** Two copies would both be answered by the same single station, and the
form would stand on half of itself.

### `kind: want_type`

```yaml
- { kind: want_type, type: weather }                    # 極まった, anywhere
- { kind: want_type, type: budget, status: any }        # merely deployed
- { kind: want_type, type: transit_search, join: to }   # …whose `to` is in this group
- { kind: want_type, type: reminder, count: 2 }         # two of them
```

| field | meaning |
|:---|:---|
| `type` | the want type name |
| `status` | `""`/`achieved` (default), `any`, or an exact status |
| `count` | how many are needed (default 1) |
| `join` | the PARAMETER whose value must belong to the joined constellation |

`status: any` is for wants that never finish — a watcher such as `budget` sits
at `reaching` and keeps aggregating, so asking for a 極まった one is a wait with
no end.

`join` is what ties a want to the group: `join: to` means "the want's `to`
parameter names one of this constellation's values".

### Relations: `importFrom`, `paramFrom`, `owns`

`join` says two wants are *about* the same value. These say one is **fed by**
the other — a real connection on the board, not a coincidence of proximity.

```yaml
# The consumer's STATE is fed by the provider's state
- { kind: want_type, type: weather_effect,
    importFrom: { type: weather, state: weather_condition, into: weather_condition } }

# The consumer's PARAMETER is fed by the provider's state
- { kind: want_type, type: reminder,
    paramFrom: { type: transit_search, state: departure, into: event_time } }

# The consumer is the PARENT of the provider
- { kind: want_type, type: budget, status: any, owns: { type: transit_search } }
```

| field | meaning |
|:---|:---|
| `type` | the want type at the other end |
| `state` | the state field that travels |
| `into` | which inlet here receives it. Optional; without it any inlet counts |

**`importFrom` vs `paramFrom` is not a style choice.** A want's state and its
parameters are filled by different mechanisms, and a want's own code reads one
or the other — see *Taking a Value From Another Want* in `want-system.md`. A
reminder reads `event_time` with `GetStringParam`, so `importFrom` there would
match a wire that never changes when the alarm goes off. Check which the want
actually reads.

`owns` is for the wants that are fed by what is under them rather than by a
wire — a `budget` adds up the costs its children report, so what 銭 needs is not
a budget beside the route but a budget the route is under.

When an earlier waza has already settled **which** wants of the provider's type
count (the route that arrives in *this* group, say), the relation must hold
against one of those, not against any want of that type elsewhere.

### `kind: repeat`

```yaml
- { kind: repeat, kata: kata-eki, minCount: 3 }
```

Satisfied out of the record book: another kata has been 極まった that many
times. It has nothing on the board, so it is never drawn and never suggested.

## `mark` — what a kata leaves behind

```yaml
mark: { label: 線, icon: TrainTrack, form: rail }
```

| field | meaning |
|:---|:---|
| `label` | the word the mark carries (usually the kata's own name) |
| `icon` | a lucide icon name — drawn on the constellation's dot, on the kata card above the name, and in the Thing panel's theme header |
| `form` | optional: redraws the constellation's LINE itself, by id in the GUI's constellation-form registry (`rail` draws the map symbol for a railway) |

## Secrets: `hidden` and `veiled`

Two different things, and the difference is the point.

```yaml
hidden: true    # 口伝 — the kata is not listed at all until it is nearly held
veiled: true    # 合  — it IS listed, with the right number of blanks
```

A **口伝** hides that the form exists: you cannot grind for what you have never
heard of. It appears once it is complete or one 所作 away.

A **veiled** form is the opposite kind of mystery. "There is a form of two 所作
here" is exactly the invitation; which two is what you go and find. It ships
with its 所作 blanked, its name replaced by `？`, and its mark drawn out of
focus — a shape you can almost read. Lifted for good the first time it is
held, because a form you have held is one you know.

Neither ships a suggestion (see below): "put a city next to that station and
you will find something" is the answer to the question the veil is asking.

## Suggestions

The engine offers the move that would finish a form the player already knows,
computed with the same evaluation and shipped on `/api/v1/kata` as
`suggestions[]`. It is only ever an offer — nothing is created, and an offer
ignored costs nothing.

The rules are deliberately narrow:

- the kata has **three or more** 所作 and exactly one is unsatisfied. A form of
  two is one thing plus one other thing, so every value on the board is one
  move from it and the offers come in dozens
- it is measured inside a named scope
- something of that scope is **on the board**, so the offer has somewhere to
  start
- for a missing thing: something that fits is on the board too, and only the
  nearest such pair is offered
- for a missing want: the offer names the type, and anchors on the want it
  would be related to when the waza names one
- masked kata offer nothing

The hint says what is actually left to do, which depends on what is there:

```
Place a reminder fed by the transit_search's departure     (no reminder at all)
Feed the transit_search's departure into the reminder's event_time  (one, unwired)
Put the transit_search under the budget                    (owns, unparented)
```

## Where the vocabulary comes from

| term | meaning |
|:---|:---|
| 型 kata | a named combination |
| 所作 waza | one condition inside it |
| 変化 henka | a variation of the combination |
| 帯 obi / level | a belt: a group of kata, opened by clearing the one below |
| 極まる | to hold the form — every 所作 satisfied at once |
| 練度 | how many times, and where: 初伝 (shoden) → 皆伝 (kaiden) |
| 口伝 kuden | a form not spoken of until you are near it |
