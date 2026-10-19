package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// A checklist says how many are left — from its type's voice script, which the
// loader found beside checklist.yaml — with the mood its words have, and a
// want with no script still says something.
func TestGetWantVoice(t *testing.T) {
	// Spawn the script per call: the shared agent service would outlive the
	// test and hold its output open.
	t.Setenv("MYWANT_MRS_SHARED", "0")
	t.Setenv("MYWANT_RUNNER", "off")
	s := patchTestServer(t)
	if s.wantTypeLoader.GetVoiceScript("checklist") == "" {
		t.Fatal("checklist.voice.py was not found beside checklist.yaml")
	}

	w := patchTestDo(t, s, "POST", "/api/v1/wants", []map[string]any{{
		"metadata": map[string]any{"name": "voice-checklist", "type": "checklist"},
		"spec":     map[string]any{},
	}})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}

	body := func(id string) map[string]any {
		r := patchTestDo(t, s, "GET", "/api/v1/wants/"+id+"/voice", nil)
		if r.Code != http.StatusOK {
			t.Fatalf("voice: %d %s", r.Code, r.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	say := func(id string) any { return body(id)["say"] }

	var id string
	for i := 0; i < 150 && id == ""; i++ {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			if want.Metadata.Name == "voice-checklist" {
				id = want.Metadata.ID
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	want, _, found := s.globalBuilder.FindWantByID(id)
	if !found {
		t.Fatal("checklist never appeared")
	}

	// The board writes the items (the type itself only declares them).
	want.StoreState("items", []any{
		map[string]any{"id": "a", "text": "牛乳", "done": true},
		map[string]any{"id": "b", "text": "卵", "done": false},
		map[string]any{"id": "c", "text": "パン", "done": false},
	})
	if got := say(id); got != "あと2個残ってるよ" {
		t.Errorf("checklist says %v", got)
	}
	want.StoreState("items", []any{map[string]any{"id": "a", "text": "牛乳", "done": true}})
	if got := body(id); got["say"] != "全部終わった！" || got["mood"] != "joy" {
		t.Errorf("after ticking the rest, checklist says %v", got)
	}

	if got := body(patchTestWant(t, s)); got["say"] == nil || got["mood"] == nil {
		t.Errorf("a new queue says something too, got %v", got)
	}
	if r := patchTestDo(t, s, "GET", "/api/v1/wants/nope/voice", nil); r.Code != http.StatusNotFound {
		t.Errorf("unknown want: %d", r.Code)
	}
}
