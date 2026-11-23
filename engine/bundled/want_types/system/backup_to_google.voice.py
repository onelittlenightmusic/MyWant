#!/usr/bin/env python3
"""What the Google Drive backup says: where it stands with keeping everything
safe.

- Not signed in yet: asks for Google's consent, quietly.
- Failed: that it could not, and why, in a word.
- Done: how long ago the last copy was made, and how big it is — and, once
  that is well past its ten-minute round, that it is getting old.
- Nothing yet: that it is about to.

Its news (backed up, failed — its timings) is said the same way, from where it
stands now. Lines are written here in Japanese; the server translates them for
a character reading in another language. See engine/core/want_voice.go for how
a voice script is run.
"""
import json
import sys
from datetime import datetime, timezone

# Past this, the last copy is getting old (it runs every ten minutes).
STALE_MINUTES = 60


def flat(state: dict) -> dict:
    """The want's state, its labels (current, goal, …) folded away."""
    out = dict(state)
    for label in ("current", "goal", "plan", "internal"):
        if isinstance(state.get(label), dict):
            out.update(state[label])
    return out


def size_text(n) -> str:
    try:
        n = float(n)
    except (TypeError, ValueError):
        return ""
    if n <= 0:
        return ""
    for unit in ("B", "KB", "MB", "GB"):
        if n < 1024 or unit == "GB":
            return f"{n:.0f} {unit}" if unit == "B" else f"{n:.1f} {unit}"
        n /= 1024
    return ""


def ago_minutes(iso: str):
    try:
        t = datetime.fromisoformat(str(iso).replace("Z", "+00:00"))
    except ValueError:
        return None
    if t.tzinfo is None:
        t = t.replace(tzinfo=timezone.utc)
    return max(0, int((datetime.now(timezone.utc) - t).total_seconds() // 60))


def ago_text(m: int) -> str:
    if m < 1:
        return "いま"
    if m < 60:
        return f"{m}分前"
    if m < 60 * 24:
        return f"{m // 60}時間前"
    return f"{m // (60 * 24)}日前"


def lines(s: dict) -> tuple[list[str], str]:
    status = s.get("backup_status") or ""
    if status == "waiting_auth" or s.get("authorized") is False:
        return ["Google にサインインしてもらえる？ バックアップできるようになるよ",
                "Google の許可を待ってるよ"], "waiting"
    if status == "failed":
        err = str(s.get("error") or "").strip().splitlines()[0][:40] if s.get("error") else ""
        why = f"（{err}）" if err else ""
        return [f"バックアップできなかった…{why}", "うまく保存できなかった、もう一回やってみるね"], "worried"
    when = s.get("last_backup_at") or ""
    m = ago_minutes(when) if when else None
    size = size_text(s.get("backup_bytes"))
    of = f"（{size}）" if size else ""
    if m is None:
        return ["これからバックアップするね", "みんなの状態を Drive に保存するよ"], "eager"
    if m > STALE_MINUTES:
        return [f"最後のバックアップは{ago_text(m)}…ちょっと間があいてる", f"{ago_text(m)}から保存できてないみたい"], "worried"
    when_said = "さっき" if m < 1 else f"{ago_text(m)}に"
    return [f"{when_said}バックアップしたよ{of}", "みんなの状態、Drive にちゃんと保存してあるよ"], "calm"


def main() -> None:
    data = json.loads(sys.argv[1]) if len(sys.argv) > 1 else {}
    say, mood = lines(flat(data.get("state") or {}))
    out = {"say": say or None}
    if mood:
        out["mood"] = mood
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
