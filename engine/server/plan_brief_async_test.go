package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

// A brief asked for apart is pending until it is made; ?wait holds the answer
// until then; made, it is there in full; one that could not be made says why;
// one never asked for is not there.
func TestBriefJobFetchedWhenMade(t *testing.T) {
	s := &Server{}
	r := mux.NewRouter()
	r.HandleFunc("/plans/brief/{id}", s.handlePlanBriefGet).Methods("GET")
	get := func(path string) (int, briefJobView) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		var v briefJobView
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		return rec.Code, v
	}

	release := make(chan struct{})
	j := startBriefJob(func() (planBriefResponse, error) {
		<-release
		return planBriefResponse{Prompt: "p", Refs: []string{"-"}}, nil
	})
	if _, v := get("/plans/brief/" + j.id); v.Status != "pending" || v.Brief != nil {
		t.Fatalf("not made yet: %+v", v)
	}
	go func() { time.Sleep(50 * time.Millisecond); close(release) }()
	code, v := get("/plans/brief/" + j.id + "?wait=5")
	if code != http.StatusOK || v.Status != "done" || v.Brief == nil || v.Brief.Prompt != "p" {
		t.Fatalf("waited for: %d %+v", code, v)
	}

	failed := startBriefJob(func() (planBriefResponse, error) { return planBriefResponse{}, errors.New("backend down") })
	if _, v := get("/plans/brief/" + failed.id + "?wait=5"); v.Status != "failed" || v.Message != "backend down" {
		t.Errorf("failed: %+v", v)
	}
	panicked := startBriefJob(func() (planBriefResponse, error) { panic("boom") })
	if _, v := get("/plans/brief/" + panicked.id + "?wait=5"); v.Status != "failed" {
		t.Errorf("a panic is a failure, not the server down: %+v", v)
	}
	if code, _ := get("/plans/brief/brief-none"); code != http.StatusNotFound {
		t.Errorf("never asked for: %d", code)
	}
}
