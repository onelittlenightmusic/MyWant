#!/usr/bin/env python3
"""What a reminder does with a murmur it heard (spec.hear): for each item with
a time that came (change: added — a reservation made), it proposes a reminder
an hour before; for each that went (removed — cancelled), it proposes taking
away the reminder it made for it. Asked of the person, not done. Whatever
type said it: the item's set labels (time, place) and key are all it reads.
See engine/core/want_hearer.go."""
import json
import sys
from datetime import datetime, timezone

AHEAD = "60 minutes"


def when(label):
    try:
        t = datetime.fromisoformat(label)
    except (TypeError, ValueError):
        return None
    return t if t.tzinfo else None  # a reminder needs an instant


def main() -> None:
    given = json.loads(sys.argv[1])
    heard = given.get("heard") or {}
    who = heard.get("from_want_name") or "予定"
    now = datetime.now(timezone.utc)
    proposals = []
    for change in heard.get("change") or []:
        for item in change.get("items") or []:
            labels = item.get("labels") or {}
            t = when(labels.get("time"))
            key = item.get("key")
            if t is None or not key:
                continue
            place = labels.get("place") or ""
            at = t.strftime("%-m/%-d %H:%M")
            what = f"{place}の{at}" if place else at
            if change.get("kind") == "added" and t > now:
                proposals.append({
                    "action": "create_want",
                    "about": key,
                    "want": {
                        "type": "reminder",
                        "name": f"{what}のリマインダー",
                        "params": {"message": f"{what}（{who}）", "event_time": t.isoformat(),
                                   "ahead": AHEAD, "require_reaction": False},
                    },
                    "ask": f"「{who}」に{what}が入りました。1時間前のリマインダーを入れますか？",
                })
            elif change.get("kind") == "removed":
                proposals.append({
                    "action": "delete_wants",
                    "about": key,
                    "ask": f"「{who}」の{what}がなくなりました。そのリマインダーを消しますか？",
                })
    out = {"proposals": proposals}
    if proposals:
        out["say"] = "リマインダー、どうする？"
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
