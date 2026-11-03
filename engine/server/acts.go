package server

// acts.go — a plan carried out, kept like a Kubernetes Job.
//
// Posting an Act that names a Plan (plans.go) is what makes something happen
// on the board. The Act is answered at once, Pending, and carried out behind
// it; its status is written as it goes, so reading it back says how far it
// has got:
//
//	apiVersion: mywant.io/v1
//	kind: Act
//	metadata:
//	  name: act-1730…
//	  createdAt: …
//	spec:
//	  plan: plan-1730…           # the Plan carried out
//	  turn: turn-…               # the robot's turn it belongs to, if any
//	  locale: ja_JP
//	status:
//	  phase: Running             # Pending → Running → Succeeded | Failed
//	  startTime: …
//	  completionTime: …
//	  steps:                     # each tool run as it is done — the same
//	  - tool: point              #   runs /api/v1/tools/call makes
//	    arguments: {name: …}
//	    output: …
//	  cards: […]                 # what the steps showed
//	  prompt: …                  # when done: what to word the answer from
//	  message: …                 # when failed: why
//
//	POST   /api/v1/acts                a plan to carry out → the Act, Pending
//	GET    /api/v1/acts                the acts, newest first (?limit=)
//	GET    /api/v1/acts/{name}         one, as far as it has got;
//	                                   ?wait=<seconds> holds the answer until
//	                                   it is done (or the seconds are up)
//	DELETE /api/v1/acts/{name}         its record let go — what it did on the
//	                                   board stays done
//
// Kept in memory, the latest actsKept.

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

const (
	actsKept   = 200
	actWaitMax = 120 * time.Second
)

const (
	actPending   = "Pending"
	actRunning   = "Running"
	actSucceeded = "Succeeded"
	actFailed    = "Failed"
)

type actSpec struct {
	Plan   string `json:"plan" yaml:"plan"`
	Turn   string `json:"turn,omitempty" yaml:"turn,omitempty"`
	Locale string `json:"locale,omitempty" yaml:"locale,omitempty"`
}

