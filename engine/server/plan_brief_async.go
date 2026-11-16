package server

// plan_brief_async.go — a brief asked for and fetched when it is ready.
//
// A brief is made inside the request that asks for it. On fly's shared CPU,
// throttled once its quota is spent, making one took longer than the phone's
// thirty seconds (2026-11-16: 武蔵小金井から中野坂上, NSURLError -1001) and the
// turn was lost though the brief would have come. So it can be asked for and
// fetched apart, as an act is (acts.go):
//
//	POST /api/v1/plans/brief  {…, "async": true}
//	                          → 202 {id, status: pending}; made on its own
//	GET  /api/v1/plans/brief/{id}[?wait=<seconds>]
//	                          → {id, status: pending | done | failed,
//	                             brief (done), message (failed)};
//	                          ?wait holds the answer until it is made (or the
//	                          seconds are up)
//
// Without async the brief comes in the answer, as before. A brief is kept a
// while after it is made (briefKeep), then let go: nothing is persisted.

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const (
	briefKeep    = 10 * time.Minute
	briefWaitMax = 60 * time.Second
)

type briefJob struct {
	id      string
	created time.Time
	done    chan struct{}
	brief   planBriefResponse
	err     error
}

var briefJobs = struct {
	sync.Mutex
	byID map[string]*briefJob
}{byID: map[string]*briefJob{}}

type briefJobView struct {
	ID      string             `json:"id"`
	Status  string             `json:"status"`
	Brief   *planBriefResponse `json:"brief,omitempty"`
	Message string             `json:"message,omitempty"`
}

func (j *briefJob) view() briefJobView {
	v := briefJobView{ID: j.id, Status: "pending"}
	select {
	case <-j.done:
		if j.err != nil {
			v.Status, v.Message = "failed", j.err.Error()
		} else {
			b := j.brief
			v.Status, v.Brief = "done", &b
		}
	default:
	}
	return v
}

// startBrief makes the brief on its own goroutine and hands back its job.
func (s *Server) startBrief(req planBriefRequest) *briefJob {
	return startBriefJob(func() (planBriefResponse, error) {
		return s.planBrief(req.Question, req.Locale, req.Turn, req.Timezone, req.Since)
	})
}

func startBriefJob(build func() (planBriefResponse, error)) *briefJob {
	j := &briefJob{id: "brief-" + uuid.NewString()[:8], created: time.Now(), done: make(chan struct{})}
	briefJobs.Lock()
	for id, old := range briefJobs.byID {
		if time.Since(old.created) > briefKeep {
			delete(briefJobs.byID, id)
		}
	}
	briefJobs.byID[j.id] = j
	briefJobs.Unlock()
	go func() {
		defer close(j.done)
		defer func() {
			if r := recover(); r != nil {
				j.err = fmt.Errorf("brief panicked: %v", r)
			}
		}()
		j.brief, j.err = build()
	}()
	return j
}

// GET /api/v1/plans/brief/{id}[?wait=<seconds>]
func (s *Server) handlePlanBriefGet(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	briefJobs.Lock()
	j, ok := briefJobs.byID[id]
	briefJobs.Unlock()
	if !ok {
		http.Error(w, "no brief "+id, http.StatusNotFound)
		return
	}
	if secs, err := strconv.ParseFloat(r.URL.Query().Get("wait"), 64); err == nil && secs > 0 {
		wait := time.Duration(secs * float64(time.Second))
		if wait > briefWaitMax {
			wait = briefWaitMax
		}
		select {
		case <-j.done:
		case <-time.After(wait):
		case <-r.Context().Done():
			return
		}
	}
	robotWriteJSON(w, j.view())
}
