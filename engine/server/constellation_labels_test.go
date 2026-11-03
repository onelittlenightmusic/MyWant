package server

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	mwlabels "mywant/engine/labels"
)

// A constellation's own labels ride on its members: given on create, read
// back on list and get, taken by a member joining, carried through a rename,
// replaced and cleared by an update, and written out and read back in.
func TestConstellationLabels(t *testing.T) {
	dir := t.TempDir()
	s := &Server{
		thingStore:  &ThingStore{path: filepath.Join(dir, "memo.yaml")},
		thingLabels: &ThingLabelStore{mwlabels.NewFileStore(filepath.Join(dir, "memo-labels.yaml"))},
	}
	r := mux.NewRouter()
	r.HandleFunc("/constellations", s.createConstellation).Methods("POST")
	r.HandleFunc("/constellations", s.getConstellations).Methods("GET")
	r.HandleFunc("/constellations/export", s.exportConstellations).Methods("POST")
	r.HandleFunc("/constellations/import", s.importConstellations).Methods("POST")
	r.HandleFunc("/constellations/{name}", s.getConstellation).Methods("GET")
	r.HandleFunc("/constellations/{name}", s.updateConstellation).Methods("PUT")
	r.HandleFunc("/constellations/{name}", s.deleteConstellation).Methods("DELETE")
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
		}
		return rec
	}
	get := func(name string) constellationDTO {
		var c constellationDTO
		_ = json.Unmarshal(do("GET", "/constellations/"+name, "").Body.Bytes(), &c)
		return c
	}

	rec := do("POST", "/constellations", `{"name":"中央線","members":["thg-1","thg-2"],"labels":{"line":"jr","area":"tokyo"}}`)
	var made constellationDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &made)
	if made.Labels["line"] != "jr" {
		t.Errorf("create answers with its labels: %+v", made)
	}
	if c := get("中央線"); c.Labels["line"] != "jr" || c.Labels["area"] != "tokyo" {
		t.Errorf("read back: %+v", c)
	}
	if got := s.thingLabels.Get("thg-1")["constellation/中央線"]; got != "true" {
		t.Errorf("membership unchanged: %q", got)
	}

	// A member joining takes them.
	do("PUT", "/constellations/中央線", `{"members":["thg-1","thg-2","thg-3"]}`)
	if v := decodeConstellationLabels(s.thingLabels.Get("thg-3")[constellationLabelsKey("中央線")]); v["line"] != "jr" {
		t.Errorf("a joining member takes the labels: %v", v)
	}
	// A rename carries them.
	do("PUT", "/constellations/中央線", `{"name":"中央快速線"}`)
	if c := get("中央快速線"); c.Labels["area"] != "tokyo" {
		t.Errorf("carried through a rename: %+v", c)
	}
	if v := s.thingLabels.Get("thg-1")[constellationLabelsKey("中央線")]; v != "" {
		t.Errorf("the old name's labels let go: %q", v)
	}
	// An update replaces them; {} clears them.
	do("PUT", "/constellations/中央快速線", `{"labels":{"line":"jr-east"}}`)
	if c := get("中央快速線"); len(c.Labels) != 1 || c.Labels["line"] != "jr-east" {
		t.Errorf("replaced: %+v", c)
	}
	exported := do("POST", "/constellations/export", "").Body.String()
	if !strings.Contains(exported, "labels:") || !strings.Contains(exported, "jr-east") {
		t.Errorf("exported with its labels:\n%s", exported)
	}
	do("PUT", "/constellations/中央快速線", `{"labels":{}}`)
	if c := get("中央快速線"); c.Labels != nil {
		t.Errorf("cleared: %+v", c)
	}

	// Read back in.
	do("DELETE", "/constellations/中央快速線", "")
	do("POST", "/constellations/import", exported)
	if c := get("中央快速線"); c.Labels["line"] != "jr-east" || len(c.Members) != 3 {
		t.Errorf("imported with its labels: %+v", c)
	}
}
