package mywant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// voiceWant is a want with n answers, the latest arriving at latest.
func voiceWant(t *testing.T, id string, n int, latest time.Time) *Want {
	t.Helper()
	w := NewWantWithLocals(Metadata{Name: "w-" + id, ID: id, Type: "voice_test"}, WantSpec{}, nil, "base")
	for i := 0; i < n; i++ {
		at := latest.Add(time.Duration(i-n+1) * time.Minute)
		w.AddResultHistory(ResultHistoryEntry{Result: fmt.Sprintf("answer-%d", i), Timestamp: at})
	}
	return w
}

func TestDefaultVoiceLines(t *testing.T) {
	cases := []struct {
		n    int
		want string // one of the lines, "" for none
	}{
		{0, "はじめての仕事だ"}, {4, "まだ4回だけ、覚えてる途中"}, {5, "だんだん慣れてきたよ"}, {9, "だんだん慣れてきたよ"},
		{12, "12回こなしたよ"}, {15, "いい感じに回ってる"}, {20, "ベテランです"},
	}
	for _, c := range cases {
		lines := defaultVoiceLines(c.n)
		if c.want == "" {
			if len(lines) != 0 {
				t.Errorf("n=%d: no voice yet, got %v", c.n, lines)
			}
			continue
		}
		found := false
		for _, l := range lines {
			found = found || l == c.want
		}
		if !found {
			t.Errorf("n=%d: %q not among %v", c.n, c.want, lines)
		}
	}
}

// Where a want stands comes before how much it has done: waiting, failed or
// put away it says that; reaching it talks about its experience; finished
// early on it says so; deleting it says nothing.
func TestDefaultVoiceFor(t *testing.T) {
	cases := []struct {
		status     WantStatus
		experience int
		want       string // one of the lines, "" for none
	}{
		{WantStatusIdle, 20, "準備してるよ"},
		{WantStatusWaitingUserAction, 20, "あなたの返事を待ってるよ"},
		{WantStatusFailed, 0, "うまくいかなかった…"},
		{WantStatusArchived, 3, "しまわれてます"},
		{WantStatusReaching, 0, "はじめての仕事だ"},
		{WantStatusReaching, 20, "ベテランです"},
		{WantStatusAchieved, 2, "できたよ"},
		{WantStatusAchieved, 20, "ベテランです"},
		{WantStatusReachingWithWarning, 12, warningVoice},
		{WantStatusReachingWithWarning, 12, "調子いいよ"},
		{WantStatusAchievedWithWarning, 0, "できたよ"},
		{WantStatusDeleting, 20, ""},
	}
	for _, c := range cases {
		lines := defaultVoiceFor(c.status, c.experience)
		if c.want == "" {
			if len(lines) != 0 {
				t.Errorf("%s/%d: nothing to say, got %v", c.status, c.experience, lines)
			}
			continue
		}
		found := false
		for _, l := range lines {
			found = found || l == c.want
		}
		if !found {
			t.Errorf("%s/%d: %q not among %v", c.status, c.experience, c.want, lines)
		}
	}
	if len(statusVoice[WantStatusReaching]) != 0 {
		t.Error("reaching is left to experience")
	}
}

// Without a script every want says something, the same thing until it changes,
// and seasoned wants do not all say the same line.
func TestWantVoiceDefault(t *testing.T) {
	base := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	if got := WantVoice(context.Background(), voiceWant(t, "new", 0, base), "").Say; got == "" {
		t.Error("a want that has done nothing yet still says something")
	}

	w := voiceWant(t, "steady", 20, base)
	w.SetStatus(WantStatusReaching)
	first := WantVoice(context.Background(), w, "").Say
	if first == "" {
		t.Fatal("a want with twenty answers says something")
	}
	if again := WantVoice(context.Background(), w, "").Say; again != first {
		t.Errorf("same want, same answers: %q then %q", first, again)
	}

	said := map[string]bool{}
	for i := 0; i < 12; i++ {
		v := voiceWant(t, fmt.Sprintf("v%d", i), 20, base)
		v.SetStatus(WantStatusReaching)
		said[WantVoice(context.Background(), v, "").Say] = true
	}
	if len(said) < 2 {
		t.Errorf("twelve seasoned wants all said %v", said)
	}
}

