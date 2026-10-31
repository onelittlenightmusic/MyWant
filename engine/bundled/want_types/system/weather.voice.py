#!/usr/bin/env python3
"""What a weather want says when someone stands on it: how today feels — warm,
cold, rain coming — rather than the numbers its card already shows.
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


def main() -> None:
    want = json.loads(sys.argv[1])
    answer = (want.get("event") or {}).get("result")
    print(json.dumps({"say": lines(want.get("state") or {}, answer) or None}, ensure_ascii=False))


if __name__ == "__main__":
    main()
