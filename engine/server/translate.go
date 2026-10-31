package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// Translations of what wants say — made ahead, in batches, by a model.
//
// What a want says is written in Japanese. A person who reads the board in
// another language (a character's display language) is shown its translation,
// and that is never made while they wait: the engine says each line as it is
// decided (core/lines_said.go), this gathers them for a few seconds, and one
// model job per language translates the lot (model_jobs.go: a device that can,
// else this server's own model, else nobody — and the line stays Japanese
// until someone can). Each sentence is translated once and kept
// (translations.json under ~/.mywant); asking for a line in another language
// only ever reads what is kept (GET /wants/{id}/voice?lang=, POST
// /translations/lookup), so the page never waits on a model.

// sourceLanguage is the language wants speak in.
const sourceLanguage = "ja"

// How long lines are gathered before they are sent, and how many at most go
// in one job: a model asked one line at a time spends its start-up on every
// line, and a numbered list much longer than this comes back with lines lost.
const (
	translateWindow   = 3 * time.Second
	translateBatchMax = 20
	// How long a batch waits for a device before this server's own model
	// does it: nobody is waiting for it, so a phone or a Mac that checks in
	// within a couple of minutes is worth waiting for.
	translateDeviceWait = 2 * time.Minute
)

// languageNames are what a model is told it translates into.
var languageNames = map[string]string{"en": "English", "ja": "Japanese"}

// ── The store ───────────────────────────────────────────────────────────────

type translationEntry struct {
	Text        string `json:"text"`
	Lang        string `json:"lang"`
	Translation string `json:"translation"`
}

// translationStore keeps each sentence's translations, by the sentence's hash
// and the language.
type translationStore struct {
	mu      sync.RWMutex
	path    string
	entries map[string]translationEntry
}

func translationKey(text, lang string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:]) + "|" + lang
}

