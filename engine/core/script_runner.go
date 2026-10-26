package mywant

// script_runner.go — where an inline script (a want type's `script:` or
// `scriptFile:`, any runtime) is run.
//
// An inline script is as much someone else's code as a plugin's main.py, so it
// goes the same way (mrs_sandbox.go): to the runner when there is one, shut in
// with profile main — the network, config.yaml's environments, plugin-data/<want
// type> to keep files in, nothing of state.yaml or the secrets. It is never run
// here instead when the runner does not answer; without a runner,
// MYWANT_SANDBOX=required refuses, and anything else runs it here as before.
//
// Each runtime says what to run as an inlineRun: a command line, the files it
// is given (the script, goal.json…), the variables that point at them, the
// files it leaves to be read back. A command-line argument that names one of
// the files is that file wherever the call runs.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// inlineScriptTimeout is how long an inline script may run unless its runtime
// says otherwise.
const inlineScriptTimeout = 120 * time.Second

type inlineRun struct {
	Want    *Want
	Argv    []string
	Files   map[string][]byte // written beside each other in a directory of the call's own
	FileEnv map[string]string // variable → one of Files (MYWANT_GOAL_FILE → goal.json)
	Env     map[string]string
	Outputs []string // files the command writes there, read back
	Timeout time.Duration
}

type inlineResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	Outputs  map[string][]byte // only those that were written
}

// runInline runs r and returns how it ended. A command that ran and failed is
// not an error: its exit code and stderr are in the result.
func runInline(r inlineRun) (*inlineResult, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = inlineScriptTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if base := MRSRunnerURL(); base != "" {
		return runInlineRunner(ctx, base, r)
	}
	if mrsSandboxRequired() {
		return nil, errMRSNoRunner
	}
	mrsWarnUnsandboxed()
	return runInlineHere(ctx, r)
}

// withStateFiles adds the want's goal/current/plan, as writeStateFiles writes
// them, to r's files and points the MYWANT_*_FILE variables at them.
func (r *inlineRun) withStateFiles() error {
	files, err := stateFiles(r.Want)
	if err != nil {
		return err
	}
	if r.Files == nil {
		r.Files = map[string][]byte{}
	}
	if r.FileEnv == nil {
		r.FileEnv = map[string]string{}
	}
	for name, data := range files {
		r.Files[name] = data
	}
	for k, name := range stateFileEnv {
		r.FileEnv[k] = name
	}
	return nil
}

func runInlineHere(ctx context.Context, r inlineRun) (*inlineResult, error) {
	dir, err := os.MkdirTemp("", "inline-script-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)
	for name, data := range r.Files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", name, err)
		}
	}
	argv := make([]string, len(r.Argv))
	for i, a := range r.Argv {
		if _, ok := r.Files[a]; ok {
			a = filepath.Join(dir, a)
		}
		argv[i] = a
	}
	env := os.Environ()
	for k, v := range r.Env {
		env = append(env, k+"="+v)
	}
	for k, name := range r.FileEnv {
		env = append(env, k+"="+filepath.Join(dir, name))
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = env
	var stdout, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	res := &inlineResult{}
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, callEnded("script", ctx)
		}
		res.ExitCode = exit.ExitCode()
	}
	res.Stdout, res.Stderr = stdout.Bytes(), stderr.Bytes()
	res.Outputs = map[string][]byte{}
	for _, name := range r.Outputs {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			res.Outputs[name] = data
		}
	}
	return res, nil
}

func runInlineRunner(ctx context.Context, base string, r inlineRun) (*inlineResult, error) {
	typeName := "inline"
	if r.Want != nil && r.Want.Metadata.Type != "" {
		typeName = r.Want.Metadata.Type
	}
	files := make(map[string]string, len(r.Files))
	for name, data := range r.Files {
		files[name] = string(data)
	}
	env := mrsRunnerEnv()
	if env == nil {
		env = map[string]string{}
	}
	for k, v := range r.Env {
		env[k] = v
	}
	timeout := mrsRunnerDefaultTimeout
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
	}
	params := map[string]any{
		"command":  r.Argv,
		"files":    files,
		"file_env": r.FileEnv,
		"outputs":  r.Outputs,
		"env":      env,
		"plugin":   typeName, // its plugin-data directory
		"profile":  MRSProfileMain,
		"timeout":  timeout.Seconds(),
	}
	result, err := postMRSExecute(ctx, base, "inline:"+typeName, params, MRSRunOptions{})
	if errors.Is(err, errMRSUnreachable) {
		return nil, runnerNotAnswering(base, err)
	}
	if err != nil {
		return nil, err
	}
	res := &inlineResult{
		ExitCode: int(mrsFloat(result["exit_code"])),
		Stdout:   []byte(mrsStr(result["stdout"])),
		Stderr:   []byte(mrsStr(result["stderr"])),
		Outputs:  map[string][]byte{},
	}
	if outs, ok := result["outputs"].(map[string]any); ok {
		for name, v := range outs {
			if s, ok := v.(string); ok {
				res.Outputs[name] = []byte(s)
			}
		}
	}
	return res, nil
}

// limitedBuffer keeps the first 8MB a command writes, so a script that loops
// printing cannot take the engine's memory with it.
type limitedBuffer struct{ buf []byte }

const limitedBufferMax = 8 << 20

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := limitedBufferMax - len(b.buf); room > 0 {
		if len(p) > room {
			b.buf = append(b.buf, p[:room]...)
		} else {
			b.buf = append(b.buf, p...)
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte { return b.buf }

// runPrintingScript runs r with the want's state files and applies the JSON
// the script prints — the shell and python runtimes.
func runPrintingScript(r inlineRun, label, what string, apply func(*scriptOutput)) error {
	want := r.Want
	if err := r.withStateFiles(); err != nil {
		return scriptErr(want, label, err.Error())
	}
	want.StoreLog("[%s] executing %s", label, what)
	res, err := runInline(r)
	if err != nil {
		return scriptErr(want, label, err.Error())
	}
	if res.ExitCode != 0 {
		return scriptErr(want, label, fmt.Sprintf("script failed: exit status %d\nstderr:\n%s", res.ExitCode, res.Stderr))
	}
	out, err := parseScriptOutput(res.Stdout)
	if err != nil {
		return scriptErr(want, label, err.Error())
	}
	apply(out)
	return nil
}
