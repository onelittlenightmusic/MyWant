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
	if _, err := s.fmFitParams("fit_probe", params, ""); err != nil || params["at"] != "09:40" {
		t.Fatalf("fit = %v, %v", params, err)
	}
	if _, err := s.fmFitParams("fit_probe", map[string]any{"how": "着"}, ""); err == nil || !strings.Contains(err.Error(), "到着, 出発") {
		t.Errorf("a value outside the choices: %v", err)
	}
	if h := s.fmHintsText(); !strings.Contains(h, "- fit_probe: Say it plainly.") || !strings.Contains(h, "「probe at nine」 → deploy_want fit_probe") {
		t.Errorf("hints = %q", h)
	}
	if d, _ := s.fmDescribeType(map[string]string{"type": "fit_probe"}); !strings.Contains(d, "How: Say it plainly.") {
		t.Errorf("describe = %q", d)
	}
}

func TestFMMarkedParams(t *testing.T) {
	labels := map[string]string{fmMarkerLabel + "from": "から,発", fmMarkerLabel + "to": "まで,へ"}
	cases := []struct {
		question string
		in       map[string]any
		from, to any
		said     bool
	}{
		// The model's ends swapped: put back.
		{"銀座から中野坂上の乗り換え", map[string]any{"from": "中野坂上", "to": "銀座"}, "銀座", "中野坂上", true},
		// Already right: left alone, nothing said.
		{"銀座から中野坂上の乗り換え", map[string]any{"from": "銀座", "to": "中野坂上"}, "銀座", "中野坂上", false},
		// The destination's word alone is enough to turn them round.
		{"中野坂上まで、銀座駅発で", map[string]any{"from": "中野坂上", "to": "銀座"}, "銀座", "中野坂上", true},
		// Only the destination sent, under from: moved, from left for here.
		{"戸塚への乗り換え", map[string]any{"from": "戸塚"}, nil, "戸塚", true},
		// A time before まで is not where to: only marked parameters move.
		{"9時40分までに戸塚へ", map[string]any{"to": "戸塚", "time": "9時40分"}, nil, "戸塚", false},
		// Nothing marked: as sent.
		{"新宿と渋谷", map[string]any{"from": "渋谷", "to": "新宿"}, "渋谷", "新宿", false},
	}
	for _, c := range cases {
		said := fmMarkedParams(labels, c.in, c.question)
		if c.in["from"] != c.from || c.in["to"] != c.to || (said != "") != c.said {
			t.Errorf("%s: params = %v, said %q", c.question, c.in, said)
		}
	}
	if got := fmMarkedParams(labels, map[string]any{"from": "中野坂上", "to": "銀座"}, ""); got != "" {
		t.Errorf("no question: %q", got)
	}
}
