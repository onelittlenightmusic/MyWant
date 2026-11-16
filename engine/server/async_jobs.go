package server

// async_jobs.go — a request answered later, fetched when it is done.
//
// Some requests do their work before they answer: a plan's brief and its
// parameters build a prompt from the whole board, a magic sentence waits for
// the want it made to answer, a tool may wait for a want run again. On fly's
// shared CPU, throttled once its quota is spent, that took longer than the
// phone's thirty seconds and what was asked was lost though the answer would
// have come (2026-11-16: 武蔵小金井から中野坂上, NSURLError -1001). So any of
// them can be asked for at once and fetched apart — one way for all:
//
//	POST <route> with  Prefer: respond-async
//	    → 202 {id, status: pending}, Preference-Applied: respond-async,
//	      Location: /api/v1/jobs/{id}; the handler runs on its own
//	GET  /api/v1/jobs/{id}[?wait=<seconds>]
//	    → while it runs: 202 {id, status: pending} (Preference-Applied too);
//	      ?wait holds the answer until it is done (or the seconds are up)
//	    → done: the route's own answer, as it would have come — its status,
//	      its Content-Type, its body
//
// Done, a job is told on the event stream (GET /api/v1/events, handlers_sse.go)
// as `event: job` {id, route, status: done, code}: a client holding the stream
// fetches it then; one that is not waits on ?wait.
//
// Without the header a route answers in the request, as before. A job is kept
// a while after it is asked for (asyncJobKeep), in memory only.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const (
	asyncJobKeep    = 10 * time.Minute
	asyncJobWaitMax = 60 * time.Second
	// asyncPreference is RFC 7240's: the client would rather not wait.
	asyncPreference = "respond-async"
)

type asyncJob struct {
	id      string
	route   string
	created time.Time
	done    chan struct{}
	rec     *jobRecorder
}

var asyncJobs = struct {
	sync.Mutex
	byID map[string]*asyncJob
}{byID: map[string]*asyncJob{}}

// jobRecorder keeps what a handler answers, to hand on when it is fetched.
type jobRecorder struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (r *jobRecorder) Header() http.Header { return r.header }
func (r *jobRecorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.body.Write(b)
}
func (r *jobRecorder) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
}

// wantsAsync: the request asks to be answered at once (Prefer: respond-async).
func wantsAsync(r *http.Request) bool {
	for _, v := range r.Header.Values("Prefer") {
		for _, p := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(p), asyncPreference) {
				return true
			}
		}
	}
	return false
}

// asyncable lets a route be asked for at once and fetched from /jobs/{id}.
func asyncable(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !wantsAsync(r) {
			h(w, r)
			return
		}
		// The body read now: the request is answered before the handler
		// reads it. Its context let go of the request's, which ends with the
		// 202.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		inner := r.Clone(context.WithoutCancel(r.Context()))
		inner.Body = io.NopCloser(bytes.NewReader(body))
		inner.ContentLength = int64(len(body))
		inner.Header.Del("Prefer")
		j := startAsyncJob(r.URL.Path, func(rec *jobRecorder) { h(rec, inner) })
		writeJobPending(w, j)
	}
}

func startAsyncJob(route string, run func(*jobRecorder)) *asyncJob {
	j := &asyncJob{id: "job-" + uuid.NewString()[:8], route: route, created: time.Now(),
		done: make(chan struct{}), rec: &jobRecorder{header: http.Header{}}}
	asyncJobs.Lock()
	for id, old := range asyncJobs.byID {
		if time.Since(old.created) > asyncJobKeep {
			delete(asyncJobs.byID, id)
		}
	}
	asyncJobs.byID[j.id] = j
	asyncJobs.Unlock()
	go func() {
		defer close(j.done)
		defer func() {
			if p := recover(); p != nil {
				log.Printf("[jobs] %s %s panicked: %v", j.id, route, p)
				j.rec.header = http.Header{"Content-Type": {"text/plain; charset=utf-8"}}
				j.rec.code = http.StatusInternalServerError
				j.rec.body.Reset()
				fmt.Fprintf(&j.rec.body, "%s failed: %v\n", route, p)
			}
		}()
		run(j.rec)
		if j.rec.code == 0 {
			j.rec.code = http.StatusOK
		}
	}()
	// Told on the event stream when it is done: a client holding the stream
	// fetches it then, without a request held open for it. Only that it is
	// done — every client is told, and a brief is kilobytes.
	go func() {
		<-j.done
		broadcastSSE("job", map[string]any{"id": j.id, "route": route, "status": "done", "code": j.rec.code})
	}()
	return j
}

func writeJobPending(w http.ResponseWriter, j *asyncJob) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Preference-Applied", asyncPreference)
	w.Header().Set("Location", "/api/v1/jobs/"+j.id)
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": j.id, "status": "pending", "route": j.route})
}

// GET /api/v1/jobs/{id}[?wait=<seconds>]
func (s *Server) handleJobGet(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	asyncJobs.Lock()
	j, ok := asyncJobs.byID[id]
	asyncJobs.Unlock()
	if !ok {
		http.Error(w, "no job "+id, http.StatusNotFound)
		return
	}
	if secs, err := strconv.ParseFloat(r.URL.Query().Get("wait"), 64); err == nil && secs > 0 {
		wait := time.Duration(secs * float64(time.Second))
		if wait > asyncJobWaitMax {
			wait = asyncJobWaitMax
		}
		select {
		case <-j.done:
		case <-time.After(wait):
		case <-r.Context().Done():
			return
		}
	}
	select {
	case <-j.done:
	default:
		writeJobPending(w, j)
		return
	}
	for k, vs := range j.rec.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(j.rec.code)
	_, _ = w.Write(j.rec.body.Bytes())
}
