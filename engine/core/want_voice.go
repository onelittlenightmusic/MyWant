package mywant

// want_voice.go — what a want says when someone stands on its tile.
//
// A want that has answered many times has a history behind it, and the board
// showed it only to someone who opened the card. A word over the tile says it
// without being asked — and only to the one already standing there, so it is
// not a call for attention.
//
// What it says is the want TYPE's to decide, never a single want's: a type
// that wants a voice of its own keeps a Python script beside its definition,
//
//	~/.mywant/custom-types/<plugin>/voice.py      (a custom package)
//	want_types/<dir>/<type>.voice.py              (a bundled type; its
//	                                               directory is shared)
//
// and the engine runs it the way it runs any MRS script, with one JSON
// argument (voiceInput) and one JSON object back:
//
//	{"say": "あと2個残ってるよ"}        one line
//	{"say": ["a", "b"]}                 lines to take turns with
//	{"say": null}                       nothing to say right now
//	{"say": "…", "mood": "sad"}         and how it feels (else read from
//	                                    the words — see moodOf)
//
// A type with no script speaks from defaultVoiceFor: where it stands, then
// how much it has done.
//
// The same script says the want's news — a new answer, or getting there — in
// the notice and the push it raises (WantNewsVoice): its input then carries
// "event", {"kind": "output", "result": <the answer>} or {"kind": "achieved"},
// where standing on the tile carries none. A script that says nothing to it
// leaves the notice as it was, the answer itself.
// And it says the way somewhere, when someone is shown it (WantGuideVoice):
// its input then carries "event", {"kind": "guide", "guide": {...}} — the
// guide's facts and the moments it needs a line for — and it answers with one
// line per moment, {"say": {"<moment>": "…"}}. The words are the script's; the
// guide (a booking card's "take me to 18:00 in Room02") only says what is
// going on. They are said like any other line, so they are translated too.
// Either way the line is worked out here, on the server, so every surface that
// draws the want (the board, the extension, the phone) says the same thing.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

// voiceScriptFile is the script a custom package keeps at the top of its
// directory, beside its want type's YAML.
const voiceScriptFile = "voice.py"

// voiceScriptSuffix names a bundled type's script: <type>.voice.py beside
// <type>.yaml, because bundled types share a directory.
const voiceScriptSuffix = ".voice.py"

// voiceTimeout bounds one run: a voice is a nicety, and a script that hangs
// must not hold up the request that asked for it.
const voiceTimeout = 3 * time.Second

// voiceScriptOnDisk finds the voice script for a type loaded from yamlPath:
// <type>.voice.py beside it, or — for a custom package — voice.py.
func voiceScriptOnDisk(yamlPath string, allowPlain bool) string {
	named := strings.TrimSuffix(yamlPath, filepath.Ext(yamlPath)) + voiceScriptSuffix
	if fileExists(named) {
		return realScriptPath(named)
	}
	if allowPlain {
		plain := filepath.Join(filepath.Dir(yamlPath), voiceScriptFile)
		if fileExists(plain) {
			return realScriptPath(plain)
		}
	}
	return ""
}

// realScriptPath is where a script really is: a custom package is a link
// (custom-types/<plugin> → customs/<plugin>), and a voice script's sandbox sees
// only its own directory — bound as named, a link points at nothing inside it.
func realScriptPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// voiceScriptEmbedded finds a bundled type's voice script in the binary and
// writes it out where python3 can read it — named by its content, so a new
// build writes a new file and an unchanged one reuses the old. Under ~/.mywant,
// not the temp directory, because the runner sees ~/.mywant and not our /tmp;
// each script in a directory of its own, because a voice call sees only its
// script's directory.
var voiceScriptDir = func() string { return filepath.Join(MyWantHomeDir(), "cache", "voice") }

