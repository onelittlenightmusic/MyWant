package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

// A route asked with Prefer: respond-async answers at once (202, its job);
// the job is pending until the handler is done — ?wait holds it until then —
// and then is the route's own answer, its status and body, the body read and
// the context alive though the request that asked was long answered. Without
// the header the route answers as before; a panic is a 500, not the server
// down; a job never asked for is not there.
func TestAsyncJobsAnswerLater(t *testing.T) {
	release := make(chan struct{})
	slow := func(w http.ResponseWriter, r *http.Request) {
		<-release
		body, _ := io.ReadAll(r.Body)
		if r.Context().Err() != nil {
			http.Error(w, "context gone", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"got":` + string(body) + `}`))
	}
	s := &Server{}
	r := mux.NewRouter()
	r.HandleFunc("/slow", asyncable(slow)).Methods("POST")
	r.HandleFunc("/boom", asyncable(func(http.ResponseWriter, *http.Request) { panic("boom") })).Methods("POST")
	r.HandleFunc("/jobs/{id}", s.handleJobGet).Methods("GET")
	do := func(method, path, body string, async bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if async {
			req.Header.Set("Prefer", "respond-async")
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	idOf := func(rec *httptest.ResponseRecorder) string {
		var v struct{ ID, Status string }
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		if rec.Code != http.StatusAccepted || v.Status != "pending" || rec.Header().Get("Preference-Applied") != "respond-async" {
			t.Fatalf("pending: %d %s", rec.Code, rec.Body)
		}
		return v.ID
	}

	events, cancel := globalSSEHub.subscribe()
	defer cancel()
	id := idOf(do("POST", "/slow", `{"q":1}`, true))
	idOf(do("GET", "/jobs/"+id, "", false)) // still pending
	go func() { time.Sleep(50 * time.Millisecond); close(release) }()
	rec := do("GET", "/jobs/"+id+"?wait=5", "", false)
	if rec.Code != http.StatusCreated || rec.Body.String() != `{"got":{"q":1}}` || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("its own answer: %d %q %v", rec.Code, rec.Body, rec.Header())
	}
	// Told on the event stream, done, with its status.
	select {
	case msg := <-events:
		if !strings.HasPrefix(msg, "event: job\n") || !strings.Contains(msg, `"id":"`+id+`"`) || !strings.Contains(msg, `"code":201`) {
			t.Errorf("told: %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Error("not told on the event stream")
	}
	if again := do("GET", "/jobs/"+id, "", false); again.Code != http.StatusCreated {
		t.Errorf("fetched again: %d", again.Code)
	}
	if rec := do("POST", "/slow", `{"q":2}`, false); rec.Code != http.StatusCreated {
		t.Errorf("without the header, as before: %d %s", rec.Code, rec.Body)
	}
	boom := idOf(do("POST", "/boom", "", true))
	if rec := do("GET", "/jobs/"+boom+"?wait=5", "", false); rec.Code != http.StatusInternalServerError {
		t.Errorf("a panic: %d %s", rec.Code, rec.Body)
	}
	if rec := do("GET", "/jobs/job-none", "", false); rec.Code != http.StatusNotFound {
		t.Errorf("never asked for: %d", rec.Code)
	}
}
