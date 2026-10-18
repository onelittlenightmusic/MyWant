package mywant

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// mrsFakeRunner answers every call with result and keeps what it was sent.
func mrsFakeRunner(t *testing.T, result map[string]any) (*httptest.Server, *[]ExecuteRequest) {
	t.Helper()
	var got []ExecuteRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ExecuteRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		got = append(got, req)
		enc := json.NewEncoder(w)
		_ = enc.Encode(map[string]any{"_progress": 40, "_message": "in the runner"})
		_ = enc.Encode(map[string]any{"status": "completed", "state_updates": result})
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

// mrsSandboxHome gives the test a ~/.mywant of its own, with a config whose
// environments a main call is handed.
func mrsSandboxHome(t *testing.T) string {
	t.Helper()
	return mrsSandboxHomeIn(t, t.TempDir())
}

func mrsSandboxHomeIn(t *testing.T, home string) string {
	t.Helper()
	t.Setenv("HOME", home)
	SetConfigEnvPath("")
	if err := os.MkdirAll(filepath.Join(home, ".mywant"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "environments:\n  PLUGIN_KEY: from-config\n"
	if err := os.WriteFile(filepath.Join(home, ".mywant", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// With a runner every call goes there, as the profile it was asked for, with
// the config's environments for a main call and none for a voice.
func TestMRSRunner_CallsGoToTheRunner(t *testing.T) {
	mrsSandboxHome(t)
	srv, got := mrsFakeRunner(t, map[string]any{"ok": true})
	t.Setenv("MYWANT_RUNNER", srv.URL)
	t.Setenv("PLUGIN_KEY", "")

	var progress []string
	res, err := mrsRun(t, "/plugins/transit/main.py", MRSRunOptions{
		Args:       []string{"{}"},
		Isolated:   true, // means nothing in the runner
		OnProgress: func(p int, m string) { progress = append(progress, fmt.Sprintf("%d %s", p, m)) },
	})
	if err != nil || res["ok"] != true {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if len(progress) != 1 || progress[0] != "40 in the runner" {
		t.Errorf("progress = %v", progress)
	}
	if _, err := mrsRun(t, "/cache/voice/abc/checklist.voice.py", MRSRunOptions{Profile: MRSProfileVoice}); err != nil {
		t.Fatal(err)
	}

	if len(*got) != 2 {
		t.Fatalf("runner got %d calls", len(*got))
	}
	main, voice := (*got)[0].Params, (*got)[1].Params
	if main["profile"] != "main" || main["script"] != "/plugins/transit/main.py" || (*got)[0].AgentName != "transit" {
		t.Errorf("main call = %v", (*got)[0])
	}
	if env, _ := main["env"].(map[string]any); env["PLUGIN_KEY"] != "from-config" {
		t.Errorf("main env = %v", main["env"])
	}
	if secs, _ := main["timeout"].(float64); secs <= 0 || secs > 20 {
		t.Errorf("timeout = %v, want the caller's deadline", main["timeout"])
	}
	if voice["profile"] != "voice" || voice["env"] != nil {
		t.Errorf("voice call = %v", voice)
	}
}

// A runner that does not answer is an error: the script is not run here
// instead, which would be the way round the sandbox.
func TestMRSRunner_UnreachableIsNotRunHere(t *testing.T) {
	mrsNeedPython(t)
	home := mrsSandboxHome(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // a port nothing listens on
	t.Setenv("MYWANT_RUNNER", srv.URL)

	marker := filepath.Join(home, "ran")
	script := mrsTestScript(t, filepath.Join(home, "plugin"), fmt.Sprintf(
		"import json\nopen(%q, 'w').write('x')\nprint(json.dumps({}))\n", marker))
	_, err := mrsRun(t, script, MRSRunOptions{})
	if err == nil || !strings.Contains(err.Error(), "runner at") {
		t.Errorf("err = %v", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("the script ran in the engine")
	}
}

// runner.json, left by `mywant runner start`, is read when MYWANT_RUNNER is
// not set; MYWANT_RUNNER=off ignores it.
func TestMRSRunnerURL(t *testing.T) {
	home := mrsSandboxHome(t)
	t.Setenv("MYWANT_RUNNER", "")
	if got := MRSRunnerURL(); got != "" {
		t.Errorf("none yet: %q", got)
	}
	file := filepath.Join(home, ".mywant", MRSRunnerFile)
	if err := os.WriteFile(file, []byte(`{"url": "http://127.0.0.1:8090/"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := MRSRunnerURL(); got != "http://127.0.0.1:8090" {
		t.Errorf("from runner.json: %q", got)
	}
	t.Setenv("MYWANT_RUNNER", "http://runner:9000")
	if got := MRSRunnerURL(); got != "http://runner:9000" {
		t.Errorf("env first: %q", got)
	}
	t.Setenv("MYWANT_RUNNER", "off")
	if got := MRSRunnerURL(); got != "" {
		t.Errorf("off: %q", got)
	}
}

func TestMRSRunner_RequiredWithoutOneRefuses(t *testing.T) {
	mrsSandboxHome(t)
	t.Setenv("MYWANT_RUNNER", "off")
	t.Setenv("MYWANT_SANDBOX", "required")
	if _, err := mrsRun(t, "/plugins/x/main.py", MRSRunOptions{}); !errors.Is(err, errMRSNoRunner) {
		t.Errorf("err = %v", err)
	}
}

// mrsStartRunner runs the real service as a runner and points the engine at it.
func mrsStartRunner(t *testing.T, o RunnerOptions) {
	t.Helper()
	script, err := mrsSharedScriptPath()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", append([]string{script}, o.RunnerArgs()...)...)
	cmd.Stderr = os.Stderr
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, _ := bufio.NewReader(out).ReadBytes('\n')
	var hello struct{ Port int }
	if json.Unmarshal(line, &hello) != nil || hello.Port == 0 {
		t.Fatalf("runner said %q", line)
	}
	go func() { _, _ = io.Copy(io.Discard, out) }()
	t.Setenv("MYWANT_RUNNER", fmt.Sprintf("http://127.0.0.1:%d", hello.Port))
}

// The runner without its jail, which runs anywhere: each call a fork of its
// own, with the scrubbed environment, plugin-data, a timeout that kills.
func TestMRSRunner_RealRunnerWithoutJail(t *testing.T) {
	mrsNeedPython(t)
	home := mrsSandboxHome(t)
	t.Setenv("ENGINE_ONLY_SECRET", "s3cret")
	mrsStartRunner(t, RunnerOptions{Host: "127.0.0.1", NoJail: true, APIURL: "http://api.inside:8080"})

	script := mrsTestScript(t, filepath.Join(home, ".mywant", "custom-types", "demo"), `import json, os, sys, time
if sys.argv[1] == "slow":
    time.sleep(30)
print(json.dumps({"_progress": 10, "_message": "go"}), flush=True)
print(json.dumps({
    "pid": os.getpid(),
    "key": os.environ.get("PLUGIN_KEY"),
    "secret": os.environ.get("ENGINE_ONLY_SECRET"),
    "url": os.environ.get("MYWANT_URL"),
    "data": os.environ.get("MYWANT_PLUGIN_DATA"),
}))
`)
	var progress int
	a, err := mrsRun(t, script, MRSRunOptions{Args: []string{"x"}, OnProgress: func(p int, _ string) { progress = p }})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := mrsRun(t, script, MRSRunOptions{Args: []string{"x"}})
	if a["pid"] == b["pid"] {
		t.Error("two calls ran in one process")
	}
	if progress != 10 || a["key"] != "from-config" || a["secret"] != nil || a["url"] != "http://api.inside:8080" {
		t.Errorf("progress=%d result=%v", progress, a)
	}
	if want := filepath.Join(home, ".mywant", "plugin-data", "demo"); a["data"] != want {
		t.Errorf("data = %v, want %s", a["data"], want)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := RunMRSScript(ctx, script, MRSRunOptions{Args: []string{"slow"}}); err == nil {
		t.Error("a script past its deadline answered")
	}
	if time.Since(start) > 6*time.Second {
		t.Errorf("the timeout took %v", time.Since(start))
	}
}

// The jail itself: Linux, as root (the runner's container). Run with
// `sudo -E go test ./core -run Jail` on a Linux machine.
func TestMRSRunner_Jail(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("the jail needs Linux and root")
	}
	mrsNeedPython(t)
	// Not under /tmp, which every call gets a fresh one of.
	base, err := os.MkdirTemp("/var/lib", "mywant-jail-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	home := mrsSandboxHomeIn(t, base)
	// The jail runs a call as whoever owns ~/.mywant: here, not root.
	if err := os.Chown(filepath.Join(home, ".mywant"), 1000, 1000); err != nil {
		t.Fatal(err)
	}
	mrsStartRunner(t, RunnerOptions{Host: "127.0.0.1"})

	mywant := filepath.Join(home, ".mywant")
	for _, f := range []string{"state.yaml", "secrets/token.json"} {
		p := filepath.Join(mywant, f)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("secret"), 0o644)
	}
	probe := `import json, os, socket, sys
def can_write(p):
    try:
        open(p, "w").write("x"); return True
    except OSError:
        return False
def can_read(p):
    try:
        open(p).read(); return True
    except OSError:
        return False
def can_connect():
    try:
        socket.create_connection(("1.1.1.1", 53), 2).close(); return True
    except OSError:
        return False
m = os.path.join(os.environ["HOME"], ".mywant")
print(json.dumps({
    "state": can_read(os.path.join(m, "state.yaml")),
    "secrets": can_read(os.path.join(m, "secrets", "token.json")),
    "write_state": can_write(os.path.join(m, "state.yaml")),
    "write_plugin": can_write(os.path.join(os.path.dirname(os.path.abspath(sys.argv[0])), "x")),
    "write_data": can_write(os.path.join(os.environ.get("MYWANT_PLUGIN_DATA") or "/nonexistent", "x")),
    "write_root": can_write("/usr/x"),
    "net": can_connect(),
    "uid": os.getuid(),
}))
`
	script := mrsTestScript(t, filepath.Join(mywant, "custom-types", "probe"), probe)
	main, err := mrsRun(t, script, MRSRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]bool{"state": false, "secrets": false, "write_state": false,
		"write_plugin": false, "write_data": true, "write_root": false} {
		if main[k] != want {
			t.Errorf("main %s = %v, want %v", k, main[k], want)
		}
	}
	if main["uid"] != float64(1000) {
		t.Errorf("main ran as uid %v, want ~/.mywant's owner", main["uid"])
	}

	voice, err := mrsRun(t, script, MRSRunOptions{Profile: MRSProfileVoice})
	if err != nil {
		t.Fatal(err)
	}
	if voice["net"] != false || voice["write_data"] != false || voice["state"] != false {
		t.Errorf("voice = %v", voice)
	}
	if _, err := os.Stat(filepath.Join(mywant, "state.yaml")); err != nil {
		t.Error(err)
	}

	// A ~/.mywant that shows as root's (a Mac's, through Docker Desktop) is
	// not a reason to run as root.
	if err := os.Chown(mywant, 0, 0); err != nil {
		t.Fatal(err)
	}
	asRoot, err := mrsRun(t, script, MRSRunOptions{Profile: MRSProfileVoice})
	if err != nil {
		t.Fatal(err)
	}
	if asRoot["uid"] != float64(65534) {
		t.Errorf("with a root-owned ~/.mywant the call ran as uid %v, want nobody", asRoot["uid"])
	}
}
