package mywant

import (
	"errors"
	"path/filepath"
	"testing"
)

func inlineTestWant() *Want {
	w := &Want{Metadata: Metadata{Name: "w", ID: "w1", Type: "demo_type"}}
	w.StateLabels = map[string]StateLabel{"count": LabelCurrent, "env": LabelCurrent}
	w.SetCurrent("count", 1)
	return w
}

// The same inline scripts, here and in the runner: they read the want's
// state, print what changes, and leave files to be read back. In the runner
// they get the config's environments and plugin-data/<want type>, not the
// engine's own environment.
func TestInlineScript_HereAndInTheRunner(t *testing.T) {
	mrsNeedPython(t)
	for _, where := range []string{"here", "runner"} {
		t.Run(where, func(t *testing.T) {
			home := mrsSandboxHome(t)
			t.Setenv("ENGINE_ONLY_SECRET", "s3cret")
			t.Setenv("MYWANT_SANDBOX", "")
			if where == "runner" {
				mrsStartRunner(t, RunnerOptions{Host: "127.0.0.1", NoJail: true})
			} else {
				t.Setenv("MYWANT_RUNNER", "off")
			}

			want := inlineTestWant()
			py := `import json, os
current = json.load(open(os.environ["MYWANT_CURRENT_FILE"]))
print(json.dumps({"current_updates": {"count": current["count"] + 1}}))`
			if err := (&pythonRuntime{}).ExecuteDo(want, py); err != nil {
				t.Fatal(err)
			}
			if got := GetCurrent(want, "count", 0); got != 2 {
				t.Errorf("python: count = %v", got)
			}

			sh := `echo "{\"current_updates\": {\"env\": \"${PLUGIN_KEY:-}/${ENGINE_ONLY_SECRET:-}/${MYWANT_PLUGIN_DATA:-}\"}}"`
			if err := (&shellRuntime{}).ExecuteDo(want, sh); err != nil {
				t.Fatal(err)
			}
			wantEnv := "from-config//" + filepath.Join(home, ".mywant", "plugin-data", "demo_type")
			if where == "here" {
				wantEnv = "/s3cret/"
			}
			if got := GetCurrent(want, "env", ""); got != wantEnv {
				t.Errorf("shell: env = %q, want %q", got, wantEnv)
			}

			res, err := runInline(inlineRun{
				Want:    want,
				Argv:    []string{"/bin/sh", "-c", `cat "$IN" > "$OUT"; echo oops >&2; exit 3`},
				Files:   map[string][]byte{"in.txt": []byte("kept")},
				FileEnv: map[string]string{"IN": "in.txt", "OUT": "out.txt"},
				Outputs: []string{"out.txt", "never.txt"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if res.ExitCode != 3 || string(res.Stderr) != "oops\n" || string(res.Outputs["out.txt"]) != "kept" {
				t.Errorf("result = %+v", res)
			}
			if _, ok := res.Outputs["never.txt"]; ok {
				t.Error("a file never written came back")
			}

			if err := (&shellRuntime{}).ExecuteDo(want, "exit 1"); err == nil {
				t.Error("a failing script did not fail")
			}
		})
	}
}

func TestInlineScript_RequiredWithoutRunnerRefuses(t *testing.T) {
	mrsSandboxHome(t)
	t.Setenv("MYWANT_RUNNER", "off")
	t.Setenv("MYWANT_SANDBOX", "required")
	if _, err := runInline(inlineRun{Want: inlineTestWant(), Argv: []string{"true"}}); !errors.Is(err, errMRSNoRunner) {
		t.Errorf("err = %v", err)
	}
}
