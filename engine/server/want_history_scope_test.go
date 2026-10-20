package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// A list carries its wants' answers only; a single want carries every history
// unless it asks for answers only — and either can ask for the other.
func TestWantHistoryScope(t *testing.T) {
	t.Setenv("MYWANT_MRS_SHARED", "0")
	t.Setenv("MYWANT_RUNNER", "off")
	s := patchTestServer(t)

	w := patchTestDo(t, s, "POST", "/api/v1/wants", []map[string]any{{
		"metadata": map[string]any{"name": "history-scope", "type": "checklist"},
		"spec":     map[string]any{},
	}})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var id string
	for i := 0; i < 150 && id == ""; i++ {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			if want.Metadata.Name == "history-scope" {
				id = want.Metadata.ID
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	want, _, found := s.globalBuilder.FindWantByID(id)
	if !found {
		t.Fatal("want never appeared")
	}
	want.StoreState("items", []any{map[string]any{"id": "a", "text": "牛乳", "done": false}})

	get := func(path string) map[string]any {
		r := patchTestDo(t, s, "GET", path, nil)
		if r.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, r.Code, r.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	inList := func(path string) map[string]any {
		for _, w := range get(path)["wants"].([]any) {
			m := w.(map[string]any)
			if m["metadata"].(map[string]any)["id"] == id {
				return m
			}
		}
		t.Fatalf("%s: want not listed", path)
		return nil
	}
	states := func(w map[string]any) int {
		h, _ := w["history"].(map[string]any)
		l, _ := h["stateHistory"].([]any)
		return len(l)
	}

	if w := inList("/api/v1/wants"); states(w) != 0 || w["history_scope"] != "results" {
		t.Errorf("list: %d state entries, scope %v", states(w), w["history_scope"])
	}
	if w := inList("/api/v1/wants?history=all"); states(w) == 0 || w["history_scope"] != nil {
		t.Errorf("list ?history=all: %d state entries, scope %v", states(w), w["history_scope"])
	}
	if w := get("/api/v1/wants/" + id); states(w) == 0 || w["history_scope"] != nil {
		t.Errorf("want: %d state entries, scope %v", states(w), w["history_scope"])
	}
	if w := get("/api/v1/wants/" + id + "?history=results"); states(w) != 0 || w["history_scope"] != "results" {
		t.Errorf("want ?history=results: %d state entries, scope %v", states(w), w["history_scope"])
	}
}
