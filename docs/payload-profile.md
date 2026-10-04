# Payload profile: where the size of a want is

How big a want is when an API hands it out, and which parts of it make it big.
Written to decide what a size-limited reader should be given first — and what
can go — before building a general way to shrink responses to a token budget.

The numbers come from `tools/payload_profile.py` (see [Measuring it
yourself](#measuring-it-yourself)) run against one working board: 40 wants and
111 want types. Sizes are characters of compact JSON. They are one board's
numbers, not a law; the shape is what carries over.

## Why this matters

The robot's on-device model reads the board through tools (`engine/server/fm.go`),
and a tool's answer goes into the model's context window together with the
instructions, the tool schemas and the conversation so far:

| Where the model runs | Context window |
|---|---|
| iPhone (Apple FoundationModels) | 4,096 tokens |
| Mac (`fmtool`) | 8,192 tokens |

The instructions and tool schemas alone take roughly 2,000 tokens. When the
`look` tool was changed to return a want and its type whole (history excluded),
a reservation question on the iPhone failed before the model could answer:

```
Provided 5,327 tokens, but the maximum allowed is 4,096.
```

The same response was fine on the Mac. With the history included it failed on
both (about 13,600 tokens). So any reader with a budget needs the response cut
to fit, and cutting well needs to know where the weight is. As a rule of thumb
a model reads this JSON at about a third of a token per character.

## Wants (`GET /api/v1/wants/{id}`)

40 wants, 341,300 characters: **8,532 on average, 66,185 at most.**

| Path | Share | Largest in one want |
|---|---:|---:|
| **`history`** | **66.5%** | 53,755 |
| `history.resultHistory[]` (past results) | 40.6% | 49,313 |
| `history.stateHistory[]` (past state values) | 22.4% | 34,959 |
| `history.logHistory[]`, `history.agentHistory[]` | 2.8% | 955 |
| `state.current` (what the want says now) | 13.3% | 27,278 |
| `metadata` | 7.1% | 1,168 |
| `metadata.labels` (mostly display hints) | 3.8% | 580 |
| `state_timestamps` (when each state field changed) | 4.8% | 1,058 |
| `hidden_state` | 2.4% | 7,118 |
| `connectivity_metadata`, `hash`, `exposable_fields` | ~2.4% | 346 |
| `spec` (incl. `params`) | 1.1% | 506 |

Inside `history`, almost all of it is copies of values:

| Path inside `history` | Share of history |
|---|---:|
| `resultHistory[].fields` | 44.9% |
| `stateHistory[].stateValue` | 26.7% |
| `resultHistory[].result` | 5.7% |
| ids and timestamps of the entries | ~10% |
| `logHistory[].logs` | 2.4% |

Inside `state`, `current` is 94% — but over half of that is one field of one
want: the robot's own conversation record (`state.current.fm_turns`, 26,463
characters). Apart from it, current values are small: a few hundred characters
per want, with the odd long text (a route list, an OCR'd page, a chat buffer)
at 1–2 thousand.

The ten heaviest wants are all heavy for the same reason:

| Kind of want | Size | of which `history` |
|---|---:|---:|
| the robot | 66,185 | 36,368 (and `state` 27,537) |
| a route search | 59,894 | 53,755 |
| a picture with recognized text | 49,370 | 46,257 |
| a music player | 31,223 | 21,005 (and `hidden_state` 7,118) |
| a reservation check | 17,451 | 14,172 |
| a route search | 15,259 | 10,230 |
| a web page | 12,721 | 10,771 |
| an agent's status | 11,714 | 9,075 |
| a weather report | 9,244 | 6,801 |
| a character's chat | 7,102 | 3,091 |

What a want *is* right now — its name, type, status, current values and final
result — is typically under 1,500 characters. Everything else is what it has
been, and how the system keeps it.

## Want types (`GET /api/v1/want-types/{name}`)

111 types, 594,199 characters: **5,353 on average, 15,252 at most.**

| Path | Share |
|---|---:|
| **`state[]`** (state field definitions, 18 per type on average) | **55.1%** |
| `metadata` | 12.3% |
| `metadata.description` | 6.9% |
| `metadata.labels` | 3.0% |
| `parameters[]` | 11.6% |
| `examples[]` | 9.4% |
| `source` (where the type was installed from) | 2.9% |
| `onInitialize`, `constraints`, `connectivity`, `agents`, … | ~5% |

Within one state field definition:

| Field | Share |
|---|---:|
| `description` | 30.1% |
| `name` | 8.8% |
| `label`, `type` | 10.0% |
| `persistent`, `initialValue`, `fetchFrom`, `onFetchData`, `subType`, `example`, … | ~7% |
| (JSON keys and punctuation) | the rest |

**Eight state fields are in at least 80% of all types** — `achieved`,
`achieving_percentage`, `action_by_agent`, `agent_result`, `completed`,
`desired_dispatch`, `final_result`, `say` — and their definitions are
**41.2% of all state definitions**. They are the framework's own fields, the
same in every type, and say nothing about the type at hand.

## What it suggests

The weight and the meaning are in different places. The meaning a reader needs
to understand a want — what the type is, what each value means, what the
values are now — is small: the type's `description`, each state field's
`name` and `description`, and `state.current` / `final_result`. The weight is
in the past (`history`), in bookkeeping, and in definitions every type
repeats.

That gives an order for shrinking a response to a budget, cheapest loss first:

1. **`history`** — two thirds of a want; the past, not the present.
2. **Bookkeeping** — `state_timestamps`, `connectivity_metadata`, `hash`,
   `exposable_fields`; on a type, `source`.
3. **Display hints** — labels that only tell a GUI how to draw (colours,
   gradients, icons).
4. **The framework's shared state definitions** — the eight fields above:
   41% of a type's state definitions, the same for every type.
5. **How a type is made and run** — `examples`, `onInitialize`, and per field
   `initialValue`, `persistent`, `fetchFrom`, `onFetchData`.
6. **Long single values** — truncate a current value past a length (a chat
   record, an OCR'd page) rather than drop it.
7. **Kept to the end** — the type's `description`; each state field's `name`
   and `description`; current values and the final result.

Steps 1 and 2 alone take about 70% off an average want.

A general mechanism would take a budget on the request (for example a
`token-limit` on any GET that returns wants or types) and apply these steps in
order until the response fits, so a small client asks for what it can read
instead of every client getting the largest form.

## Measuring it yourself

`tools/payload_profile.py` needs only Python 3 and a running server.

```sh
# The local server (or $MYWANT_SERVER_URL)
python3 tools/payload_profile.py

# Another server, more rows per table, deeper paths
python3 tools/payload_profile.py --server http://host:8080 --top 40 --depth 4

# A server that asks for basic auth ($MYWANT_USER / $MYWANT_PASSWORD also work)
python3 tools/payload_profile.py --user me --password secret

# Machine-readable, e.g. to compare two runs
python3 tools/payload_profile.py --json > profile.json
```

| Option | Default | |
|---|---|---|
| `--server` | `$MYWANT_SERVER_URL` or `http://localhost:8080` | the server to read |
| `--user`, `--password` | `$MYWANT_USER`, `$MYWANT_PASSWORD` | basic auth, when the server asks |
| `--depth` | 3 | how many levels of a want's paths are broken out |
| `--top` | 25 | rows per table |
| `--heaviest` | 10 | how many of the heaviest wants to list |
| `--show-names` | off | name the heaviest wants; by default only their types are printed, so the output can be shared |
| `--json` | off | print the profile as JSON |

What it prints:

- **wants** — every path in `GET /api/v1/wants/{id}`, summed over all wants:
  total characters, share of the whole, the largest single occurrence, and how
  many times the path occurs. Array indices are folded (`history.stateHistory[]`);
  map keys are kept, so each `state.current.<field>` and
  `metadata.labels.<key>` is its own row.
- **inside history / inside state** — the same, one level further into the two
  parts that matter most.
- **want types** — every path in `GET /api/v1/want-types/{name}`, and then
  inside one state field definition.
- **heaviest wants** — the largest wants and their four largest parts.
- **shared state definitions** — the state fields at least 80% of types carry,
  and their share of all state definitions.

It only reads (GETs). A want deleted while it runs is profiled from the list's
copy; a type that cannot be read is skipped and named on stderr.
