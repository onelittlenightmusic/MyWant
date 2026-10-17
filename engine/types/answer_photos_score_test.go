package types

import "testing"

func TestScoreCardTotal(t *testing.T) {
	cases := []struct {
		q     string
		lines []string
		want  string
	}{
		{"今朝のゴルフのスコアは？", []string{"4 2 | 3 | 4 | 5 | 6 | 7 | 8", "PAR | 4 | 4 | 5", "Player 1 | E | 1E | +1 | 87（+15）"}, "87（+15）"},
		{"スコアは？", []string{"Player 1 | E", "+1 | 79（＋7）"}, "79（+7）"},
		{"score?", []string{"8 71（-1）"}, "71（-1）"},
		{"いつ撮った？", []string{"Player 1 | 87（+15）"}, ""},
		{"スコアは？", []string{"PAR | 4 | 4 | 72"}, ""},
	}
	for _, c := range cases {
		if got := scoreCardTotal(c.q, c.lines); got != c.want {
			t.Errorf("%q %v: got %q, want %q", c.q, c.lines, got, c.want)
		}
	}
}
