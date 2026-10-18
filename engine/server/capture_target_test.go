package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	mwlabels "mywant/engine/labels"
)

func TestShareContentCapturesIntoTheTarget(t *testing.T) {
	dir := t.TempDir()
	s := &Server{
		thingStore:  &ThingStore{path: filepath.Join(dir, "memo.yaml")},
		thingLabels: &ThingLabelStore{mwlabels.NewFileStore(filepath.Join(dir, "memo-labels.yaml"))},
	}
	savedTargets := captureTargets
	captureTargets = &captureTargetStore{path: filepath.Join(dir, "capture-target.yaml")}
	defer func() { captureTargets = savedTargets }()
	saved := sharedContentClient
	defer func() { sharedContentClient = saved }()
	sharedContentClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("reached %s", r.URL)
	})}

	share := func(track ...string) map[string]json.RawMessage {
		url := "https://open.spotify.com/track/x"
		if len(track) > 0 {
			url = "https://open.spotify.com/track/" + track[0]
		}
		rec := httptest.NewRecorder()
		s.shareContent(rec, httptest.NewRequest(http.MethodPost, "/api/v1/things/share",
			strings.NewReader(`{"urls":["`+url+`"],"capture":true}`)))
		if rec.Code != http.StatusOK {
			t.Fatalf("HTTP %d: %s", rec.Code, rec.Body)
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	// No target: made and pinned, and said that there was nowhere to capture.
	got := share()
	if _, ok := got["captureError"]; !ok {
		t.Errorf("no target: want captureError, got %s", got)
	}

	// Aimed through the API, as the board does.
	rec := httptest.NewRecorder()
	s.putCaptureTarget(rec, httptest.NewRequest(http.MethodPut, "/api/v1/capture-target",
		strings.NewReader(`{"target":{"id":"want-1/result/a","constellation":"SmartGolf 10.11","label":"SmartGolf"}}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT HTTP %d: %s", rec.Code, rec.Body)
	}
	if tg := captureTargets.Get(); tg == nil || tg.Constellation != "SmartGolf 10.11" || tg.Kind != "thing" {
		t.Fatalf("stored target = %+v", tg)
	}

	got = share()
	var things []sharedThingResult
	_ = json.Unmarshal(got["things"], &things)
	if len(things) != 1 || !things[0].Captured || !things[0].Pinned {
		t.Fatalf("got %+v", things)
	}
	if v := s.thingLabels.Get(things[0].ID)[constellationKey("SmartGolf 10.11")]; v != "true" {
		t.Errorf("membership label = %q", v)
	}

	// Aimed with no name, as the board aims now: the first capture is asked
	// about, named then; once the tile has its constellation, the next go
	// straight in.
	rec = httptest.NewRecorder()
	s.putCaptureTarget(rec, httptest.NewRequest(http.MethodPut, "/api/v1/capture-target",
		strings.NewReader(`{"target":{"id":"want-1/result/b","constellation":"","label":"10/12 08:00 北新宿店"}}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT unnamed HTTP %d: %s", rec.Code, rec.Body)
	}
	asked := share("y")
	_ = json.Unmarshal(asked["things"], &things)
	first := things[0].ID
	var waiting []captureProposal
	_ = json.Unmarshal(asked["proposals"], &waiting)
	if len(waiting) != 1 || waiting[0].Thing != first {
		t.Errorf("reply's proposals = %s", asked["proposals"])
	}
	props := s.captureProposals()
	if len(props) != 1 || props[0].Thing != first || props[0].Answer != "want-1/result/b" || props[0].Constellation != "10.12 08:00 北新宿店" || props[0].Want != "want-1" {
		t.Fatalf("proposals = %+v", props)
	}
	rec = httptest.NewRecorder()
	req := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/api/v1/capture/proposals/"+first,
		strings.NewReader(`{"accept":true,"constellation":"朝のラウンド"}`)), map[string]string{"thing": first})
	s.answerCaptureProposal(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("answer HTTP %d: %s", rec.Code, rec.Body)
	}
	for _, id := range []string{first, "want-1/result/b"} {
		if v := s.thingLabels.Get(id)[constellationKey("朝のラウンド")]; v != "true" {
			t.Errorf("%s not joined: %q", id, v)
		}
	}
	_ = json.Unmarshal(share("z")["things"], &things)
	if things[0].ID == first {
		t.Fatal("the second capture is the same thing")
	}
	if len(s.captureProposals()) != 0 {
		t.Errorf("asked again once named: %+v", s.captureProposals())
	}
	if v := s.thingLabels.Get(things[0].ID)[constellationKey("朝のラウンド")]; v != "true" {
		t.Errorf("second capture not joined: %q", v)
	}

	// A sender that asks itself: the proposal says until when, so the board
	// waits.
	rec = httptest.NewRecorder()
	s.putCaptureTarget(rec, httptest.NewRequest(http.MethodPut, "/api/v1/capture-target",
		strings.NewReader(`{"target":{"id":"want-1/result/c","constellation":"","label":"10/13"}}`)))
	rec = httptest.NewRecorder()
	s.shareContent(rec, httptest.NewRequest(http.MethodPost, "/api/v1/things/share",
		strings.NewReader(`{"urls":["https://open.spotify.com/track/w"],"capture":true,"ask":true}`)))
	if props := s.captureProposals(); len(props) != 1 || props[0].AskedUntil == "" {
		t.Errorf("asked by the sender: proposals = %+v", props)
	}

	// A thing on the board already, caught: the same as one just shared — the
	// target named in no constellation yet, its tile is proposed.
	rec = httptest.NewRecorder()
	s.putCaptureTarget(rec, httptest.NewRequest(http.MethodPut, "/api/v1/capture-target",
		strings.NewReader(`{"target":{"id":"want-1/result/d","constellation":"","label":"10/14"}}`)))
	_ = json.Unmarshal(share("v")["things"], &things)
	old := things[0].ID
	_ = s.thingLabels.Remove(old, captureProposalLabel)
	rec = httptest.NewRecorder()
	s.captureExistingThing(rec, mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/api/v1/capture/things/"+old, strings.NewReader(`{}`)), map[string]string{"thing": old}))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"proposal"`) {
		t.Errorf("existing thing: HTTP %d %s", rec.Code, rec.Body)
	}

	// Waited for, by thing: answered at once when one is there, empty when
	// the wait is up for a thing with none.
	rec = httptest.NewRecorder()
	s.getCaptureProposals(rec, httptest.NewRequest(http.MethodGet, "/api/v1/capture/proposals?things=nope&wait=1", nil))
	if !strings.Contains(rec.Body.String(), `"proposals":[]`) {
		t.Errorf("waited for none: %s", rec.Body)
	}

	// Put down: gone.
	rec = httptest.NewRecorder()
	s.putCaptureTarget(rec, httptest.NewRequest(http.MethodPut, "/api/v1/capture-target", strings.NewReader(`{"target":null}`)))
	if captureTargets.Get() != nil {
		t.Error("target still set after null")
	}
}