func voiceScriptEmbedded(fsys fs.FS, yamlPath string) string {
	named := strings.TrimSuffix(yamlPath, filepath.Ext(yamlPath)) + voiceScriptSuffix
	data, err := fs.ReadFile(fsys, named)
	if err != nil {
		return ""
	}
	return cacheVoiceScript(filepath.Base(named), data)
}

// voiceScriptBundled is a bundled type's script read from the repo (a server
// run there loads engine/bundled from disk), written out as an embedded one
// is: the runner does not see the repo.
func voiceScriptBundled(yamlPath string) string {
	named := voiceScriptOnDisk(yamlPath, false)
	if named == "" {
		return ""
	}
	data, err := os.ReadFile(named)
	if err != nil {
		return ""
	}
	return cacheVoiceScript(filepath.Base(named), data)
}

func cacheVoiceScript(name string, data []byte) string {
	sum := sha256.Sum256(data)
	dir := filepath.Join(voiceScriptDir(), hex.EncodeToString(sum[:])[:12])
	out := filepath.Join(dir, name)
	if !fileExists(out) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			ErrorLog("[VOICE] cannot make %s: %v", dir, err)
			return ""
		}
		if err := os.WriteFile(out, data, 0o644); err != nil {
			ErrorLog("[VOICE] cannot write %s: %v", out, err)
			return ""
		}
	}
	return out
}

// VoiceEvent is a want's news, handed to its voice script.
type VoiceEvent struct {
	Kind   string
	Result any
	// Guide: what a "guide" event is showing the way to (WantGuideVoice).
	Guide any
	// Heard: the murmur a "heard" event is about (want_hear.go) — who said
	// it, at which timing, its words and its answer.
	Heard *HeardMurmur
	// Vars: the names an event is about ("from", "to"), for its words —
	// the default lines' {from} (want_voice_defaults.go), and a script's.
	Vars map[string]string
}

// WantHomeOf, when set (by the server), is the home of the character that
// asked for w at this time of day, in w's type's words — handed to its voice
// script as "home":
//
//	{"part": "夜", "place": "中野坂上", "param": "to", "value": "中野坂上",
//	 "through": "", "asks": 3, "of": 4, "here": true}
//
// here: w's own parameter already names it. nil when the character has no
// home now, or the type no word for it. The words are the script's.
var WantHomeOf func(w *Want) map[string]any

func wantHome(w *Want) map[string]any {
	if WantHomeOf == nil {
		return nil
	}
	return WantHomeOf(w)
}

// voiceInput is what a script is handed; ev is nil standing on the tile.
func voiceInput(w *Want, ev *VoiceEvent) map[string]any {
	entries := w.ResultHistoryEntries()
	var last any
	if len(entries) > 0 {
		last = entries[len(entries)-1].Result
	}
	meta := w.GetMetadata()
	return map[string]any{
		"name":        meta.Name,
		"type":        meta.Type,
		"experience":  len(entries),
		"last_answer": last,
		"state":       w.GetAllState(),
		"params":      w.GetSpec().Params,
		"event":       voiceEventInput(ev),
		"home":        wantHome(w),
	}
}

func voiceEventInput(ev *VoiceEvent) any {
	if ev == nil {
		return nil
	}
	out := map[string]any{"kind": ev.Kind}
	if len(ev.Vars) > 0 {
		out["vars"] = ev.Vars
	}
	if ev.Result != nil {
		out["result"] = ev.Result
	}
	if ev.Guide != nil {
		out["guide"] = ev.Guide
	}
	if m := ev.Heard; m != nil {
		out["heard"] = map[string]any{
			"murmur_id": m.MurmurID, "from_want_id": m.FromID, "from_want_name": m.FromName,
			"from_want_type": m.FromType, "timings": m.Timings, "text": m.Text,
			"result": m.Result, "fields": m.Fields, "labels": m.Labels,
		}
	}
	return out
}

