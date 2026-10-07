package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A type's labels and parameters, not this file, decide how deploy_want's
// params are fitted and what the robot is told.
func TestFMFitParamsByWhatEachParameterIs(t *testing.T) {
	dir := t.TempDir()
	yaml := `wantType:
  metadata:
    name: fit_probe
    title: Fit probe
    description: A probe.
    version: '1.0'
    category: utility
    pattern: independent
    labels:
      robot-hint: Say it plainly.
      "robot-qa/probe at nine": deploy_want fit_probe with params at=09:00
  parameters:
    - name: at
      type: string
      subType: time
      required: false
      default: ""
    - name: how
      type: string
      required: false
      default: ""
      validation:
        enum: [到着, 出発]
  state: []
`
	if err := os.WriteFile(filepath.Join(dir, "fit_probe.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(Config{Port: 0, Host: "localhost", Debug: true, WantTypesDir: dir})
	s.setupRoutes()

	params := map[string]any{"at": "9時40分", "how": "到着"}
	if _, err := s.fmFitParams("fit_probe", params); err != nil || params["at"] != "09:40" {
		t.Fatalf("fit = %v, %v", params, err)
	}
	if _, err := s.fmFitParams("fit_probe", map[string]any{"how": "着"}); err == nil || !strings.Contains(err.Error(), "到着, 出発") {
		t.Errorf("a value outside the choices: %v", err)
	}
	if h := s.fmHintsText(); !strings.Contains(h, "- fit_probe: Say it plainly.") || !strings.Contains(h, "「probe at nine」 → deploy_want fit_probe") {
		t.Errorf("hints = %q", h)
	}
	if d, _ := s.fmDescribeType(map[string]string{"type": "fit_probe"}); !strings.Contains(d, "How: Say it plainly.") {
		t.Errorf("describe = %q", d)
	}
}
