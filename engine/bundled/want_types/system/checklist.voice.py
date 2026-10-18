#!/usr/bin/env python3
"""What a checklist says when someone stands on it: how many are left, or
that none are. See engine/core/want_voice.go for how a voice script is run."""
import json
import sys


def main() -> None:
    want = json.loads(sys.argv[1])
    items = want.get("state", {}).get("items") or []
    if isinstance(items, str):
        try:
            items = json.loads(items)
        except ValueError:
            items = []
    if not items:
        say = None
    else:
        left = sum(1 for i in items if isinstance(i, dict) and not i.get("done"))
        say = f"あと{left}個残ってるよ" if left else "全部終わった！"
    print(json.dumps({"say": say}, ensure_ascii=False))


if __name__ == "__main__":
    main()
