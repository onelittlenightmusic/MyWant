package server

// plans.go — a plan, as a declarative document kept on the server.
//
// A small on-device model works out what to do about a question with the
// server's help (plan.go's brief and params: what may answer it, and how a
// want would be filled), and what it decided is posted here and kept as a
// Plan — a declaration of what is to be done, nothing done yet:
//
//	apiVersion: mywant.io/v1
//	kind: Plan
//	metadata:
//	  name: plan-1730…            (given by the server)
//	  createdAt: 2026-11-03T…
//	  labels: {device: iphone}    (kept as posted)
//	spec:
//	  question: 最新のゴルフのスコアは？
//	  action: existing             # existing | new | none
//	  target: smartgolf-check-reserved-instance
//	  capability: rounds           # for existing: what is asked of it
//	  type: ""                     # for new: the type to make
//	  params: {}                   # new: its parameters; existing + restart: what it changes
//
// It is carried out by an Act that names it (acts.go). Posted as YAML or
// JSON; read back as JSON, or YAML with ?format=yaml.
//
//	POST   /api/v1/plans          a plan declared → the Plan, named
//	GET    /api/v1/plans          the plans, newest first (?limit=,
//	                              ?labelSelector=k=v,k2!=v2,k3)
//	GET    /api/v1/plans/{name}   one
//	DELETE /api/v1/plans/{name}   let go
//	POST   /api/v1/plans/brief    help to decide: the choices and their prompt
//	POST   /api/v1/plans/params   help to fill a type's parameters
//
// The document is want-spec's Plan (types_plan.go, spec/plan-spec.yaml);
// where it is kept, robot_resources.go.

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	ws "github.com/onelittlenightmusic/want-spec"
	"gopkg.in/yaml.v3"
)

// readDoc reads a posted document, YAML or JSON (JSON is YAML).
func readDoc(r *http.Request, into any) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, into)
}

// writeDoc answers with a document: JSON, or YAML when ?format=yaml (or the
// request accepts YAML).
func writeDoc(w http.ResponseWriter, r *http.Request, status int, doc any) {
	if r.URL.Query().Get("format") == "yaml" || strings.Contains(r.Header.Get("Accept"), "yaml") {
		out, err := yaml.Marshal(doc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(status)
		_, _ = w.Write(out)
		return
	}
	if status != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
	}
	robotWriteJSON(w, doc)
}

func listLimit(r *http.Request, max int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v < max {
		return v
	}
	return max
}

// listSelector is ?labelSelector=k=v,… (kubectl's -l); a bad one is
// answered 400 and ok is false.
func listSelector(w http.ResponseWriter, r *http.Request) (ws.LabelSelector, bool) {
	sel, err := ws.ParseLabelSelector(r.URL.Query().Get("labelSelector"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}
	return sel, true
}

func planNamed(name string) (*ws.Plan, bool) {
	lockRobotRes()
	defer robotRes.mu.Unlock()
	for _, p := range robotRes.plans {
		if p.Metadata.Name == name {
			return p, true
		}
	}
	return nil, false
}

// POST /api/v1/plans
func (s *Server) handlePlanCreate(w http.ResponseWriter, r *http.Request) {
	var doc ws.Plan
	if err := readDoc(r, &doc); err != nil {
		http.Error(w, "bad plan: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := doc.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	doc.Spec.Normalize()
	now := time.Now()
	doc.APIVersion, doc.Kind = ws.ResourceAPIVersion, ws.PlanKind
	doc.Metadata = ws.ResourceMeta{Name: fmt.Sprintf("plan-%d", now.UnixNano()), CreatedAt: now.Format(time.RFC3339), Labels: doc.Metadata.Labels}

	lockRobotRes()
	dir := robotRes.plansDir
	robotRes.mu.Unlock()
	// Written before it is listed, so one deleted at once is not written back.
	if err := dir.Save(doc.Metadata.Name, &doc); err != nil {
		log.Printf("[WARN] plans: %v", err)
	}
	lockRobotRes()
	robotRes.plans = append(robotRes.plans, &doc)
	plans, acts := pruneRobotResLocked(now)
	robotRes.mu.Unlock()
	removeRobotRes(plans, acts)

	w.Header().Set("Location", "/api/v1/plans/"+doc.Metadata.Name)
	writeDoc(w, r, http.StatusCreated, &doc)
}

// GET /api/v1/plans
func (s *Server) handlePlanList(w http.ResponseWriter, r *http.Request) {
	limit := listLimit(r, plansKept)
	sel, ok := listSelector(w, r)
	if !ok {
		return
	}
	lockRobotRes()
	out := ws.PlanList{Plans: make([]ws.Plan, 0, limit)}
	for i := len(robotRes.plans) - 1; i >= 0 && len(out.Plans) < limit; i-- {
		if sel.Matches(robotRes.plans[i].Metadata.Labels) {
			out.Plans = append(out.Plans, *robotRes.plans[i])
		}
	}
	robotRes.mu.Unlock()
	writeDoc(w, r, http.StatusOK, &out)
}

// GET /api/v1/plans/{name}
func (s *Server) handlePlanGet(w http.ResponseWriter, r *http.Request) {
	if p, ok := planNamed(mux.Vars(r)["name"]); ok {
		writeDoc(w, r, http.StatusOK, p)
		return
	}
	http.Error(w, "no plan "+mux.Vars(r)["name"], http.StatusNotFound)
}

// DELETE /api/v1/plans/{name}
func (s *Server) handlePlanDelete(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	lockRobotRes()
	for i, p := range robotRes.plans {
		if p.Metadata.Name == name {
			robotRes.plans = append(robotRes.plans[:i], robotRes.plans[i+1:]...)
			robotRes.mu.Unlock()
			removeRobotRes([]string{name}, nil)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	robotRes.mu.Unlock()
	http.Error(w, "no plan "+name, http.StatusNotFound)
}
