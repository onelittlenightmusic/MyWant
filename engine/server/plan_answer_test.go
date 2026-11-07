package server

import (
	"path/filepath"
	"testing"
)

func TestAnswerTemplateFilled(t *testing.T) {
	values := map[string]any{"departure": "06:44", "arrival": "08:40", "fare": float64(4268), "transfers": float64(3), "itinerary": []any{"x"}}
	get := func(k string) any { return values[k] }

	got, ok := fillAnswer("「{name}」を出しました。{departure}発、{arrival}着、{fare}円、乗り換え{transfers}回です。", "伊東→中野坂上", get)
	if !ok || got != "「伊東→中野坂上」を出しました。06:44発、08:40着、4268円、乗り換え3回です。" {
		t.Errorf("filled %q %v", got, ok)
	}
	// A value not there (not done yet), or one a sentence cannot say: no answer.
	if _, ok := fillAnswer("{duration_minutes}分です。", "r", get); ok {
		t.Error("a field with no value filled")
	}
	if _, ok := fillAnswer("{itinerary}", "r", get); ok {
		t.Error("a list said in a sentence")
	}
}

func TestAnswerTemplateUsable(t *testing.T) {
	fields := []string{"departure", "fare"}
	for tmpl, want := range map[string]bool{
		"「{name}」は{departure}発、{fare}円です。": true,
		"{departure}発、{arrival}着":          false, // a field the type does not have
		"-":                                false,
		"":                                 false,
	} {
		if got := answerTemplateUsable(tmpl, fields); got != want {
			t.Errorf("%q usable %v, want %v", tmpl, got, want)
		}
	}
	var def planTypeDef
	def.State = append(def.State,
		struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}{"fare", "Fare in yen for the first route"},
		struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}{"summary", "JSON string summarising the best route"},
		struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}{"achieved", "done"})
	if names, _ := def.answerFields(); len(names) != 1 || names[0] != "fare" {
		t.Errorf("answer fields %v: not the JSON, not the bookkeeping", names)
	}
}

// Kept by type and language, and read again from the file.
func TestAnswerTemplateKept(t *testing.T) {
	answerTemplates.mu.Lock()
	savedPath, savedLoaded, savedByKey := answerTemplates.path, answerTemplates.loaded, answerTemplates.byKey
	answerTemplates.mu.Unlock()
	t.Cleanup(func() {
		answerTemplates.mu.Lock()
		answerTemplates.path, answerTemplates.loaded, answerTemplates.byKey = savedPath, savedLoaded, savedByKey
		answerTemplates.mu.Unlock()
	})
	answerTemplates.mu.Lock()
	answerTemplates.path = filepath.Join(t.TempDir(), "answer-templates.yaml")
	answerTemplates.loaded, answerTemplates.byKey = false, nil
	answerTemplates.mu.Unlock()

	keepAnswerTemplate("transit_search", "ja_JP", "{name}: {fare}円")
	if got := answerTemplateFor("transit_search", "ja-JP"); got != "{name}: {fare}円" {
		t.Errorf("ja-JP reads ja_JP's: %q", got)
	}
	if got := answerTemplateFor("transit_search", "en_US"); got != "" {
		t.Errorf("another language: %q", got)
	}
	answerTemplates.mu.Lock()
	answerTemplates.loaded, answerTemplates.byKey = false, nil
	answerTemplates.mu.Unlock()
	if got := answerTemplateFor("transit_search", "ja"); got != "{name}: {fare}円" {
		t.Errorf("read again: %q", got)
	}
}
