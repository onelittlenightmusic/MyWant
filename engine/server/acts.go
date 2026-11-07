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
//	GET    /api/v1/acts                the acts, newest first (?limit=,
//	                                   ?labelSelector=k=v,…)
//	GET    /api/v1/acts/{name}         one, as far as it has got;
//	                                   ?wait=<seconds> holds the answer until
//	                                   it is done (or the seconds are up)
//	DELETE /api/v1/acts/{name}         its record let go — what it did on the
//	                                   board stays done
//
// The document is want-spec's Act (types_act.go, spec/act-spec.yaml); where
// it is kept, robot_resources.go — written again at each step, so it is there
// after a restart (Failed, if the restart caught it unfinished).

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	ws "github.com/onelittlenightmusic/want-spec"
)

const actWaitMax = 120 * time.Second

func actNamed(name string) (*actEntry, bool) {
	lockRobotRes()
	defer robotRes.mu.Unlock()
	for _, a := range robotRes.acts {
		if a.doc.Metadata.Name == name {
			return a, true
		}
	}
	return nil, false
}

// snapshotLocked is the act as it stands, copied (robotRes.mu held).
func (a *actEntry) snapshotLocked() ws.Act {
	d := a.doc
	d.Status.Steps = append([]ws.RobotStep{}, a.doc.Status.Steps...)
	d.Status.Cards = append([]*ws.RobotCard{}, a.doc.Status.Cards...)
	return d
}

// snapshot is the act as it stands, copied out from under the lock.
func (a *actEntry) snapshot() ws.Act {
	robotRes.mu.Lock()
	defer robotRes.mu.Unlock()
	return a.snapshotLocked()
}

// update changes the act and writes it.
func (a *actEntry) update(f func(d *ws.Act)) {
	robotRes.mu.Lock()
	f(&a.doc)
	robotRes.mu.Unlock()
	a.save()
}

// POST /api/v1/acts
func (s *Server) handleActCreate(w http.ResponseWriter, r *http.Request) {
	var doc ws.Act
	if err := readDoc(r, &doc); err != nil {
		http.Error(w, "bad act: "+err.Error(), http.StatusBadRequest)
		return
	}
	if doc.Kind != "" && doc.Kind != ws.ActKind {
		http.Error(w, "kind must be "+ws.ActKind, http.StatusBadRequest)
		return
	}
	plan, ok := planNamed(doc.Spec.Plan)
	if !ok {
		http.Error(w, "spec.plan names no plan: "+doc.Spec.Plan, http.StatusNotFound)
		return
	}
	now := time.Now()
	doc.APIVersion, doc.Kind = ws.ResourceAPIVersion, ws.ActKind
	doc.Metadata = ws.ResourceMeta{Name: fmt.Sprintf("act-%d", now.UnixNano()), CreatedAt: now.Format(time.RFC3339), Labels: doc.Metadata.Labels}
	doc.Status = ws.ActStatus{Phase: ws.ActPending, Steps: []ws.RobotStep{}, Cards: []*ws.RobotCard{}}
	a := &actEntry{doc: doc, done: make(chan struct{})}
	lockRobotRes()
	robotRes.acts = append(robotRes.acts, a)
	plans, acts := pruneRobotResLocked(now)
	robotRes.mu.Unlock()
	removeRobotRes(plans, acts)
	a.save()

	go s.runAct(a, plan.Spec)

	w.Header().Set("Location", "/api/v1/acts/"+doc.Metadata.Name)
	snap := a.snapshot()
	writeDoc(w, r, http.StatusCreated, &snap)
}

// runAct carries the plan out, writing the act's status as it goes.
func (s *Server) runAct(a *actEntry, plan ws.PlanSpec) {
	defer close(a.done)
	// What the plan did, into the asker's story (story.go).
	defer func() { story.actDone(a.snapshot(), plan) }()
	defer func() {
		if p := recover(); p != nil {
			a.update(func(d *ws.Act) {
				d.Status.Phase = ws.ActFailed
				d.Status.Message = fmt.Sprint(p)
				d.Status.CompletionTime = time.Now().Format(time.RFC3339)
			})
		}
	}()
	spec := a.snapshot().Spec
	a.update(func(d *ws.Act) {
		d.Status.Phase = ws.ActRunning
		d.Status.StartTime = time.Now().Format(time.RFC3339)
	})
	req := actRequest{
		Turn: spec.Turn, Question: plan.Question, Locale: spec.Locale,
		Kind: plan.Action, Ref: plan.Target, Capability: plan.Capability,
		Type: plan.Type, Params: plan.Params,
	}
	seen := map[string]bool{}
	prompt, _, _ := s.doAct(req, func(st robotStep) {
		a.update(func(d *ws.Act) {
			d.Status.Steps = append(d.Status.Steps, st)
			if st.Card != nil && !seen[st.Card.ID] {
				seen[st.Card.ID] = true
				d.Status.Cards = append(d.Status.Cards, st.Card)
			}
		})
	})
	// Worded already, where the type's answer template is filled by what
	// the want found (plan_answer.go).
	answer := s.actAnswer(plan, spec.Locale, a.snapshot().Status.Cards)
	a.update(func(d *ws.Act) {
		d.Status.Phase = ws.ActSucceeded
		d.Status.Prompt = prompt
		d.Status.Answer = answer
		d.Status.CompletionTime = time.Now().Format(time.RFC3339)
	})
}

// GET /api/v1/acts
func (s *Server) handleActList(w http.ResponseWriter, r *http.Request) {
	limit := listLimit(r, actsKept)
	sel, ok := listSelector(w, r)
	if !ok {
		return
	}
	lockRobotRes()
	out := ws.ActList{Acts: make([]ws.Act, 0, limit)}
	for i := len(robotRes.acts) - 1; i >= 0 && len(out.Acts) < limit; i-- {
		if sel.Matches(robotRes.acts[i].doc.Metadata.Labels) {
			out.Acts = append(out.Acts, robotRes.acts[i].snapshotLocked())
		}
	}
	robotRes.mu.Unlock()
	writeDoc(w, r, http.StatusOK, &out)
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
	lockRobotRes()
	for i, a := range robotRes.acts {
		if a.doc.Metadata.Name == name {
			robotRes.acts = append(robotRes.acts[:i], robotRes.acts[i+1:]...)
			a.deleted = true
			robotRes.mu.Unlock()
			removeRobotRes(nil, []*actEntry{a})
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	robotRes.mu.Unlock()
	http.Error(w, "no act "+name, http.StatusNotFound)
}