// A script decides the line, gets the want handed to it, is run again only
// when the want changes, and a failing script is silence, not a crash.
func TestWantVoiceScript(t *testing.T) {
	saved := runVoiceScript
	defer func() { runVoiceScript = saved }()

	runs := 0
	var got map[string]any
	runVoiceScript = func(_ context.Context, script string, input []byte) (map[string]any, error) {
		runs++
		if err := json.Unmarshal(input, &got); err != nil {
			t.Fatalf("input is not JSON: %v", err)
		}
		if script == "broken.py" {
			return nil, errors.New("Traceback …")
		}
		state := got["state"].(map[string]any)
		if script == "moody.py" {
			return map[string]any{"say": "あと少し", "mood": "eager"}, nil
		}
		if script == "odd.py" {
			return map[string]any{"say": "うまくいかなかった", "mood": "furious"}, nil
		}
		return map[string]any{"say": fmt.Sprintf("あと%v個", state["left"])}, nil
	}

	w := voiceWant(t, "script", 2, time.Now())
	w.StoreState("left", 3)
	if say := WantVoice(context.Background(), w, "voice.py").Say; say != "あと3個" {
		t.Errorf("say = %q", say)
	}
	if got["name"] != "w-script" || got["type"] != "voice_test" || got["experience"] != float64(2) || got["last_answer"] != "answer-1" {
		t.Errorf("input = %v", got)
	}
	WantVoice(context.Background(), w, "voice.py")
	if runs != 1 {
		t.Errorf("unchanged want ran the script %d times", runs)
	}
	w.StoreState("left", 1)
	if say := WantVoice(context.Background(), w, "voice.py").Say; say != "あと1個" || runs != 2 {
		t.Errorf("after a change: %q (runs %d)", say, runs)
	}
	if say := WantVoice(context.Background(), voiceWant(t, "broken", 20, time.Now()), "broken.py").Say; say != "" {
		t.Errorf("a failing script says nothing, got %q", say)
	}
	if line := WantVoice(context.Background(), w, "voice.py"); line.Mood != MoodCalm {
		t.Errorf("a script that gives no mood is read by its words: %+v", line)
	}
	if line := WantVoice(context.Background(), voiceWant(t, "moody", 0, time.Now()), "moody.py"); line.Mood != MoodEager {
		t.Errorf("a script's own mood wins: %+v", line)
	}
	if line := WantVoice(context.Background(), voiceWant(t, "odd", 0, time.Now()), "odd.py"); line.Mood != MoodSad {
		t.Errorf("a mood the board cannot play is read from the words instead: %+v", line)
	}
}

// Every default line has the mood its words say, and the stronger feeling is
// heard first.
func TestMoodOf(t *testing.T) {
	cases := map[string]string{
		"できたよ":          MoodJoy,
		"いつもありがとう":      MoodJoy,
		"12回こなしたよ":      MoodJoy,
		"はじめての仕事だ":      MoodEager,
		"今日も任せて":        MoodEager,
		"まだ3回だけ、覚えてる途中": MoodEager,
		"準備してるよ":        MoodCalm,
		"ひと休み中":         MoodCalm,
		"しまわれてます":       MoodCalm,
		"あなたの返事を待ってるよ":  MoodWaiting,
		"急がなくていいよ":      MoodWaiting,
		"うまくいかなかった…":    MoodSad,
		"おしまい":          MoodSad,
		"設定を見てもらえる？":    MoodWorried,
		"調子が悪いみたい":      MoodWorried,
		warningVoice:    MoodWorried,
		"雪だ！足元に気をつけて":   MoodWorried,
		"全部終わった！":       MoodJoy,
		"今日は空が見えなかった":   MoodSad,
		"霧で前が見えないよ":     MoodWorried,
	}
	for line, want := range cases {
		if got := moodOf(line); got != want {
			t.Errorf("%q: %s, want %s", line, got, want)
		}
	}
}

func TestVoiceLines(t *testing.T) {
	if l := voiceLines(nil); l != nil {
		t.Errorf("null: %v", l)
	}
	if l := voiceLines("  "); l != nil {
		t.Errorf("blank: %v", l)
	}
	if l := voiceLines([]any{"a", "", 3, "b"}); len(l) != 2 || l[0] != "a" || l[1] != "b" {
		t.Errorf("list: %v", l)
	}
}