// defaultVoiceFor is what w says with no script: its status first, then how
// much it has done.
func defaultVoiceFor(status WantStatus, experience int) []string {
	warned := false
	switch status {
	case WantStatusReachingWithWarning:
		status, warned = WantStatusReaching, true
	case WantStatusAchievedWithWarning:
		status, warned = WantStatusAchieved, true
	}
	var lines []string
	switch {
	case status == WantStatusDeleting:
		return nil
	case defaultBook().status[status] != nil:
		lines = defaultBook().status[status]
	case status == WantStatusAchieved && experience < 5:
		lines = defaultBook().achieved
	default:
		lines = defaultVoiceLines(experience)
	}
	if warned {
		lines = append(append([]string{}, lines...), defaultBook().warning)
	}
	return lines
}

func defaultVoiceLines(experience int) []string {
	for _, step := range defaultBook().experience {
		if experience >= step.at {
			out := make([]string, len(step.lines))
			for i, l := range step.lines {
				if strings.Contains(l, "%d") {
					l = fmt.Sprintf(l, experience)
				}
				out[i] = l
			}
			return out
		}
	}
	return nil
}

// voiceSeed picks among lines: the want and when its latest answer arrived.
// Never chance — the same want says the same thing until it answers again —
// and never the experience count, which stops at twenty and would have every
// seasoned want say the same line forever.
func voiceSeed(w *Want) uint32 {
	h := fnv.New32a()
	h.Write([]byte(w.GetMetadata().ID))
	if entries := w.ResultHistoryEntries(); len(entries) > 0 {
		h.Write([]byte(entries[len(entries)-1].Timestamp.UTC().Format(time.RFC3339Nano)))
	}
	return h.Sum32()
}

// voiceLines reads a script's "say": a string, a list of strings, or nothing.
func voiceLines(v any) []string {
	switch t := v.(type) {
	case string:
		if s := strings.TrimSpace(t); s != "" {
			return []string{s}
		}
	case []any:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}
	return nil
}

// runVoiceScript is how a script is run — a variable so tests need no python3.
var runVoiceScript = func(ctx context.Context, script string, input []byte) (map[string]any, error) {
	return RunMRSScript(ctx, script, MRSRunOptions{Args: []string{string(input)}, Profile: MRSProfileVoice})
}

// A mood is the kind of feeling a line has — what the board plays as the
// line comes up, so the same word sounds the same wherever it is said.
const (
	MoodJoy     = "joy"     // できた、ありがとう、いい天気！
	MoodEager   = "eager"   // はじめて、やってみる、任せて
	MoodCalm    = "calm"    // 準備してる、ひと休み — anything not below
	MoodWaiting = "waiting" // 待ってるよ、急がなくていい
	MoodSad     = "sad"     // うまくいかなかった、おしまい
	MoodWorried = "worried" // 気になる、調子が悪い、気をつけて
)

// moodWords sorts a line by its words: the first mood with a word in the line
// is its mood, so the stronger feelings are looked for first — "雪だ！足元に
// 気をつけて" is a worry before it is a cheer. A line with none is calm.
var moodWords = []struct {
	mood  string
	words []string
}{
	{MoodWorried, []string{"気になる", "調子が悪い", "見てもらえる", "こわい", "気をつけ", "忘れないで", "見えない", "溶け"}},
	{MoodSad, []string{"うまくいかなかった", "おしまい", "替わった", "見えなかった", "寒い", "さむい"}},
	{MoodWaiting, []string{"待って", "急がなく", "呼んで"}},
	{MoodJoy, []string{"できた", "終わった", "おわった", "ありがとう", "元気", "ベテラン", "調子いい", "いい感じ", "いい天気", "ぽかぽか", "目をつぶって", "こなした", "！"}},
	{MoodEager, []string{"はじめ", "やってみる", "やらせて", "任せて", "聞いて", "これから", "覚えて", "慣れて", "分かってきた"}},
}

// MoodOf is the mood of a line said elsewhere — a notice — by its words.
func MoodOf(line string) string { return moodOf(line) }

