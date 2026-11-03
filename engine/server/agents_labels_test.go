package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// An agent made by POST /agents keeps the labels it was given, and every GET
// of agents answers with them.
func TestAgentLabelsThroughTheAPI(t *testing.T) {
	s := &Server{agentRegistry: mywant.NewAgentRegistry()}
	r := mux.NewRouter()
	r.HandleFunc("/agents", s.createAgent).Methods("POST")
	r.HandleFunc("/agents", s.listAgents).Methods("GET")
	r.HandleFunc("/agents/{name}", s.getAgent).Methods("GET")
	do := func(method, path, body string) map[string]any {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
		}
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	tier := func(a map[string]any) any {
		labels, _ := a["labels"].(map[string]any)
		return labels["tier"]
	}
	if made := do("POST", "/agents", `{"name":"lab","type":"do","capabilities":["c"],"labels":{"tier":"gold"}}`); tier(made) != "gold" {
		t.Errorf("created: %v", made)
	}
	if got := do("GET", "/agents/lab", ""); tier(got) != "gold" {
		t.Errorf("get: %v", got)
	}
	list, _ := do("GET", "/agents", "")["agents"].([]any)
	if len(list) != 1 || tier(list[0].(map[string]any)) != "gold" {
		t.Errorf("list: %v", list)
	}
}