func TestVoiceScriptDiscovery(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("print('{}')"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	yaml := write("transit_search.yaml")
	if got := voiceScriptOnDisk(yaml, true); got != "" {
		t.Errorf("no script yet, got %q", got)
	}
	plain := write("voice.py")
	if got := voiceScriptOnDisk(yaml, true); got != plain {
		t.Errorf("a custom package's voice.py: got %q", got)
	}
	if got := voiceScriptOnDisk(yaml, false); got != "" {
		t.Errorf("a bundled directory's voice.py belongs to no one type, got %q", got)
	}
	named := write("transit_search.voice.py")
	if got := voiceScriptOnDisk(yaml, true); got != named {
		t.Errorf("<type>.voice.py comes first, got %q", got)
	}

	savedDir := voiceScriptDir
	defer func() { voiceScriptDir = savedDir }()
	voiceScriptDir = func() string { return filepath.Join(dir, "cache") }
	fsys := fstest.MapFS{
		"want_types/system/checklist.yaml":     {Data: []byte("wantType: {}")},
		"want_types/system/checklist.voice.py": {Data: []byte("print('{\"say\": \"hi\"}')")},
		"want_types/system/weather.yaml":       {Data: []byte("wantType: {}")},
	}
	out := voiceScriptEmbedded(fsys, "want_types/system/checklist.yaml")
	if out == "" {
		t.Fatal("bundled script not found")
	}
	if filepath.Base(out) != "checklist.voice.py" || !strings.HasPrefix(out, filepath.Join(dir, "cache")+string(filepath.Separator)) {
		t.Errorf("written to %s", out)
	}
	if data, err := os.ReadFile(out); err != nil || string(data) != "print('{\"say\": \"hi\"}')" {
		t.Errorf("written out as %q (%v)", data, err)
	}
	if got := voiceScriptEmbedded(fsys, "want_types/system/weather.yaml"); got != "" {
		t.Errorf("weather has no script, got %q", got)
	}
}

// The checklist's own script, run by python3 the way the engine runs it.
func TestChecklistVoiceScript(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	script, err := filepath.Abs("../bundled/want_types/system/checklist.voice.py")
	if err != nil {
		t.Fatal(err)
	}
	run := func(items any) any {
		in, _ := json.Marshal(map[string]any{"state": map[string]any{"items": items}})
		out, err := exec.Command("python3", script, string(in)).Output()
		if err != nil {
			t.Fatalf("script failed: %v", err)
		}
		var res map[string]any
		if err := json.Unmarshal(out, &res); err != nil {
			t.Fatalf("output %q: %v", out, err)
		}
		return res["say"]
	}
	items := []map[string]any{{"text": "a", "done": true}, {"text": "b", "done": false}, {"text": "c", "done": false}}
	if got := run(items); got != "あと2個残ってるよ" {
		t.Errorf("two left: %v", got)
	}
	if got := run([]map[string]any{{"text": "a", "done": true}}); got != "全部終わった！" {
		t.Errorf("none left: %v", got)
	}
	if got := run(`[{"text": "a", "done": false}]`); got != "あと1個残ってるよ" {
		t.Errorf("items as a JSON string: %v", got)
	}
	if got := run([]any{}); got != nil {
		t.Errorf("an empty list says nothing: %v", got)
	}
}

// The weather's own script: how today feels, not the numbers again.
func TestWeatherVoiceScript(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	script, err := filepath.Abs("../bundled/want_types/system/weather.voice.py")
	if err != nil {
		t.Fatal(err)
	}
	run := func(state map[string]any) any {
		in, _ := json.Marshal(map[string]any{"state": state})
		out, err := exec.Command("python3", script, string(in)).Output()
		if err != nil {
			t.Fatalf("script failed: %v", err)
		}
		var res map[string]any
		if err := json.Unmarshal(out, &res); err != nil {
			t.Fatalf("output %q: %v", out, err)
		}
		return res["say"]
	}
	got := fmt.Sprint(run(map[string]any{
		"weather_text":      "快晴 21°C (湿度 73%) (最高 23° / 最低 12°)",
		"weather_condition": "sunny",
	}))
	if got != "[いい天気！21°でぽかぽかあったかいよ 21°でぽかぽかあったかいよ、朝晩は冷えるよ（最低12°）]" {
		t.Errorf("a warm clear day: %v", got)
	}
	if got := fmt.Sprint(run(map[string]any{"weather_text": "雨 3°C", "weather_condition": "rain"})); got != "[傘を忘れないでね3°でさむい…ぶるぶる]" {
		t.Errorf("a cold rainy day: %v", got)
	}
	if got := run(map[string]any{"weather_text": "", "error": "timeout"}); fmt.Sprint(got) != "[今日は空が見えなかった]" {
		t.Errorf("a failed fetch: %v", got)
	}
	if got := run(map[string]any{}); got != nil {
		t.Errorf("not fetched yet says nothing: %v", got)
	}
}
