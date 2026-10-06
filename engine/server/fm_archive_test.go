package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fmArchiveTestWant puts a want on the test server and waits for it.
func fmArchiveTestWant(t *testing.T, s *Server, name string, labels map[string]string, params map[string]any) string {
	t.Helper()
	w := patchTestDo(t, s, "POST", "/api/v1/wants", []map[string]any{{
		"metadata": map[string]any{"name": name, "type": "queue", "labels": labels},
		"spec":     map[string]any{"params": params},
	}})
	if w.Code != http.StatusCreated {
		t.Fatalf("create %s: %d %s", name, w.Code, w.Body.String())
	}
	for i := 0; i < 150; i++ {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			if want.Metadata.Name == name {
				return want.Metadata.ID
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", name)
	return ""
}

func fmParams(t *testing.T, v map[string]any) string {
	t.Helper()
	b, _ := json.Marshal(v)
	return string(b)
}

// The robot's 「荻窪から中野坂上の乗り換え」 on Fly, 2026-10-06: it named its new
// want after the type, an archived want already had that name, and the deploy
// failed with a 409 the person never saw the reason for.
func TestFMDeployTakesAnotherNameWhenAnArchivedWantHasIt(t *testing.T) {
	s := patchTestServer(t)
	fmArchiveTestWant(t, s, "transit_search",
		map[string]string{"mywant.io/archived": "true"}, map[string]any{"service_time": 0.1})

	out, err := s.fmDeployWant(map[string]string{
		"type": "queue", "name": "transit_search",
		"params": fmParams(t, map[string]any{"service_time": 0.7}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `Deployed "transit_search-2"`) {
		t.Errorf("out = %s", out)
	}
}

func TestFMArchivedWantIsOfferedBack(t *testing.T) {
	s := patchTestServer(t)
	id := fmArchiveTestWant(t, s, "old-route",
		map[string]string{"mywant.io/archived": "true", "mywant.io/canvas-x": "3", "mywant.io/canvas-y": "4"},
		map[string]any{"service_time": 0.2})

	// The same thing asked again: not made twice, offered back.
	out, err := s.fmDeployWant(map[string]string{
		"type": "queue", "params": fmParams(t, map[string]any{"service_time": 0.2}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "archived") || !strings.Contains(out, `restore_want with name "old-route"`) {
		t.Errorf("deploy out = %s", out)
	}
	for _, w := range s.globalBuilder.GetAllWantStates() {
		if w.Metadata.Name != "old-route" && w.Metadata.Type == "queue" {
			t.Errorf("a second want was made: %s", w.Metadata.Name)
		}
	}

	// Off the board: point has nowhere to walk, and says why.
	out, err = s.fmPoint(map[string]string{"name": "old-route"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "archived") {
		t.Errorf("point out = %s", out)
	}

	// Yes: the archive label comes off.
	args := map[string]string{"name": "old-route"}
	out, err = s.fmRestoreWant(args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `Restored "old-route"`) {
		t.Errorf("restore out = %s", out)
	}
	want, _, _ := s.globalBuilder.FindWantByID(id)
	if want.GetLabels()["mywant.io/archived"] == "true" {
		t.Errorf("still archived: %v", want.GetLabels())
	}
	if _, err := s.fmRestoreWant(map[string]string{"name": "old-route"}); err == nil {
		t.Errorf("restoring a want that is not archived should say so")
	}
}

// The Mac's robot, asked 「新宿から横浜の乗り換え」, sent name "新宿→横浜" and
// params {}: a route with neither end was made, and it said it had searched.
// A required parameter left out now makes nothing, and the model is told which.
func TestFMDeployRefusesMissingRequiredParams(t *testing.T) {
	s := patchTestServer(t)
	_, err := s.fmDeployWant(map[string]string{"type": "dynamic_background", "name": "bg", "params": "{}"})
	if err == nil || !strings.Contains(err.Error(), "character_id") {
		t.Fatalf("err = %v, want it to name character_id", err)
	}
	for _, w := range s.globalBuilder.GetAllWantStates() {
		if w.Metadata.Name == "bg" {
			t.Errorf("a want was made without its required parameter")
		}
	}
}
