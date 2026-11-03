package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// A plan is declared in YAML (or JSON), named by the server, read back as
// either; one that says no action, or another kind, is refused; an act must
// name a plan there is.
func TestPlansAndActsDocuments(t *testing.T) {
	s := &Server{}
	r := mux.NewRouter()
	r.HandleFunc("/plans", s.handlePlanCreate).Methods("POST")
	r.HandleFunc("/plans/{name}", s.handlePlanGet).Methods("GET")
	r.HandleFunc("/acts", s.handleActCreate).Methods("POST")
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}

	rec := do("POST", "/plans", "spec:\n  question: 最新のゴルフのスコアは？\n  action: existing\n  target: golf\n  capability: rounds\n  type: \"-\"\n")
	if rec.Code != http.StatusCreated {
		t.Fatalf("plan: %d %s", rec.Code, rec.Body)
	}
	var plan planDoc
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Kind != "Plan" || plan.APIVersion != resourceAPIVersion || !strings.HasPrefix(plan.Metadata.Name, "plan-") {
		t.Errorf("named and kinded: %+v", plan)
	}
	if plan.Spec.Capability != "rounds" || plan.Spec.Type != "" {
		t.Errorf("spec as declared, - as nothing: %+v", plan.Spec)
	}
	if y := do("GET", "/plans/"+plan.Metadata.Name+"?format=yaml", ""); !strings.Contains(y.Body.String(), "kind: Plan") {
		t.Errorf("read back as YAML: %s", y.Body)
	}

	if rec := do("POST", "/plans", `{"spec":{"question":"x","action":"jump"}}`); rec.Code != http.StatusBadRequest {
		t.Errorf("an action there is not: %d", rec.Code)
	}
	if rec := do("POST", "/plans", `{"kind":"Act","spec":{"action":"none"}}`); rec.Code != http.StatusBadRequest {
		t.Errorf("another kind: %d", rec.Code)
	}
	if rec := do("POST", "/acts", `{"spec":{"plan":"plan-none"}}`); rec.Code != http.StatusNotFound {
		t.Errorf("an act of no plan: %d", rec.Code)
	}
}
