package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A way kept for a link (POST /web-wants/guides) is fetched by its id, once
// and again, and an id never kept is not found.
func TestWebGuideKeptAndFetched(t *testing.T) {
	s := patchTestServer(t)
	w := patchTestDo(t, s, "POST", "/api/v1/web-wants/guides", map[string]any{
		"url": "https://x/", "steps": []any{map[string]any{"type": "click"}},
	})
	var kept struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &kept); err != nil || kept.ID == "" {
		t.Fatalf("keep: %d %s", w.Code, w.Body.String())
	}
	w = patchTestDo(t, s, "GET", "/api/v1/web-wants/guides/"+kept.ID, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"steps"`) {
		t.Fatalf("fetch: %d %s", w.Code, w.Body.String())
	}
	w = patchTestDo(t, s, "GET", "/api/v1/web-wants/guides/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown id: %d", w.Code)
	}
}
