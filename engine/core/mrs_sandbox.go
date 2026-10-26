package mywant

// mrs_sandbox.go — MRS scripts run in the runner, a container of their own.
//
// A plugin's main.py and a type's voice.py are other people's Python. Run by
// the engine they could read state.yaml and the secrets, write anywhere and
// reach anything. The runner (`mywant runner serve`, mrs_agent_service.py
// --runner) forks each call and shuts it in before the script is read — see
// mywant-ideas/impl-sandbox.md for what each profile sees.
//
// Where the runner is comes from MYWANT_RUNNER, else from ~/.mywant/runner.json
// (written by `mywant runner start`). With a runner every call goes there, and
// a call that does not get through is NOT run here instead: that would be the
// way round the sandbox. Without one, MYWANT_SANDBOX=required refuses; anything
// else runs the script as before, and says so once.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Profiles a runner call is shut in with.
const (
	MRSProfileMain  = "main"  // a plugin's script: the network, plugin-data/<plugin>
	MRSProfileVoice = "voice" // a voice script: its own directory, nothing else
)

// MRSRunnerFile is where `mywant runner start` leaves the runner's address.
const MRSRunnerFile = "runner.json"

// MRSRunnerConfig is the content of MRSRunnerFile.
type MRSRunnerConfig struct {
	URL string `json:"url"`
}

// mrsRunnerDefaultTimeout is sent when the caller set no deadline: the runner
// kills a child that runs longer.
const mrsRunnerDefaultTimeout = 120 * time.Second

// MRSRunnerURL is the runner to send scripts to, or "" for none.
// MYWANT_RUNNER=off says none whatever runner.json says (tests use it).
func MRSRunnerURL() string {
	if v := strings.TrimSpace(os.Getenv("MYWANT_RUNNER")); v != "" {
		if strings.EqualFold(v, "off") {
			return ""
		}
		return strings.TrimRight(v, "/")
	}
	data, err := os.ReadFile(filepath.Join(MyWantHomeDir(), MRSRunnerFile))
	if err != nil {
		return ""
	}
	var c MRSRunnerConfig
	if json.Unmarshal(data, &c) != nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(c.URL), "/")
}

func mrsSandboxRequired() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MYWANT_SANDBOX")), "required")
}

var errMRSNoRunner = errors.New("MYWANT_SANDBOX=required and no runner is configured (mywant runner start, or MYWANT_RUNNER)")

var mrsUnsandboxedOnce sync.Once

func mrsWarnUnsandboxed() {
	mrsUnsandboxedOnce.Do(func() {
		InfoLog("[MRS] WARN: no runner configured; plugin scripts run unsandboxed in the engine's own environment (mywant runner start)")
	})
}

// mrsRunnerEnv is what a main call is given of the engine's environment: the
// config's environments (the API keys plugins are configured with) as they are
// in force now. The rest of the engine's environment stays with the engine.
func mrsRunnerEnv() map[string]string {
	keys, err := readConfigEnvironments(configEnvPath())
	if err != nil {
		return nil
	}
	env := make(map[string]string, len(keys))
	for k, v := range keys {
		if cur := os.Getenv(k); cur != "" {
			v = cur // exported in the shell, as for a spawn
		}
		if v != "" {
			env[k] = v
		}
	}
	return env
}

// runMRSRunner runs one call on the runner at base.
func runMRSRunner(ctx context.Context, base, scriptPath string, opt MRSRunOptions) (map[string]any, error) {
	profile := opt.Profile
	if profile == "" {
		profile = MRSProfileMain
	}
	timeout := mrsRunnerDefaultTimeout
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
	}
	params := map[string]any{
		"script":  scriptPath,
		"args":    opt.Args,
		"profile": profile,
		"timeout": timeout.Seconds(),
	}
	if profile == MRSProfileMain {
		params["env"] = mrsRunnerEnv()
	}
	if opt.Context != nil {
		// The want this call runs for, for the script's SDK — without this
		// server's own address, which is this machine's and not the runner's:
		// there the SDK reaches the server by the MYWANT_URL the runner sets.
		c := make(map[string]any, len(opt.Context))
		for k, v := range opt.Context {
			c[k] = v
		}
		delete(c, "server")
		params["context"] = c
	}
	result, err := postMRSExecute(ctx, base, filepath.Base(filepath.Dir(scriptPath)), params, opt)
	if errors.Is(err, errMRSUnreachable) {
		return nil, runnerNotAnswering(base, err)
	}
	return result, err
}

// runnerNotAnswering says so, and what brings plugin scripts back: the runner
// is the user's to start (Docker Desktop, then mywant runner start), or to
// give up for the engine's own unsandboxed run (mywant runner stop). The
// want the call was for shows it as its error.
func runnerNotAnswering(base string, err error) error {
	return fmt.Errorf("runner at %s is not answering — start Docker Desktop, then run `mywant runner start` "+
		"(or `mywant runner stop` to run plugin scripts in the engine, unsandboxed): %w", base, err)
}

// RunnerOptions is how `mywant runner serve` starts the runner.
type RunnerOptions struct {
	Host string
	Port int
	// NoJail runs each call forked but not shut in — for trying the runner on
	// a machine without Linux namespaces. Never in a deployment.
	NoJail bool
	// Expose names directories a main call may also read (a development
	// checkout a plugin looks for, like ~/work/machine-readable-skills-browser).
	Expose []string
	// APIURL is MYWANT_URL as seen from inside the runner's container.
	APIURL string
}

// RunnerArgs is the service's command line for o, after the script's path.
func (o RunnerOptions) RunnerArgs() []string {
	args := []string{"--runner", "--host", o.Host, "--port", fmt.Sprint(o.Port)}
	if o.NoJail {
		args = append(args, "--no-jail")
	}
	for _, p := range o.Expose {
		args = append(args, "--expose", p)
	}
	if o.APIURL != "" {
		args = append(args, "--api-url", o.APIURL)
	}
	return args
}

// ServeRunner becomes the runner: the embedded service, exec'd in this
// process's place so no Go process stays resident beside it.
func ServeRunner(o RunnerOptions) error {
	script, err := mrsSharedScriptPath()
	if err != nil {
		return err
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		return err
	}
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "MYWANT_MRS_SERVE=") {
			env = append(env, kv)
		}
	}
	// The SDK on the path from the start: a call's own environment is shut
	// down to a few variables, but the interpreter's path is set by then.
	return syscall.Exec(python, append([]string{"python3", script}, o.RunnerArgs()...), mrsPythonEnv(env))
}
