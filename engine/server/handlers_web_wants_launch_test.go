package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// The want card's "open in a new tab" asks for fill_only: the claim handed to
// the extension must carry it, so the page is filled but nothing is pressed.
func TestLaunchWebWantFillOnlyReachesTheClaim(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".mywant", "custom-types", "example_web")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	elems := `{"example.com":[{"role":"textbox","name":"Search","selector":"input[name=q]","field_key":"search"}]}`
	if err := os.WriteFile(filepath.Join(dir, "elements.json"), []byte(elems), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, ok := navLaunchQueue.dequeue(); ok; _, ok = navLaunchQueue.dequeue() {
	}

	for _, fillOnly := range []bool{true, false} {
		body := `{"target_url":"https://example.com/","field_values":{"search":"cats"}`
		if fillOnly {
			body += `,"fill_only":true`
		}
		body += `}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/web-wants/example_web/launch", strings.NewReader(body))
		req = mux.SetURLVars(req, map[string]string{"name": "example_web"})
		rec := httptest.NewRecorder()
		(&Server{}).launchWebWant(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("fill_only=%v: status %d: %s", fillOnly, rec.Code, rec.Body.String())
		}
		claim, ok := navLaunchQueue.dequeue()
		if !ok {
			t.Fatalf("fill_only=%v: no claim queued", fillOnly)
		}
		if claim.FillOnly != fillOnly || claim.FieldValues["search"] != "cats" {
			t.Fatalf("fill_only=%v: claim = %+v", fillOnly, claim)
		}
	}
}
