package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The definitions are want-spec's, handed out from the binary: an index of
// its spec files, and each one as YAML or JSON — wherever the server runs.
func TestSpecFromWantSpec(t *testing.T) {
	s := patchTestServer(t)
	get := func(path, accept string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/v1"+path, nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		return rec
	}

	rec := get("/spec", "application/json")
	var idx specIndex
	if err := json.Unmarshal(rec.Body.Bytes(), &idx); err != nil {
		t.Fatal(err, rec.Body)
	}
	has := map[string]bool{}
	for _, f := range idx.Specs {
		has[f] = true
	}
	for _, f := range []string{"plan-spec.yaml", "act-spec.yaml", "agent-spec.yaml", "constellation-spec.yaml", "want-spec.yaml", "schemas/Want.yaml"} {
		if !has[f] {
			t.Errorf("index lacks %s: %v", f, idx.Specs)
		}
	}
	if idx.Module != wantSpecModule {
		t.Errorf("module: %q", idx.Module)
	}
	if y := get("/spec", ""); !strings.Contains(y.Body.String(), "specs:") || y.Header().Get("Content-Type") != "application/yaml" {
		t.Errorf("index as YAML: %s", y.Body)
	}

	if y := get("/spec/plan-spec.yaml", ""); y.Code != http.StatusOK || !strings.Contains(y.Body.String(), "PlanSpec:") {
		t.Errorf("a spec as YAML: %d", y.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(get("/spec/schemas/Want.yaml", "application/json").Body.Bytes(), &doc); err != nil || len(doc) == 0 {
		t.Errorf("a nested spec as JSON: %v %v", doc, err)
	}
	for _, bad := range []string{"/spec/nope.yaml", "/spec/../go.mod", "/spec/%2e%2e/want_spec.go"} {
		// Not found, or (mux cleaning the path) sent elsewhere — never served.
		if rec := get(bad, ""); rec.Code == http.StatusOK {
			t.Errorf("%s served: %.80s", bad, rec.Body)
		}
	}
}
