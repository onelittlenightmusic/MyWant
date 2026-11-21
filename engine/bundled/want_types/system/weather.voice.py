#!/usr/bin/env python3
"""What a weather want says when someone stands on it: how today feels — warm,
cold, rain coming — rather than the numbers its card already shows. And, when
it heard another want's murmur (event "heard", spec.hear) — a reservation made
nearby — its answer to it.
See engine/core/want_voice.go for how a voice script is run."""
import json
import re
import sys

# The first temperature in weather_text is now; 最高/最低 follow when the
# source gave them: "快晴 19°C (湿度 73%) (最高 23° / 最低 12°)".
TEMP = re.compile(r"(-?\d+(?:\.\d+)?)\s*°")
HIGH = re.compile(r"最高\s*(-?\d+(?:\.\d+)?)")
LOW = re.compile(r"最低\s*(-?\d+(?:\.\d+)?)")

# How a temperature feels, from the top: the first step it reaches.
FEEL = [
    (30, "暑い…溶けちゃいそう"),
    (25, "今日は暑いね"),
    (20, "ぽかぽかあったかいよ"),
    (15, "過ごしやすい日だよ"),
    (10, "ちょっと肌寒いかも"),
    (5, "寒いね、上着がいるよ"),
]
FREEZING = "さむい… ぶるぶる"

# What the sky makes it say (weather_condition, see weather_types.go).
SKY = {
    "sunny": "いい天気！",
    "cloudy": "今日はくもりだよ",
    "rain": "傘を忘れないでね",
    "snow": "雪だ！ 足元に気をつけて",
    "storm": "雷が鳴ってる… こわい",
    "fog": "霧で前が見えないよ",
}


def number(pattern: re.Pattern, text: str):
    m = pattern.search(text)
    return float(m.group(1)) if m else None


def feel(temp: float) -> str:
    for at, line in FEEL:
        if temp >= at:
            return line
    return FREEZING


def lines(state: dict, answer=None) -> list:
    """Every line says how it feels; the sky and a cold night ride along.
    A new answer (the news, event.result) is read in place of the state's."""
    text = answer if isinstance(answer, str) and answer else state.get("weather_text") or ""
    if not text:
        return ["今日は空が見えなかった"] if state.get("error") else []
    sky = SKY.get(state.get("weather_condition") or "")
    temp = number(TEMP, text)
    if temp is None:
        return [sky] if sky else []
    now = f"{temp:.0f}°で{feel(temp)}"
    # A sky that ends on its own mark is a sentence of its own, a space after
    # it so the bubble breaks there; one that does not takes a comma, or it
    # reads as one word ("傘を忘れないでね12°で…").
    joined = f"{sky} {now}" if sky and sky[-1] in "！？。…" else f"{sky}、{now}"
    out = [joined if sky else now]
    high, low = number(HIGH, text), number(LOW, text)
    if high is not None and low is not None and high - low >= 10:
        out.append(f"{now}、朝晩は冷えるよ（最低{low:.0f}°）")
    return out


# When the reservation another want just made is (its answer's datetime):
# "2026-11-22 10:00" → "11/22 10:00".
WHEN = re.compile(r"(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})")


def heard_reply(state: dict, heard: dict) -> list:
    """What it says to a murmur it heard (spec.hear): a reservation made
    nearby — when it is, and how the sky is now, to go out in. Anything else
    it heard, how the sky is."""
    now = lines(state)
    sky = now[0] if now else ""
    result = heard.get("result")
    labels = heard.get("labels") or {}
    when = ""
    # The murmur's set labels first (time: RFC3339, place), then its answer.
    m = WHEN.search(str(labels.get("time") or ""))
    if m:
        when = f"{int(m.group(2))}/{int(m.group(3))} {m.group(4)}:{m.group(5)}"
    if labels.get("place"):
        when = f"{when} {labels['place']}".strip()
    if not when and isinstance(result, dict):
        m = WHEN.search(str(result.get("datetime_rfc3339") or result.get("datetime") or ""))
        if m:
            when = f"{int(m.group(2))}/{int(m.group(3))} {m.group(4)}:{m.group(5)}"
    who = heard.get("from_want_name") or ""
    # The forecast for then and there, looked up in its Progress
    # (heard_forecast, for this murmur): said in place of the sky now.
    fc = state.get("heard_forecast") or {}
    then = ""
    if isinstance(fc, dict) and fc.get("murmur_id") == heard.get("murmur_id") and fc.get("text"):
        t = number(TEMP, fc["text"])
        cond = fc["text"].split(" ")[0]
        then = f"その時間の{fc.get('city') or ''}は{cond}"
        if t is not None:
            then += f"、{t:.0f}°で{feel(t)}"
        rain = re.search(r"降水 (\d+)%", fc["text"])
        if rain and int(rain.group(1)) >= 30:
            then += f"（降水{rain.group(1)}%、傘があると安心）"
    if heard.get("from_want_type", "").startswith("smartgolf"):
        head = f"{when}の予約、入ったね！" if when else "予約、入ったね！"
        if then:
            return [f"{head} {then}"]
        return [f"{head} 今は{sky}" if sky else head]
    if then:
        return [f"「{who}」、聞いたよ。{then}"]
    if heard.get("from_want_type") == "weather":
        # Another sky: how it is there, and how it is here.
        there = labels.get("place") or who
        return [f"{there}の空、聞いたよ。こっちは{sky}" if sky else f"{there}の空、聞いたよ"]
    return [f"「{who}」、聞いたよ。今は{sky}" if sky else f"「{who}」、聞いたよ"]


def main() -> None:
    want = json.loads(sys.argv[1])
    event = want.get("event") or {}
    if event.get("kind") == "heard" and isinstance(event.get("heard"), dict):
        print(json.dumps({"say": heard_reply(want.get("state") or {}, event["heard"]) or None}, ensure_ascii=False))
        return
    answer = event.get("result")
    print(json.dumps({"say": lines(want.get("state") or {}, answer) or None}, ensure_ascii=False))


if __name__ == "__main__":
    main()
