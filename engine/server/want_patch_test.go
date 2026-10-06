package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func patchTestDo(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b []byte
	if body != nil {
		var err error
		if b, err = json.Marshal(body); err != nil {
			t.Fatal(err)
		}
	}
	req, _ := http.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

// patchTestServer is a test server with the bundled want types (which the
// robot's tools choose from) and its reconcile loop running, which is what
// turns a POSTed want into one the builder knows.
func patchTestServer(t *testing.T) *Server {
	t.Helper()
	s := New(Config{Port: 0, Host: "localhost", Debug: true, WantTypesDir: "../../engine/bundled/want_types"})
	s.setupRoutes()
	go s.globalBuilder.ExecuteWithMode(true)
	t.Cleanup(func() { _ = s.globalBuilder.Stop() })
	return s
}

// patchTestWant makes a want and waits for it to be known by id.
func patchTestWant(t *testing.T, s *Server) string {
	t.Helper()
	w := patchTestDo(t, s, "POST", "/api/v1/wants", []map[string]any{{
		"metadata": map[string]any{
			"name":   "patch-me",
			"type":   "queue",
			"labels": map[string]string{"keep": "1", "mywant.io/archived": "true"},
		},
		"spec": map[string]any{"params": map[string]any{"service_time": 0.1, "drop": "x"}},
	}})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	for i := 0; i < 150; i++ {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			if want.Metadata.Name == "patch-me" {
				return want.Metadata.ID
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("want never appeared")
	return ""
}

func TestPatchWantMergesLabelsAndParams(t *testing.T) {
	s := patchTestServer(t)
	id := patchTestWant(t, s)

	w := patchTestDo(t, s, "PATCH", "/api/v1/wants/"+id, map[string]any{
		"metadata": map[string]any{"labels": map[string]any{"mywant.io/archived": nil, "added": "yes"}},
		"spec":     map[string]any{"params": map[string]any{"service_time": 0.5, "drop": nil}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	want, _, found := s.globalBuilder.FindWantByID(id)
	if !found {
		t.Fatal("want gone after patch")
	}
	labels := want.GetLabels()
	if _, ok := labels["mywant.io/archived"]; ok {
		t.Errorf("archived label still there: %v", labels)
	}
	if labels["added"] != "yes" || labels["keep"] != "1" {
		t.Errorf("labels = %v, want added=yes and keep=1 untouched", labels)
	}
	params := want.GetSpec().Params
	if params["service_time"] != 0.5 {
		t.Errorf("service_time = %v", params["service_time"])
	}
	if _, ok := params["drop"]; ok {
		t.Errorf("drop not removed: %v", params)
	}
	if want.Metadata.Name != "patch-me" || want.Metadata.Type != "queue" {
		t.Errorf("name/type changed: %s %s", want.Metadata.Name, want.Metadata.Type)
	}
}

func TestPatchWantRefusesOtherFields(t *testing.T) {
	s := patchTestServer(t)
	id := patchTestWant(t, s)
	for _, body := range []map[string]any{
		{"metadata": map[string]any{"name": "renamed"}},
		{"spec": map[string]any{"requires": []string{"x"}}},
		{"status": "done"},
		{"metadata": map[string]any{"labels": map[string]any{"n": 1}}},
	} {
		if w := patchTestDo(t, s, "PATCH", "/api/v1/wants/"+id, body); w.Code != http.StatusBadRequest {
			t.Errorf("%v: %d, want 400", body, w.Code)
		}
	}
	if w := patchTestDo(t, s, "PATCH", "/api/v1/wants/no-such-want", map[string]any{}); w.Code != http.StatusNotFound {
		t.Errorf("unknown want: %d, want 404", w.Code)
	}
}
