package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTranslator is a translator whose model is a function the test watches.
func fakeTranslator(t *testing.T, window time.Duration, max int, model func(lang string, texts []string) []string) (*translator, *[][]string, *sync.Mutex) {
	t.Helper()
	tr := newTranslator(loadTranslationStore(filepath.Join(t.TempDir(), "translations.json")))
	tr.window, tr.max = window, max
	tr.languages = func() []string { return []string{"en"} }
	var mu sync.Mutex
	var calls [][]string
	tr.translate = func(_ context.Context, lang string, texts []string) ([]string, error) {
		mu.Lock()
		calls = append(calls, append([]string(nil), texts...))
		mu.Unlock()
		return model(lang, texts), nil
	}
	return tr, &calls, &mu
}

func echoEnglish(_ string, texts []string) []string {
	out := make([]string, len(texts))
	for i, s := range texts {
		out[i] = "EN:" + s
	}
	return out
}

func TestTranslatorBatchesWithinTheWindow(t *testing.T) {
	tr, calls, mu := fakeTranslator(t, 50*time.Millisecond, 20, echoEnglish)
	tr.hear([]string{"できたよ", "準備してるよ"})
	tr.hear([]string{"できたよ", "雪だ！ 足元に気をつけて", "42", ""})
	time.Sleep(150 * time.Millisecond)
	tr.inFlight.Wait()

	mu.Lock()
	defer mu.Unlock()
	// One job for the lot: the second できたよ heard once, the number (no
	// Japanese in it) and the empty line not at all.
	if len(*calls) != 1 || len((*calls)[0]) != 3 {
		t.Fatalf("calls = %v, want one batch of 3", *calls)
	}
	if got, ok := tr.store.lookup("雪だ！ 足元に気をつけて", "en"); !ok || got != "EN:雪だ！ 足元に気をつけて" {
		t.Errorf("lookup = %q, %v", got, ok)
	}
	// Heard again once kept: not sent again.
	tr.hear([]string{"できたよ"})
	time.Sleep(100 * time.Millisecond)
	tr.inFlight.Wait()
	if len(*calls) != 1 {
		t.Errorf("a kept line was translated again: %v", *calls)
	}
}

func TestTranslatorSendsAFullBatchAtOnce(t *testing.T) {
	tr, calls, mu := fakeTranslator(t, time.Hour, 3, echoEnglish)
	tr.hear([]string{"一つ", "二つ", "三つ", "四つ"})
	tr.inFlight.Wait()
	mu.Lock()
	if len(*calls) != 1 || len((*calls)[0]) != 3 {
		t.Fatalf("calls = %v, want the full batch of 3 sent before the window", *calls)
	}
	mu.Unlock()
	// The fourth waits for the window.
	tr.flush()
	tr.inFlight.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(*calls) != 2 || (*calls)[1][0] != "四つ" {
		t.Errorf("calls = %v", *calls)
	}
}

func TestTranslatorKeepsNoneOfABatchThatLostALine(t *testing.T) {
	tr, _, _ := fakeTranslator(t, time.Hour, 20, func(_ string, texts []string) []string {
		return []string{"only one"}
	})
	tr.hear([]string{"一つ", "二つ"})
	tr.flush()
	tr.inFlight.Wait()
	if _, ok := tr.store.lookup("一つ", "en"); ok {
		t.Errorf("a batch that came back short was kept")
	}
	// And let go, to be tried again.
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.queued["en"]) != 0 {
		t.Errorf("queued = %v, want the lines let go", tr.queued["en"])
	}
}

func TestTranslatorWithNoReadersDoesNothing(t *testing.T) {
	tr, calls, _ := fakeTranslator(t, time.Millisecond, 20, echoEnglish)
	tr.languages = func() []string { return nil }
	tr.hear([]string{"できたよ"})
	time.Sleep(20 * time.Millisecond)
	if len(*calls) != 0 {
		t.Errorf("translated with nobody to read it: %v", *calls)
	}
}

func TestTranslationStoreOutlivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "translations.json")
	s := loadTranslationStore(path)
	if err := s.put("en", []string{" できたよ "}, []string{"Done!"}); err != nil {
		t.Fatal(err)
	}
	again := loadTranslationStore(path)
	if got, ok := again.lookup("できたよ", "en"); !ok || got != "Done!" {
		t.Errorf("after reload: %q, %v", got, ok)
	}
	if _, ok := again.lookup("できたよ", "fr"); ok {
		t.Errorf("found a language never kept")
	}
}

func TestParseNumberedList(t *testing.T) {
	out, err := parseNumberedList("Here you go:\n1. Done!\n2) Getting ready\n3．Snow! Watch your step", 3)
	if err != nil || strings.Join(out, "|") != "Done!|Getting ready|Snow! Watch your step" {
		t.Errorf("got %q, %v", out, err)
	}
	if _, err := parseNumberedList("1. a\n3. c", 3); err == nil {
		t.Errorf("a list missing a line read as whole")
	}
	if !strings.Contains(translatePrompt("en", []string{"できたよ"}), "1. できたよ") {
		t.Errorf("prompt does not number the lines")
	}
}

func TestVoiceAndLookupServeKeptTranslations(t *testing.T) {
	s := patchTestServer(t)
	// The server's translator, with a store of the test's own.
	translations.store = loadTranslationStore(filepath.Join(t.TempDir(), "translations.json"))
	translations.languages = func() []string { return nil } // nothing sent from here
	id := patchTestWant(t, s)

	ask := func(lang string) map[string]any {
		w := patchTestDo(t, s, "GET", "/api/v1/wants/"+id+"/voice?lang="+lang, nil)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	ja := ask("ja")
	said, _ := ja["say"].(string)
	if said == "" {
		t.Skipf("the test want says nothing: %v", ja)
	}
	if err := translations.store.put("en", []string{said}, []string{"EN line"}); err != nil {
		t.Fatal(err)
	}
	if en := ask("en"); en["say"] != "EN line" || en["mood"] != ja["mood"] {
		t.Errorf("en = %v, ja = %v", en, ja)
	}
	if again := ask("ja"); again["say"] != said {
		t.Errorf("ja changed: %v", again)
	}

	w := patchTestDo(t, s, "POST", "/api/v1/translations/lookup", map[string]any{
		"lang": "en", "texts": []string{said, "まだ訳していない"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("lookup: %d %s", w.Code, w.Body.String())
	}
	var out struct{ Texts []string }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if len(out.Texts) != 2 || out.Texts[0] != "EN line" || out.Texts[1] != "まだ訳していない" {
		t.Errorf("lookup = %v", out.Texts)
	}
}