// moodOf is the mood of a line, by its words.
func moodOf(line string) string {
	for _, m := range moodWords {
		for _, w := range m.words {
			if strings.Contains(line, w) {
				return m.mood
			}
		}
	}
	return MoodCalm
}

// knownMood is whether m is one of the moods the board can play.
func knownMood(m string) bool {
	switch m {
	case MoodJoy, MoodEager, MoodCalm, MoodWaiting, MoodSad, MoodWorried:
		return true
	}
	return false
}

// VoiceLine is what a want says and how it feels about it.
type VoiceLine struct {
	Say  string
	Mood string
	// Labels: what the line is about in set words — "time" (RFC3339) and
	// "place" — for the wants that hear it (MurmurLabelTime / MurmurLabelPlace),
	// given by a voice script's "labels".
	Labels map[string]string
}

type voiceCacheEntry struct {
	hash string
	line VoiceLine
}

// voiceCache remembers each want's line until the want changes: standing on a
// tile again, or another page asking, does not run the script again.
var voiceCache sync.Map // want ID → voiceCacheEntry

// WantVoice is what w says now (Say "" for nothing). script is its type's
// voice script ("" when the type has none — see WantTypeLoader.GetVoiceScript).
func WantVoice(ctx context.Context, w *Want, script string) VoiceLine {
	id := w.GetMetadata().ID
	hash := CalculateWantHash(w) + "|" + script
	if script != "" {
		// Its home changes with the time of day and the asks, not the want.
		if h, err := json.Marshal(wantHome(w)); err == nil {
			hash += "|" + string(h)
		}
	}
	if e, ok := voiceCache.Load(id); ok && e.(voiceCacheEntry).hash == hash {
		return e.(voiceCacheEntry).line
	}
	line := computeVoice(ctx, w, script)
	voiceCache.Store(id, voiceCacheEntry{hash: hash, line: line})
	return line
}

// computeVoice picks the line, and its mood: the script's "mood" when it gave
// one the board knows, or else the line's own words (moodOf).
func computeVoice(ctx context.Context, w *Want, script string) VoiceLine {
	var lines []string
	mood := ""
	if script == "" {
		lines = defaultVoiceFor(w.GetStatus(), len(w.ResultHistoryEntries()))
	} else {
		var said bool
		lines, mood, _, said = scriptVoice(ctx, w, script, nil)
		if !said {
			// A script that gives no "say" at all (one there only for its
			// labels) leaves the want its default lines; "say": null is silence.
			lines = defaultVoiceFor(w.GetStatus(), len(w.ResultHistoryEntries()))
		}
	}
	// Every line it could say, not only the one picked: the next answer picks
	// another, and that one is ready too.
	voiceLinesSaid(lines)
	return pickVoice(w, lines, mood)
}

// WantNewsVoice is w's news said in its own voice, for the notice and the push
// it raises — Say "" when its type has no script, or the script has nothing to
// say to it, and the notice says the news plainly. Never cached: news happens
// once.
// WantGuideVoice is what the type's voice says along a way it shows someone
// (a "guide" event): one line per moment, keyed as the script keyed them. Said
// like any line, so each is put to translation (voiceLinesSaid). Nothing
// without a script, or for a script that has nothing to say about it.
func WantGuideVoice(ctx context.Context, w *Want, script string, guide any) map[string]string {
	if script == "" {
		return nil
	}
	input, err := json.Marshal(voiceInput(w, &VoiceEvent{Kind: "guide", Guide: guide}))
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, voiceTimeout)
	defer cancel()
	out, err := runVoiceScript(ctx, script, input)
	if err != nil {
		ErrorLog("[VOICE] %s guide (%s): %v", w.GetMetadata().Name, script, err)
		return nil
	}
	said, _ := out["say"].(map[string]any)
	lines := map[string]string{}
	var all []string
	for k, v := range said {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			lines[k] = SpaceSentences(strings.TrimSpace(s))
			all = append(all, lines[k])
		}
	}
	voiceLinesSaid(all)
	return lines
}

