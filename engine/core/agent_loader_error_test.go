package mywant

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A plugin's do agent whose script cannot run — here, a runner that does not
// answer — says so in the want's error, and takes it back once a run goes
// through.
func TestMRSPluginDo_ErrorShownThenCleared(t *testing.T) {
	mrsSandboxHome(t)
	t.Setenv("MYWANT_SANDBOX", "")
	t.Setenv("MYWANT_MRS_SHARED", "0")
	dir := t.TempDir()
	script := filepath.Join(dir, "main.py")
	if err := os.WriteFile(script, []byte(`print('{"answer": "ok"}')`), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "agent:\n  metadata:\n    name: err_test_agent\n    capability: err_test\n    type: do\n  script:\n    path: ./main.py\n    timeout_seconds: 10\n"
	r := NewAgentRegistry()
	if err := r.RegisterMRSAgentFromYAML([]byte(yaml), filepath.Join(dir, "agent.yaml")); err != nil {
		t.Fatal(err)
	}
	a, ok := r.GetAgent("err_test_agent")
	if !ok {
		t.Fatal("agent not registered")
	}
	do := a.(*DoAgent)
	want := inlineTestWant()
	want.SetStateLabels(&WantTypeDefinition{}) // every want gets its error field, as the engine gives it

	t.Setenv("MYWANT_RUNNER", "http://127.0.0.1:1")
	if err := do.Action(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	if got := GetCurrent(want, StateFieldError, ""); !strings.Contains(got, "not answering") {
		t.Errorf("error after a runner that does not answer = %q", got)
	}

	t.Setenv("MYWANT_RUNNER", "off")
	if err := do.Action(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	if got := GetCurrent(want, StateFieldError, ""); got != "" {
		t.Errorf("error after a run that went through = %q", got)
	}
}
