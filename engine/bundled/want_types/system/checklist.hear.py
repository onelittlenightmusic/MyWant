#!/usr/bin/env python3
"""What a checklist does with a murmur it heard (spec.hear): the things the
speaker advised taking along (advice of kind "bring") that are not on the list
yet, each proposed as an item — asked of the person, not added. See
engine/core/want_hearer.go for how a hear script is run."""
import json
import sys
import time


def main() -> None:
    given = json.loads(sys.argv[1])
    want = given.get("want") or {}
    heard = given.get("heard") or {}
    items = (want.get("state") or {}).get("items") or []
    if isinstance(items, str):
        try:
            items = json.loads(items)
        except ValueError:
            items = []
    # An item is {id, text, done}, or — as a list is first given — its text.
    have = {str(i.get("text", "") if isinstance(i, dict) else i).strip() for i in items}

    proposals = []
    for advice in heard.get("advice") or []:
        if advice.get("kind") != "bring":
            continue
        item = str(advice.get("item", "")).strip()
        if not item or item in have:
            continue
        why = str(advice.get("why", "")).strip()
        who = heard.get("from_want_name") or "天気"
        reason = f"「{who}」によると{why}。" if why else f"「{who}」からの話です。"
        proposals.append({
            "action": "append",
            "field": "items",
            "value": {"id": f"item-{int(time.time() * 1000)}-{len(proposals)}", "text": item, "done": False},
            "ask": f"{reason}「{want.get('name', 'チェックリスト')}」に「{item}」を足しますか？",
            "why": why,
        })
        have.add(item)

    out = {"proposals": proposals}
    if proposals:
        out["say"] = "、".join(p["value"]["text"] for p in proposals) + "、持っていく？"
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
