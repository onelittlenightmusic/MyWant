"""A treasure's turn: what is sent to it, taken — its own hp, and at 0 it
stops itself (battle_common.take_hits)."""

from mywant import current

import battle_common as b

me = current()
b.take_hits(me)
me.finish()
