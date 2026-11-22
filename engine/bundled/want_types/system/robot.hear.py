#!/usr/bin/env python3
"""What the robot does with a murmur it heard (spec.hear): a free slot that
came (change: added items with a room, a date and a time — SmartGolf's free
slots) it offers to book. Asked of the person, not done: only a yes makes the
booking want. See engine/core/want_hearer.go."""
import json
import sys


def main() -> None:
    given = json.loads(sys.argv[1])
    heard = given.get("heard") or {}
    proposals = []
    for change in heard.get("change") or []:
        if change.get("kind") != "added":
            continue
        for item in change.get("items") or []:
            slot = item.get("value") or {}
            room, date, time = slot.get("room"), slot.get("date"), slot.get("time")
            if not (room and date and time and item.get("key")):
                continue
            store = slot.get("store") or room.split("/")[0]
            day = date[5:].replace("-", "/").lstrip("0")
            proposals.append({
                "action": "create_want",
                "about": item["key"],
                "want": {"type": "smartgolf_book", "name": f"{store} {day} {time}の予約",
                         "params": {"room": room, "date": date, "time": time}},
                "ask": f"{store}の{day} {time}が空きました。予約しますか？",
            })
            break  # one slot at a time: the robot asks one question at once
        if proposals:
            break
    print(json.dumps({"proposals": proposals}, ensure_ascii=False))


if __name__ == "__main__":
    main()
