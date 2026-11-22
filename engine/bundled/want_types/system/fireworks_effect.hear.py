#!/usr/bin/env python3
"""What a fireworks effect does with a murmur it heard (spec.hear): it goes
off — a list all done, anything it was set to hear. Done at once, unasked: it
only plays on the board. See engine/core/want_hearer.go."""
import json
import sys


def main() -> None:
    given = json.loads(sys.argv[1])
    heard = given.get("heard") or {}
    who = heard.get("from_want_name") or ""
    print(json.dumps({
        "do": [{"action": "webhook", "payload": {"action": "trigger"}}],
        "say": f"「{who}」、おつかれさま！" if who else "おめでとう！",
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
