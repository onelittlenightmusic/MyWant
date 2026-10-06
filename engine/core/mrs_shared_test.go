package mywant

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// mrsTestScript writes a plugin script into its own directory, as plugins live.
func mrsTestScript(t *testing.T, dir, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "main.py")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mrsNeedPython(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	t.Setenv("MYWANT_MRS_SHARED", "1")
	t.Cleanup(mrsShared.stop)
}

func mrsRun(t *testing.T, script string, opt MRSRunOptions) (map[string]any, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return RunMRSScript(ctx, script, opt)
}

const mrsEchoScript = `import json, os, sys

def main():
    print(json.dumps({"_progress": 50, "_message": "half"}), flush=True)
    print(json.dumps({"argv": sys.argv[1:], "pid": os.getpid(), "stdin": sys.stdin.read()}))

if __name__ == "__main__":
    main()
`

func TestMRSShared_RunsAsASpawnWould(t *testing.T) {
	mrsNeedPython(t)
	script := mrsTestScript(t, filepath.Join(t.TempDir(), "echo"), mrsEchoScript)

	var progress []string
	res, err := mrsRun(t, script, MRSRunOptions{
		Args:       []string{`{"a":1}`, "two"},
		OnProgress: func(p int, m string) { progress = append(progress, fmt.Sprintf("%d:%s", p, m)) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(res["argv"]); got != `[{"a":1} two]` {
		t.Errorf("argv = %s", got)
	}
	if res["stdin"] != "" {
		t.Errorf("stdin = %q, want empty as a spawn's /dev/null", res["stdin"])
	}
	if strings.Join(progress, ",") != "50:half" {
		t.Errorf("progress = %v", progress)
	}

	// One interpreter: the next call is served by the same process.
	again, err := mrsRun(t, script, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if again["pid"] != res["pid"] {
		t.Errorf("second call ran in pid %v, first in %v — not the shared service", again["pid"], res["pid"])
	}
	if fmt.Sprint(again["argv"]) != "[]" {
		t.Errorf("argv leaked from the previous call: %v", again["argv"])
	}
}

func TestMRSShared_TwoPluginsShareOneInterpreter(t *testing.T) {
	mrsNeedPython(t)
	root := t.TempDir()
	a := mrsTestScript(t, filepath.Join(root, "a"), mrsEchoScript)
	b := mrsTestScript(t, filepath.Join(root, "b"), mrsEchoScript)
	ra, err := mrsRun(t, a, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rb, err := mrsRun(t, b, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ra["pid"] != rb["pid"] {
		t.Errorf("two plugins ran in pids %v and %v", ra["pid"], rb["pid"])
	}
}

func TestMRSShared_IsolatedGetsItsOwn(t *testing.T) {
	mrsNeedPython(t)
	script := mrsTestScript(t, filepath.Join(t.TempDir(), "echo"), mrsEchoScript)
	shared, err := mrsRun(t, script, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	own, err := mrsRun(t, script, MRSRunOptions{Isolated: true})
	if err != nil {
		t.Fatal(err)
	}
	if shared["pid"] == own["pid"] {
		t.Errorf("isolated call ran in the shared service (pid %v)", own["pid"])
	}
}

func TestMRSShared_ConcurrentCallsKeepTheirOwnArgsAndOutput(t *testing.T) {
	mrsNeedPython(t)
	script := mrsTestScript(t, filepath.Join(t.TempDir(), "slow"), `import json, sys, time

def main():
    mine = sys.argv[1]
    time.sleep(0.3)  # the others run meanwhile
    print(json.dumps({"mine": mine, "argv_after": sys.argv[1]}))

if __name__ == "__main__":
    main()
`)
	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := fmt.Sprint(i)
			res, err := mrsRun(t, script, MRSRunOptions{Args: []string{want}})
			if err != nil {
				errs <- err.Error()
				return
			}
			if res["mine"] != want || res["argv_after"] != want {
				errs <- fmt.Sprintf("call %s got %v", want, res)
			}
		}(i)
	}
	start := time.Now()
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	// In parallel, not one after another.
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("8 calls of 0.3s took %v", d)
	}
}

func TestMRSShared_ExitAndExceptionAreFailures(t *testing.T) {
	mrsNeedPython(t)
	root := t.TempDir()
	exits := mrsTestScript(t, filepath.Join(root, "exits"), `import json, sys

def main():
    print(json.dumps({"error": "no such station"}))
    sys.exit(1)

if __name__ == "__main__":
    main()
`)
	if _, err := mrsRun(t, exits, MRSRunOptions{}); err == nil || err.Error() != "no such station" {
		t.Errorf("sys.exit(1) after an error object: err = %v", err)
	}

	raises := mrsTestScript(t, filepath.Join(root, "raises"), `def main():
    raise ValueError("bad input")

if __name__ == "__main__":
    main()
`)
	_, err := mrsRun(t, raises, MRSRunOptions{})
	if err == nil || !strings.Contains(err.Error(), "ValueError: bad input") {
		t.Errorf("exception: err = %v", err)
	}

	// The service lives on after both.
	ok := mrsTestScript(t, filepath.Join(root, "ok"), mrsEchoScript)
	if _, err := mrsRun(t, ok, MRSRunOptions{}); err != nil {
		t.Errorf("after failures: %v", err)
	}
}

func TestMRSShared_PrettyPrintedResultAndNoMain(t *testing.T) {
	mrsNeedPython(t)
	// No main(): run whole; the result spread over several lines.
	script := mrsTestScript(t, filepath.Join(t.TempDir(), "plain"), `import json, sys
print(json.dumps({"n": len(sys.argv) - 1, "nested": {"x": [1, 2]}}, indent=2))
`)
	res, err := mrsRun(t, script, MRSRunOptions{Args: []string{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(res["n"]) != "2" || fmt.Sprint(res["nested"]) != "map[x:[1 2]]" {
		t.Errorf("result = %v", res)
	}
}

func TestMRSShared_RunsTheMainGuardAsWritten(t *testing.T) {
	mrsNeedPython(t)
	// main takes its arguments (the nearest plugin), and the guard picks a
	// branch by environment (the resident-capable plugins): both as a spawn.
	script := mrsTestScript(t, filepath.Join(t.TempDir(), "guard"), `import json, os, sys

def main(args):
    print(json.dumps({"args": args}))

def serve():
    print(json.dumps({"served": True}))

if __name__ == "__main__":
    if os.environ.get("MYWANT_MRS_SERVE") == "1":
        serve()
    else:
        main(sys.argv[1:])
`)
	t.Setenv("MYWANT_MRS_SERVE", "1") // the service must not hand it on
	res, err := mrsRun(t, script, MRSRunOptions{Args: []string{"x", "y"}})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(res["args"]) != "[x y]" {
		t.Errorf("result = %v", res)
	}
}

func TestMRSShared_TopLevelRunsEveryCall(t *testing.T) {
	mrsNeedPython(t)
	// The nearest plugin's STARTED: taken at the top, the start of this call's
	// time budget — not of the first call's.
	script := mrsTestScript(t, filepath.Join(t.TempDir(), "started"), `import json, time
STARTED = time.time()
COUNT = globals().get("COUNT", 0) + 1

def main():
    print(json.dumps({"started": STARTED, "count": COUNT}))

if __name__ == "__main__":
    main()
`)
	a, err := mrsRun(t, script, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	b, err := mrsRun(t, script, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if a["started"] == b["started"] {
		t.Errorf("STARTED kept from the first call: %v", a["started"])
	}
	if fmt.Sprint(b["count"]) != "1" {
		t.Errorf("globals carried over between calls: count = %v", b["count"])
	}
}

func TestMRSShared_EditedScriptIsReloadedAndSiblingsImport(t *testing.T) {
	mrsNeedPython(t)
	dir := filepath.Join(t.TempDir(), "withhelper")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "helper_for_mrs_test.py"), []byte("WORD = 'beside'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `import json
from helper_for_mrs_test import WORD

def main():
    print(json.dumps({"v": VERSION, "word": WORD}))

VERSION = %d

if __name__ == "__main__":
    main()
`
	script := mrsTestScript(t, dir, fmt.Sprintf(body, 1))
	res, err := mrsRun(t, script, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(res["v"]) != "1" || res["word"] != "beside" {
		t.Fatalf("first = %v", res)
	}
	// A later mtime: the service must load the new file, not keep the old module.
	mrsTestScript(t, dir, fmt.Sprintf(body, 2))
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(script, later, later); err != nil {
		t.Fatal(err)
	}
	res, err = mrsRun(t, script, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(res["v"]) != "2" {
		t.Errorf("after the edit = %v", res)
	}
}

func TestMRSShared_OffSpawnsAsBefore(t *testing.T) {
	mrsNeedPython(t)
	script := mrsTestScript(t, filepath.Join(t.TempDir(), "echo"), mrsEchoScript)
	t.Setenv("MYWANT_MRS_SHARED", "0")
	a, err := mrsRun(t, script, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := mrsRun(t, script, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if a["pid"] == b["pid"] {
		t.Errorf("with the service off, two calls shared pid %v", a["pid"])
	}
}
