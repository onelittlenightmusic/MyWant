package server

import (
	"strings"
	"testing"
	"time"

	mywant "mywant/engine/core"
)

// A capability's skill runs only from inside its type's own directory.
func TestFMCapSkillPath(t *testing.T) {
	dir := "/home/me/.mywant/custom-types/golf"
	if p, ok := fmCapSkillPath(dir, "score.py"); !ok || p != dir+"/score.py" {
		t.Errorf("in the directory: %q %v", p, ok)
	}
	if p, ok := fmCapSkillPath(dir, "tools/score.py"); !ok || p != dir+"/tools/score.py" {
		t.Errorf("below it: %q %v", p, ok)
	}
	for _, bad := range []string{"../other/x.py", "/usr/bin/env", "tools/../../x.py", ".", ""} {
		if p, ok := fmCapSkillPath(dir, bad); ok {
			t.Errorf("%q let out as %q", bad, p)
		}
	}
	if _, ok := fmCapSkillPath("", "score.py"); ok {
		t.Error("a type with no directory runs nothing")
	}
}

// Its answers, newest first, the one that is its answer now said so.
func TestFMCapAnswersText(t *testing.T) {
	w := mywant.NewWantWithLocals(mywant.Metadata{Name: "golf"}, mywant.WantSpec{}, nil, "base")
	at := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	w.RestoreResultHistory([]mywant.ResultHistoryEntry{
		{Timestamp: at("2026-11-01T07:00:00+09:00"), Result: map[string]any{"store": "A"}},
		{Timestamp: at("2026-11-03T12:00:00+09:00"), Result: map[string]any{"store": "B"}, Current: true},
	})
	got := fmCapAnswersText(w)
	a, b := strings.Index(got, `"A"`), strings.Index(got, `"B"`)
	if a < 0 || b < 0 || b > a {
		t.Fatalf("newest first: %q", got)
	}
	if !strings.Contains(got, "(its answer now)") || strings.Count(got, "(its answer now)") != 1 {
		t.Errorf("the current one marked: %q", got)
	}
}
