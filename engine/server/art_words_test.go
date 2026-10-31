package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestArtWordsRecipe(t *testing.T) {
	r := artWordsRecipeFor(artWordsRequest{Value: "東京タワー", Subtype: "station", Context: strings.Repeat("あ", 500)})
	if !strings.Contains(r.Ask, `"東京タワー"`) || !strings.Contains(r.Ask, "this station") {
		t.Errorf("ask = %s", r.Ask)
	}
	if strings.Count(r.Ask, "あ") != artContextLimit {
		t.Errorf("context not cut to %d", artContextLimit)
	}
	if r.Warm {
		t.Error("warm without instead")
	}
	if !artWordsRecipeFor(artWordsRequest{Value: "x", Instead: "a tower"}).Warm {
		t.Error("instead should be warm")
	}
	if got := r.compose("  a red lattice tower  "); got != "a red lattice tower, simple flat icon, plain background" {
		t.Errorf("compose = %q", got)
	}
	if got := r.compose(""); got != "東京タワー, simple flat icon, plain background" {
		t.Errorf("fallback = %q", got)
	}
	if got := r.compose(strings.Repeat("a", 200)); len(got) != artWordsLimit+len(r.Suffix) {
		t.Errorf("not cut: %d", len(got))
	}
}

// With a model here the page is handed the words; without one, only the
// request — and either way how to put an answer together.
func TestDescribeArtEndpoint(t *testing.T) {
	s := patchTestServer(t)
	defer func(f func(string, bool) (string, bool)) { artAskModel = f }(artAskModel)

	for _, c := range []struct {
		model  func(string, bool) (string, bool)
		prompt string
	}{
		{func(string, bool) (string, bool) { return "", false }, ""},
		{func(string, bool) (string, bool) { return "a red tower", true }, "a red tower, simple flat icon, plain background"},
	} {
		artAskModel = c.model
		w := patchTestDo(t, s, "POST", "/api/v1/art/describe", map[string]any{"value": "東京タワー"})
		if w.Code != http.StatusOK {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var got artWordsRecipe
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if got.Prompt != c.prompt || got.Ask == "" || got.Fallback == "" {
			t.Errorf("got %+v, want prompt %q", got, c.prompt)
		}
	}
	if w := patchTestDo(t, s, "POST", "/api/v1/art/describe", map[string]any{"value": " "}); w.Code != http.StatusBadRequest {
		t.Errorf("empty value: %d", w.Code)
	}
}
