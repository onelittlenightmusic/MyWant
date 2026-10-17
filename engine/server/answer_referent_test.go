package server

import (
	"testing"
	"time"
)

func TestReferentTime(t *testing.T) {
	now := time.Date(2026, 10, 17, 11, 3, 0, 0, time.Local)
	at := func(d, h int) time.Time { return time.Date(2026, 10, d, h, 0, 0, 0, time.Local) }
	cases := []struct {
		q        string
		phrase   string
		from, to time.Time
	}{
		{"今朝のゴルフのスコアは？", "今朝", at(17, 4), at(17, 12)},
		{"今朝の乗り換え", "今朝", at(17, 4), at(17, 12)},
		{"昨日のゴルフは？", "昨日", at(16, 0), at(17, 0)},
		{"昨日の朝の乗り換え", "昨日の朝", at(16, 4), at(16, 12)},
		{"昨夜の天気", "昨夜", at(16, 18), at(17, 4)},
		{"おとといのゴルフ", "おととい", at(15, 0), at(16, 0)},
		{"午後のゴルフ", "午後", at(17, 12), at(17, 18)},
		{"What was this morning's golf score?", "this morning", at(17, 4), at(17, 12)},
	}
	for _, c := range cases {
		phrase, from, to, ok := referentTime(c.q, now)
		if !ok || phrase != c.phrase || !from.Equal(c.from) || !to.Equal(c.to) {
			t.Errorf("%q: got %q %v–%v ok=%v, want %q %v–%v", c.q, phrase, from, to, ok, c.phrase, c.from, c.to)
		}
	}
	// No time named: a part of a day inside a name is not one.
	for _, q := range []string{"ゴルフの最新スコアは？", "朝日新聞の天気", "夜景の写真"} {
		if phrase, _, _, ok := referentTime(q, now); ok {
			t.Errorf("%q: took %q for a time", q, phrase)
		}
	}
}
