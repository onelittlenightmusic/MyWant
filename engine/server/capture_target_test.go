package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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

	share := func() map[string]json.RawMessage {
		rec := httptest.NewRecorder()
		s.shareContent(rec, httptest.NewRequest(http.MethodPost, "/api/v1/things/share",
			strings.NewReader(`{"urls":["https://open.spotify.com/track/x"],"capture":true}`)))
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

	// Put down: gone.
	rec = httptest.NewRecorder()
	s.putCaptureTarget(rec, httptest.NewRequest(http.MethodPut, "/api/v1/capture-target", strings.NewReader(`{"target":null}`)))
	if captureTargets.Get() != nil {
		t.Error("target still set after null")
	}
}