func WantNewsVoice(ctx context.Context, w *Want, script string, ev VoiceEvent) VoiceLine {
	var lines []string
	var mood string
	var labels map[string]string
	if script != "" {
		lines, mood, labels, _ = scriptVoice(ctx, w, script, &ev)
	}
	// Nothing of its own to say about it: what any want says
	// (want_voice_defaults.go), if anything.
	if len(lines) == 0 {
		lines = EventVoiceLines(VoiceLangDefault, ev.Kind, ev.Vars)
	}
	voiceLinesSaid(lines)
	line := pickVoice(w, lines, mood)
	line.Labels = labels
	return line
}

// scriptVoice runs a type's voice script: its lines, its mood when it gave one
// the board knows, and the labels it gave its line ("labels": {"time", "place"}).
//
// said: whether it gave "say" at all (null included).
func scriptVoice(ctx context.Context, w *Want, script string, ev *VoiceEvent) ([]string, string, map[string]string, bool) {
	input, err := json.Marshal(voiceInput(w, ev))
	if err != nil {
		return nil, "", nil, true
	}
	ctx, cancel := context.WithTimeout(ctx, voiceTimeout)
	defer cancel()
	out, err := runVoiceScript(ctx, script, input)
	if err != nil {
		ErrorLog("[VOICE] %s (%s): %v", w.GetMetadata().Name, script, err)
		return nil, "", nil, true
	}
	mood := ""
	if m, ok := out["mood"].(string); ok && knownMood(m) {
		mood = m
	}
	var labels map[string]string
	if raw, ok := out["labels"].(map[string]any); ok {
		for k, v := range raw {
			if s, ok := v.(string); ok && s != "" {
				if labels == nil {
					labels = map[string]string{}
				}
				labels[k] = s
			}
		}
	}
	_, said := out["say"]
	return voiceLines(out["say"]), mood, labels, said
}

// pickVoice picks one of lines (voiceSeed), with its mood — given, or read
// from its words (moodOf).
func pickVoice(w *Want, lines []string, mood string) VoiceLine {
	if len(lines) == 0 {
		return VoiceLine{}
	}
	say := SpaceSentences(lines[voiceSeed(w)%uint32(len(lines))])
	if mood == "" {
		mood = moodOf(say)
	}
	return VoiceLine{Say: say, Mood: mood}
}

// sentenceEnd is a mark a sentence ends on; sentenceClose, what may follow one
// before the next sentence starts — more marks, a closing bracket, a space.
const (
	sentenceEnd   = "。！？!?…"
	sentenceClose = "。！？!?…、，,」』）)】]\"'〜ー"
)

// SpaceSentences puts a space between sentences run together — after 。！？
// or …, before the next one starts — so a bubble breaks its line there:
// 「雪だ！足元に気をつけて」 is said 「雪だ！ 足元に気をつけて」. Japanese has no
// spaces, and the bubble, narrow, broke wherever the width ran out, the last
// word of one sentence on the next line with the start of the other. Every
// line a want says goes through it (pickVoice), whoever wrote it — the
// engine's own, or a type's voice script.
func SpaceSentences(line string) string {
	rs := []rune(line)
	var b strings.Builder
	// A mark before anything is said (「…ちょっと気になる」) ends no sentence.
	said := false
	for i, r := range rs {
		b.WriteRune(r)
		if !strings.ContainsRune(sentenceEnd, r) {
			said = said || !unicode.IsSpace(r)
			continue
		}
		if !said || i+1 >= len(rs) {
			continue
		}
		next := rs[i+1]
		if unicode.IsSpace(next) || strings.ContainsRune(sentenceClose, next) {
			continue
		}
		b.WriteRune(' ')
	}
	return b.String()
}