func loadTranslationStore(path string) *translationStore {
	s := &translationStore{path: path, entries: map[string]translationEntry{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var list []translationEntry
	if err := json.Unmarshal(data, &list); err != nil {
		log.Printf("[translate] %s does not read (%v); starting empty", path, err)
		return s
	}
	for _, e := range list {
		s.entries[translationKey(e.Text, e.Lang)] = e
	}
	return s
}

func (s *translationStore) lookup(text, lang string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[translationKey(text, lang)]
	return e.Translation, ok
}

// put keeps translations and writes the file whole (to a side file, then moved
// over it, so a crash leaves the old one rather than half of a new one).
func (s *translationStore) put(lang string, texts, translations []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range texts {
		t = strings.TrimSpace(t)
		s.entries[translationKey(t, lang)] = translationEntry{Text: t, Lang: lang, Translation: translations[i]}
	}
	if s.path == "" {
		return nil
	}
	list := make([]translationEntry, 0, len(s.entries))
	for _, e := range s.entries {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Lang != list[j].Lang {
			return list[i].Lang < list[j].Lang
		}
		return list[i].Text < list[j].Text
	})
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ── The translator ──────────────────────────────────────────────────────────

// translator gathers lines and sends them in batches.
type translator struct {
	store *translationStore
	// languages are the ones some person reads the board in, besides the
	// source: what each line is translated into.
	languages func() []string
	// translate is the model: texts in, as many translations out, in order.
	translate func(ctx context.Context, lang string, texts []string) ([]string, error)
	window    time.Duration
	max       int

	mu       sync.Mutex
	pending  map[string][]string        // lang → texts, in the order heard
	queued   map[string]map[string]bool // lang → text → pending or being translated
	timer    *time.Timer
	inFlight sync.WaitGroup
}

func newTranslator(store *translationStore) *translator {
	return &translator{
		store:     store,
		languages: translationReaders,
		translate: translateWithModel,
		window:    translateWindow,
		max:       translateBatchMax,
		pending:   map[string][]string{},
		queued:    map[string]map[string]bool{},
	}
}

// translationsPath and translationReaders are where the server's translator
// keeps what it translated and who it translates for — variables so this
// package's tests keep both apart from the person's (main_test.go): a test
// server's translator read the real characters, and with one reading in
// English, lines the tests made were sent to the real model and kept in the
// real store.
var (
	translationsPath   = func() string { return filepath.Join(mywant.MyWantHomeDir(), "translations.json") }
	translationReaders = readerLanguages
)

// readerLanguages are the languages people have chosen to read the board in,
// other than the one wants speak.
func readerLanguages() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range mywant.ListCharacters() {
		l := c.Display.Language
		if l != "" && l != sourceLanguage && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

// hasJapanese: a line with none in it (a number, an English name, an emoji)
// reads the same in any language.
func hasJapanese(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

// hear takes lines a want is about to say: each not yet translated into each
// reader language is gathered, and sent when the window closes or a batch is
// full.
func (t *translator) hear(lines []string) {
	langs := t.languages()
	if len(langs) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, lang := range langs {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || !hasJapanese(line) || t.queued[lang][line] {
				continue
			}
			if _, done := t.store.lookup(line, lang); done {
				continue
			}
			if t.queued[lang] == nil {
				t.queued[lang] = map[string]bool{}
			}
			t.queued[lang][line] = true
			t.pending[lang] = append(t.pending[lang], line)
			if len(t.pending[lang]) >= t.max {
				t.sendLocked(lang)
			}
		}
	}
	if t.timer == nil && len(t.pending) > 0 {
		t.timer = time.AfterFunc(t.window, t.flush)
	}
}

// flush sends whatever has gathered.
func (t *translator) flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timer = nil
	for lang := range t.pending {
		t.sendLocked(lang)
	}
}

// sendLocked sends lang's gathered lines, a batch at most each job.
func (t *translator) sendLocked(lang string) {
	texts := t.pending[lang]
	delete(t.pending, lang)
	for len(texts) > 0 {
		n := min(len(texts), t.max)
		batch := texts[:n]
		texts = texts[n:]
		t.inFlight.Add(1)
		go t.run(lang, batch)
	}
}

// run translates one batch and keeps it — or, if it did not come back whole,
// keeps none of it: a list that lost a line has every line after it against
// the wrong sentence. Either way the lines are let go, to be heard again.
func (t *translator) run(lang string, texts []string) {
	defer t.inFlight.Done()
	defer func() {
		t.mu.Lock()
		for _, s := range texts {
			delete(t.queued[lang], s)
		}
		t.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), translateDeviceWait+3*time.Minute)
	defer cancel()
	out, err := t.translate(ctx, lang, texts)
	if err != nil {
		log.Printf("[translate] %d lines into %s: %v", len(texts), lang, err)
		return
	}
	if len(out) != len(texts) {
		log.Printf("[translate] %d lines into %s came back as %d; none kept", len(texts), lang, len(out))
		return
	}
	if err := t.store.put(lang, texts, out); err != nil {
		log.Printf("[translate] keeping %d lines into %s: %v", len(texts), lang, err)
	}
}

// translateWithModel asks a model for a batch: one job, a numbered list in and
// a numbered list out.
func translateWithModel(ctx context.Context, lang string, texts []string) ([]string, error) {
	text, by, err := generateText(ctx, translatePrompt(lang, texts), false, translateDeviceWait)
	if err != nil {
		return nil, err
	}
	out, err := parseNumberedList(text, len(texts))
	if err != nil {
		return nil, err
	}
	log.Printf("[translate] %d lines into %s in one job, by %s", len(texts), lang, by)
	return out, nil
}

func translatePrompt(lang string, texts []string) string {
	name := languageNames[lang]
	if name == "" {
		name = "the language " + lang
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Translate each numbered line from Japanese into %s. ", name)
	b.WriteString("These are short lines a small character on a game board says aloud: keep the casual, friendly tone; ")
	b.WriteString("keep numbers, units and names as they are, and add nothing a line does not have — no emoji, no exclamation. ")
	b.WriteString("Answer with one line for each line given, numbered the same way, and nothing else.\n\n")
	for i, s := range texts {
		fmt.Fprintf(&b, "%d. %s\n", i+1, strings.ReplaceAll(s, "\n", " "))
	}
	return b.String()
}

var numberedLine = regexp.MustCompile(`^\s*(\d+)\s*[.)．、:：]\s*(.*?)\s*$`)

// parseNumberedList reads "1. …" lines back: exactly 1…n, none empty.
func parseNumberedList(text string, n int) ([]string, error) {
	got := map[int]string{}
	for _, line := range strings.Split(text, "\n") {
		m := numberedLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		i, _ := strconv.Atoi(m[1])
		if i >= 1 && i <= n && m[2] != "" {
			if _, dup := got[i]; !dup {
				got[i] = m[2]
			}
		}
	}
	if len(got) != n {
		return nil, fmt.Errorf("asked for %d lines, read %d back", n, len(got))
	}
	out := make([]string, n)
	for i := range out {
		out[i] = got[i+1]
	}
	return out, nil
}

// ── Wiring and the endpoints ────────────────────────────────────────────────

var (
	translationsOnce sync.Once
	translations     *translator
)

// setupTranslation starts the translator — listening for the lines wants are
// about to say — and adds its endpoint.
func (s *Server) setupTranslation(api *mux.Router) {
	translationsOnce.Do(func() {
		translations = newTranslator(loadTranslationStore(translationsPath()))
		mywant.OnLinesSaid = func(lines []string) { translations.hear(lines) }
	})
	api.HandleFunc("/translations/lookup", s.lookupTranslations).Methods("POST", "OPTIONS")
}

// translated is text in lang, if it has been translated; else text.
func translated(text, lang string) string {
	if translations == nil || lang == "" || lang == sourceLanguage {
		return text
	}
	if t, ok := translations.store.lookup(text, lang); ok {
		return t
	}
	return text
}

// translatedOrHeard is translated, and a line not translated yet is heard, to
// be: news a want raised from its answer (not from its voice) was never said
// through the engine's lines, and a line decided before anyone read the board
// in this language (a voice kept since, unchanged) was heard when there was
// nobody to translate it for — so the first one to read it in that language
// is the one who asks for it. Still never waited for.
func translatedOrHeard(text, lang string) string {
	out := translated(text, lang)
	if out == text && translations != nil && lang != "" && lang != sourceLanguage {
		go translations.hear([]string{text})
	}
	return out
}

// lookupTranslations is POST /api/v1/translations/lookup {lang, texts} →
// {texts}: each one's translation where there is one, else itself. Only what
// is kept — never a model — so the page can ask as it draws.
func (s *Server) lookupTranslations(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lang  string   `json:"lang"`
		Texts []string `json:"texts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}
	out := make([]string, len(req.Texts))
	for i, t := range req.Texts {
		out[i] = translated(t, req.Lang)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"texts": out})
}