type actStatus struct {
	Phase          string       `json:"phase" yaml:"phase"`
	StartTime      string       `json:"startTime,omitempty" yaml:"startTime,omitempty"`
	CompletionTime string       `json:"completionTime,omitempty" yaml:"completionTime,omitempty"`
	Steps          []robotStep  `json:"steps" yaml:"steps"`
	Cards          []*robotCard `json:"cards" yaml:"cards"`
	Prompt         string       `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Message        string       `json:"message,omitempty" yaml:"message,omitempty"`
}

type actDoc struct {
	APIVersion string       `json:"apiVersion" yaml:"apiVersion"`
	Kind       string       `json:"kind" yaml:"kind"`
	Metadata   resourceMeta `json:"metadata" yaml:"metadata"`
	Spec       actSpec      `json:"spec" yaml:"spec"`
	Status     actStatus    `json:"status" yaml:"status"`
}

type actEntry struct {
	doc  actDoc
	done chan struct{} // closed when the act has finished
}

var actStore struct {
	mu   sync.Mutex
	list []*actEntry // oldest first
}

func (d *actDoc) finished() bool {
	return d.Status.Phase == actSucceeded || d.Status.Phase == actFailed
}

func actNamed(name string) (*actEntry, bool) {
	actStore.mu.Lock()
	defer actStore.mu.Unlock()
	for _, a := range actStore.list {
		if a.doc.Metadata.Name == name {
			return a, true
		}
	}
	return nil, false
}

// snapshot is the act as it stands, copied out from under the lock.
func (a *actEntry) snapshot() actDoc {
	actStore.mu.Lock()
	defer actStore.mu.Unlock()
	d := a.doc
	d.Status.Steps = append([]robotStep{}, a.doc.Status.Steps...)
	d.Status.Cards = append([]*robotCard{}, a.doc.Status.Cards...)
	return d
}

func (a *actEntry) update(f func(d *actDoc)) {
	actStore.mu.Lock()
	f(&a.doc)
	actStore.mu.Unlock()
}

// POST /api/v1/acts
func (s *Server) handleActCreate(w http.ResponseWriter, r *http.Request) {
	var doc actDoc
	if err := readDoc(r, &doc); err != nil {
		http.Error(w, "bad act: "+err.Error(), http.StatusBadRequest)
		return
	}
	if doc.Kind != "" && doc.Kind != "Act" {
		http.Error(w, "kind must be Act", http.StatusBadRequest)
		return
	}
	plan, ok := planNamed(doc.Spec.Plan)
	if !ok {
		http.Error(w, "spec.plan names no plan: "+doc.Spec.Plan, http.StatusNotFound)
		return
	}
	now := time.Now()
	doc.APIVersion, doc.Kind = resourceAPIVersion, "Act"
	doc.Metadata = resourceMeta{Name: fmt.Sprintf("act-%d", now.UnixNano()), CreatedAt: now.Format(time.RFC3339)}
	doc.Status = actStatus{Phase: actPending, Steps: []robotStep{}, Cards: []*robotCard{}}
	a := &actEntry{doc: doc, done: make(chan struct{})}
	actStore.mu.Lock()
	actStore.list = append(actStore.list, a)
	if len(actStore.list) > actsKept {
		actStore.list = actStore.list[len(actStore.list)-actsKept:]
	}
	actStore.mu.Unlock()

	go s.runAct(a, plan.Spec)

	w.Header().Set("Location", "/api/v1/acts/"+doc.Metadata.Name)
	snap := a.snapshot()
	writeDoc(w, r, http.StatusCreated, &snap)
}

// runAct carries the plan out, writing the act's status as it goes.
func (s *Server) runAct(a *actEntry, plan planSpec) {
	defer close(a.done)
	defer func() {
		if p := recover(); p != nil {
			a.update(func(d *actDoc) {
				d.Status.Phase = actFailed
				d.Status.Message = fmt.Sprint(p)
				d.Status.CompletionTime = time.Now().Format(time.RFC3339)
			})
		}
	}()
	spec := a.snapshot().Spec
	a.update(func(d *actDoc) {
		d.Status.Phase = actRunning
		d.Status.StartTime = time.Now().Format(time.RFC3339)
	})
	seen := map[string]bool{}
	prompt, _, _ := s.doAct(actRequest{
		Turn: spec.Turn, Question: plan.Question, Locale: spec.Locale,
		Kind: plan.Action, Ref: plan.Target, Capability: plan.Capability,
		Type: plan.Type, Params: plan.Params,
	}, func(st robotStep) {
		a.update(func(d *actDoc) {
			d.Status.Steps = append(d.Status.Steps, st)
			if st.Card != nil && !seen[st.Card.ID] {
				seen[st.Card.ID] = true
				d.Status.Cards = append(d.Status.Cards, st.Card)
			}
		})
	})
	a.update(func(d *actDoc) {
		d.Status.Phase = actSucceeded
		d.Status.Prompt = prompt
		d.Status.CompletionTime = time.Now().Format(time.RFC3339)
	})
}

// GET /api/v1/acts
func (s *Server) handleActList(w http.ResponseWriter, r *http.Request) {
	limit := listLimit(r, actsKept)
	actStore.mu.Lock()
	entries := make([]*actEntry, 0, limit)
	for i := len(actStore.list) - 1; i >= 0 && len(entries) < limit; i-- {
		entries = append(entries, actStore.list[i])
	}
	actStore.mu.Unlock()
	out := make([]actDoc, len(entries))
	for i, a := range entries {
		out[i] = a.snapshot()
	}
	writeDoc(w, r, http.StatusOK, map[string]any{"acts": out})
}

// GET /api/v1/acts/{name}[?wait=<seconds>]
func (s *Server) handleActGet(w http.ResponseWriter, r *http.Request) {
	a, ok := actNamed(mux.Vars(r)["name"])
	if !ok {
		http.Error(w, "no act "+mux.Vars(r)["name"], http.StatusNotFound)
		return
	}
	if secs, err := strconv.ParseFloat(r.URL.Query().Get("wait"), 64); err == nil && secs > 0 {
		wait := time.Duration(secs * float64(time.Second))
		if wait > actWaitMax {
			wait = actWaitMax
		}
		select {
		case <-a.done:
		case <-time.After(wait):
		case <-r.Context().Done():
			return
		}
	}
	snap := a.snapshot()
	writeDoc(w, r, http.StatusOK, &snap)
}

// DELETE /api/v1/acts/{name}
func (s *Server) handleActDelete(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	actStore.mu.Lock()
	defer actStore.mu.Unlock()
	for i, a := range actStore.list {
		if a.doc.Metadata.Name == name {
			actStore.list = append(actStore.list[:i], actStore.list[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	http.Error(w, "no act "+name, http.StatusNotFound)
}
