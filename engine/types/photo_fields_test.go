package types

import "testing"

const testSpec = `score | スコア, 打数, score | the total at the end of the "Player 1" row
course | コース, ゴルフ場 | the course after "Nature"
not a field line
played_at | いつ, 日時 | the date and time on the SCORE CARD line`

var testLines = []string{
	"SCORE CARD | Nature ザ・サザンリンクス GC | ［Stroke（シングル）］ | 2026-10-17 09:39:48",
	"4 2 | 3 | 4 | 5 | 6 | 7",
	"Player 1 | E | 1E | +1 | 87（+15）",
}

func TestParsePhotoFields(t *testing.T) {
	f := ParsePhotoFields(testSpec)
	if len(f) != 3 || f[0].Name != "score" || len(f[0].Words) != 3 || f[2].Name != "played_at" {
		t.Fatalf("got %+v", f)
	}
}

func TestParsePhotoFieldsAnswer(t *testing.T) {
	f := ParsePhotoFields(testSpec)
	got := ParsePhotoFieldsAnswer("score: 87（+15）\n- course: 「ザ・サザンリンクス GC」\nplayed_at: 2026-10-17 9:39\nweather: sunny", f, testLines)
	if got["score"] != "87（+15）" || got["course"] != "ザ・サザンリンクス GC" {
		t.Fatalf("got %v", got)
	}
	// Not in the words: made up, dropped. Not a field: dropped.
	if _, ok := got["played_at"]; ok {
		t.Errorf("kept a value not in the words: %v", got)
	}
	if _, ok := got["weather"]; ok {
		t.Errorf("kept a field the type does not have: %v", got)
	}
}

func TestParsePhotoFieldsAnswerWidth(t *testing.T) {
	f := ParsePhotoFields(testSpec)
	got := ParsePhotoFieldsAnswer("score: 79（+7）", f, []string{"Player 1 | E", "+1 | 79（＋7）"})
	if got["score"] != "79（＋7）" {
		t.Fatalf("got %v", got)
	}
}

func TestPhotoFieldAsked(t *testing.T) {
	f := ParsePhotoFields(testSpec)
	for q, want := range map[string]string{
		"今朝のゴルフのスコアは？": "score",
		"今朝のゴルフのコースは？": "course",
		"いつプレーした？":      "played_at",
		"今朝のゴルフの予約は？":   "",
	} {
		if got := PhotoFieldAsked(q, f); got != want {
			t.Errorf("%q: got %q, want %q", q, got, want)
		}
	}
}
