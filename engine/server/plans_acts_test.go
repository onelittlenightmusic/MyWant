package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	ws "github.com/onelittlenightmusic/want-spec"
)

// A plan is declared in YAML (or JSON), named by the server, read back as
// either; one that says no action, or another kind, is refused; an act must
// name a plan there is.
func TestPlansAndActsDocuments(t *testing.T) {
	useRobotResHome(t.TempDir())
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
	var plan ws.Plan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Kind != "Plan" || plan.APIVersion != ws.ResourceAPIVersion || !strings.HasPrefix(plan.Metadata.Name, "plan-") {
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

// Plans and acts outlive the server: written under ~/.mywant (here a temp
// dir), read back when asked for again; a delete lets the file go too; an act
// the stop caught unfinished is read back Failed, and done.
func TestPlansAndActsKeptOnDisk(t *testing.T) {
	home := t.TempDir()
	useRobotResHome(home)
	s := &Server{}
	r := mux.NewRouter()
	r.HandleFunc("/plans", s.handlePlanCreate).Methods("POST")
	r.HandleFunc("/plans", s.handlePlanList).Methods("GET")
	r.HandleFunc("/plans/{name}", s.handlePlanGet).Methods("GET")
	r.HandleFunc("/plans/{name}", s.handlePlanDelete).Methods("DELETE")
	r.HandleFunc("/acts", s.handleActList).Methods("GET")
	r.HandleFunc("/acts/{name}", s.handleActGet).Methods("GET")
	r.HandleFunc("/acts/{name}", s.handleActDelete).Methods("DELETE")
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	name := func(rec *httptest.ResponseRecorder) string {
		var p ws.Plan
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p.Metadata.Name
	}

	kept := name(do("POST", "/plans", "spec: {question: a, action: none}"))
	gone := name(do("POST", "/plans", `{"spec":{"question":"b","action":"none"}}`))
	if rec := do("DELETE", "/plans/"+gone, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(home, "plans", gone+".yaml")); !os.IsNotExist(err) {
		t.Errorf("a deleted plan's file is gone: %v", err)
	}

	// An act the server was carrying out when it stopped.
	running := ws.Act{APIVersion: ws.ResourceAPIVersion, Kind: ws.ActKind,
		Metadata: ws.ResourceMeta{Name: "act-1", CreatedAt: time.Now().Format(time.RFC3339)},
		Spec:     ws.ActSpec{Plan: kept},
		Status:   ws.ActStatus{Phase: ws.ActRunning, Steps: []ws.RobotStep{{Tool: "point", Done: true}}}}
	data, _ := yaml.Marshal(&running)
	_ = os.MkdirAll(filepath.Join(home, "acts"), 0755)
	if err := os.WriteFile(filepath.Join(home, "acts", "act-1.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	useRobotResHome(home) // the server started again
	if rec := do("GET", "/plans/"+kept, ""); rec.Code != http.StatusOK {
		t.Errorf("a plan read back: %d %s", rec.Code, rec.Body)
	}
	var list ws.PlanList
	_ = json.Unmarshal(do("GET", "/plans", "").Body.Bytes(), &list)
	if len(list.Plans) != 1 || list.Plans[0].Metadata.Name != kept {
		t.Errorf("only the plan kept: %+v", list.Plans)
	}
	var act ws.Act
	_ = json.Unmarshal(do("GET", "/acts/act-1?wait=5", "").Body.Bytes(), &act)
	if act.Status.Phase != ws.ActFailed || act.Status.Message == "" || len(act.Status.Steps) != 1 {
		t.Errorf("an unfinished act read back Failed, its steps kept: %+v", act.Status)
	}
	if y := do("GET", "/acts/act-1?format=yaml", ""); !strings.Contains(y.Body.String(), "phase: Failed") {
		t.Errorf("as YAML: %s", y.Body)
	}
	useRobotResHome(home)
	_ = json.Unmarshal(do("GET", "/acts/act-1", "").Body.Bytes(), &act)
	if act.Status.Phase != ws.ActFailed {
		t.Errorf("written back Failed: %+v", act.Status)
	}
	if rec := do("DELETE", "/acts/act-1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete act: %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(home, "acts", "act-1.yaml")); !os.IsNotExist(err) {
		t.Errorf("a deleted act's file is gone: %v", err)
	}
}

// Past keeping: too many, or too old, let go — but not a plan an act kept
// names, nor an act not finished.
func TestRobotResourcesPruned(t *testing.T) {
	useRobotResHome(t.TempDir())
	lockRobotRes()
	defer robotRes.mu.Unlock()
	now := time.Now()
	old := now.Add(-resourcesKeptFor - time.Hour)
	meta := func(kind string, at time.Time) ws.ResourceMeta {
		return ws.ResourceMeta{Name: fmt.Sprintf("%s-%d", kind, at.UnixNano())}
	}
	oldPlan, namedPlan, newPlan := meta("plan", old), meta("plan", old.Add(time.Second)), meta("plan", now)
	robotRes.plans = []*ws.Plan{{Metadata: oldPlan}, {Metadata: namedPlan}, {Metadata: newPlan}}
	oldAct := &actEntry{doc: ws.Act{Metadata: meta("act", old), Status: ws.ActStatus{Phase: ws.ActSucceeded}}}
	oldRunning := &actEntry{doc: ws.Act{Metadata: meta("act", old.Add(time.Second)), Spec: ws.ActSpec{Plan: namedPlan.Name}, Status: ws.ActStatus{Phase: ws.ActRunning}}}
	robotRes.acts = []*actEntry{oldAct, oldRunning}

	plans, acts := pruneRobotResLocked(now)
	if len(plans) != 1 || plans[0] != oldPlan.Name {
		t.Errorf("plans let go: %v", plans)
	}
	if len(acts) != 1 || acts[0] != oldAct || !oldAct.deleted {
		t.Errorf("acts let go: %v", acts)
	}
	if len(robotRes.plans) != 2 || len(robotRes.acts) != 1 {
		t.Errorf("kept: %d plans, %d acts", len(robotRes.plans), len(robotRes.acts))
	}
}
