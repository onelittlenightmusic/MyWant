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
//	spec:
//	  question: 最新のゴルフのスコアは？
//	  action: existing             # existing | new | none
//	  target: smartgolf-check-reserved-instance
//	  capability: rounds           # for existing: what is asked of it
//	  type: ""                     # for new: the type to make
//	  params: {}                   # for new: its parameters
//
// It is carried out by an Act that names it (acts.go). Posted as YAML or
// JSON; read back as JSON, or YAML with ?format=yaml.
//
//	POST   /api/v1/plans          a plan declared → the Plan, named
//	GET    /api/v1/plans          the plans, newest first (?limit=)
//	GET    /api/v1/plans/{name}   one
//	DELETE /api/v1/plans/{name}   let go
//	POST   /api/v1/plans/brief    help to decide: the choices and their prompt
//	POST   /api/v1/plans/params   help to fill a type's parameters

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

const (
	resourceAPIVersion = "mywant.io/v1"
	plansKept          = 200
)

type resourceMeta struct {
	Name      string `json:"name" yaml:"name"`
	CreatedAt string `json:"createdAt" yaml:"createdAt"`
}

// planSpec is what is to be done about a question.
type planSpec struct {
	Question   string            `json:"question" yaml:"question"`
	Action     string            `json:"action" yaml:"action"` // existing | new | none
	Target     string            `json:"target,omitempty" yaml:"target,omitempty"`
	Capability string            `json:"capability,omitempty" yaml:"capability,omitempty"`
	Type       string            `json:"type,omitempty" yaml:"type,omitempty"`
	Params     map[string]string `json:"params,omitempty" yaml:"params,omitempty"`
}

type planDoc struct {
	APIVersion string       `json:"apiVersion" yaml:"apiVersion"`
	Kind       string       `json:"kind" yaml:"kind"`
	Metadata   resourceMeta `json:"metadata" yaml:"metadata"`
	Spec       planSpec     `json:"spec" yaml:"spec"`
}

var planStore struct {
	mu   sync.Mutex
	list []*planDoc // oldest first
}

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

func planNamed(name string) (*planDoc, bool) {
	planStore.mu.Lock()
	defer planStore.mu.Unlock()
	for _, p := range planStore.list {
		if p.Metadata.Name == name {
			return p, true
		}
	}
	return nil, false
}

// POST /api/v1/plans
func (s *Server) handlePlanCreate(w http.ResponseWriter, r *http.Request) {
	var doc planDoc
	if err := readDoc(r, &doc); err != nil {
		http.Error(w, "bad plan: "+err.Error(), http.StatusBadRequest)
		return
	}
	if doc.Kind != "" && doc.Kind != "Plan" {
		http.Error(w, "kind must be Plan", http.StatusBadRequest)
		return
	}
	switch doc.Spec.Action {
	case "existing", "new", "none":
	default:
		http.Error(w, "spec.action must be existing, new or none", http.StatusBadRequest)
		return
	}
	// "-" is the decision's own word for nothing chosen.
	for _, f := range []*string{&doc.Spec.Target, &doc.Spec.Capability, &doc.Spec.Type} {
		if strings.TrimSpace(*f) == planNone {
			*f = ""
		}
	}
	now := time.Now()
	doc.APIVersion, doc.Kind = resourceAPIVersion, "Plan"
	doc.Metadata = resourceMeta{Name: fmt.Sprintf("plan-%d", now.UnixNano()), CreatedAt: now.Format(time.RFC3339)}
	planStore.mu.Lock()
	planStore.list = append(planStore.list, &doc)
	if len(planStore.list) > plansKept {
		planStore.list = planStore.list[len(planStore.list)-plansKept:]
	}
	planStore.mu.Unlock()
	w.Header().Set("Location", "/api/v1/plans/"+doc.Metadata.Name)
	writeDoc(w, r, http.StatusCreated, &doc)
}

// GET /api/v1/plans
func (s *Server) handlePlanList(w http.ResponseWriter, r *http.Request) {
	limit := listLimit(r, plansKept)
	planStore.mu.Lock()
	out := make([]*planDoc, 0, limit)
	for i := len(planStore.list) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, planStore.list[i])
	}
	planStore.mu.Unlock()
	writeDoc(w, r, http.StatusOK, map[string]any{"plans": out})
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
	planStore.mu.Lock()
	defer planStore.mu.Unlock()
	for i, p := range planStore.list {
		if p.Metadata.Name == name {
			planStore.list = append(planStore.list[:i], planStore.list[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	http.Error(w, "no plan "+name, http.StatusNotFound)
}
