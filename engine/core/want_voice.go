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
//
// A type with no script speaks from defaultVoice once it has some experience.
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
		return named
	}
	if allowPlain {
		plain := filepath.Join(filepath.Dir(yamlPath), voiceScriptFile)
		if fileExists(plain) {
			return plain
		}
	}
	return ""
}

// voiceScriptEmbedded finds a bundled type's voice script in the binary and
// writes it out where python3 can read it — named by its content, so a new
// build writes a new file and an unchanged one reuses the old.
func voiceScriptEmbedded(fsys fs.FS, yamlPath string) string {
	named := strings.TrimSuffix(yamlPath, filepath.Ext(yamlPath)) + voiceScriptSuffix
	data, err := fs.ReadFile(fsys, named)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	out := filepath.Join(os.TempDir(), "mywant-voice-"+hex.EncodeToString(sum[:])[:12]+".py")
	if !fileExists(out) {
		if err := os.WriteFile(out, data, 0o644); err != nil {
			ErrorLog("[VOICE] cannot write %s: %v", out, err)
			return ""
		}
	}
	return out
}

// voiceInput is what a script is handed.
func voiceInput(w *Want) map[string]any {
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
	}
}

// defaultVoice is what a want with no voice of its own says, by how much it
// has done: the lines of the highest threshold it has reached. It starts at
// five — a want that has done a thing twice has no feelings about it yet —
// and stops at twenty, because the engine keeps twenty answers
// (resultHistoryDepth) and so twenty is "a lot".
var defaultVoice = []struct {
	at    int
	lines []string
}{
	{20, []string{"ベテランです", "いつもありがとう", "なんでも聞いて", "今日も元気です", "ずっとここにいるよ"}},
	{15, []string{"もう目をつぶってもできる", "いい感じに回ってる"}},
	{10, []string{"%d回こなしたよ", "今日も任せて", "調子いいよ"}},
	{5, []string{"だんだん慣れてきたよ", "ちょっとずつ分かってきた"}},
}

func defaultVoiceLines(experience int) []string {
	for _, step := range defaultVoice {
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
	return RunMRSScript(ctx, script, MRSRunOptions{Args: []string{string(input)}})
}

type voiceCacheEntry struct {
	hash string
	say  string
}

// voiceCache remembers each want's line until the want changes: standing on a
// tile again, or another page asking, does not run the script again.
var voiceCache sync.Map // want ID → voiceCacheEntry

// WantVoice is what w says now, or "" for nothing. script is its type's voice
// script ("" when the type has none — see WantTypeLoader.GetVoiceScript).
func WantVoice(ctx context.Context, w *Want, script string) string {
	id := w.GetMetadata().ID
	hash := CalculateWantHash(w) + "|" + script
	if e, ok := voiceCache.Load(id); ok && e.(voiceCacheEntry).hash == hash {
		return e.(voiceCacheEntry).say
	}
	say := computeVoice(ctx, w, script)
	voiceCache.Store(id, voiceCacheEntry{hash: hash, say: say})
	return say
}

func computeVoice(ctx context.Context, w *Want, script string) string {
	var lines []string
	if script == "" {
		lines = defaultVoiceLines(len(w.ResultHistoryEntries()))
	} else {
		input, err := json.Marshal(voiceInput(w))
		if err != nil {
			return ""
		}
		ctx, cancel := context.WithTimeout(ctx, voiceTimeout)
		defer cancel()
		out, err := runVoiceScript(ctx, script, input)
		if err != nil {
			ErrorLog("[VOICE] %s (%s): %v", w.GetMetadata().Name, script, err)
			return ""
		}
		lines = voiceLines(out["say"])
	}
	if len(lines) == 0 {
		return ""
	}
	return lines[voiceSeed(w)%uint32(len(lines))]
}
