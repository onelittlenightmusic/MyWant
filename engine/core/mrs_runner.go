package mywant

// mrs_runner.go — the one place a Machine-Readable Skill script is executed.
//
// There used to be two nearly identical implementations: mrsRunScript (used by
// agent.yaml plugin agents) and runMRSSkillWithArgs (used by the older
// skill_path want types). Both spawned `python3 <script> <args...>` per call
// and read a stream of JSON objects back. They disagreed on the details —
// only one captured stderr, only one forwarded _progress lines — so a plugin's
// failure was legible or not depending on which generation it belonged to.
//
// They are unified here, and the unification is what makes residency possible:
// with a single entry point, `serve: true` in a plugin's agent.yaml turns the
// per-call spawn into a request to a process that is already alive. That
// matters because the spawn, not the work, is the cost — a door observation
// measured 57ms end to end, of which 0.11ms was the HTTP GET it exists to do
// and the rest was starting an interpreter and importing urllib.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MRSRunOptions carries everything that varies between MRS call sites.
type MRSRunOptions struct {
	// Args are the CLI arguments passed to the script (usually a single JSON string).
	Args []string
	// OnProgress receives {"_progress": n, "_message": "..."} lines, if non-nil.
	OnProgress func(pct int, msg string)
	// Serve asks for the resident process rather than a fresh spawn. The runner
	// falls back to spawning if the script turns out not to speak the protocol.
	Serve bool
	// CacheTTL, when > 0, serves a repeat call with identical Args from the
	// previous result. Note this keys on Args: a skill whose result varies per
	// want (rpg_door's door_id) never hits, while an argument-less monitor
	// shared by many wants always does.
	CacheTTL time.Duration
	// MaxProcs is how many resident processes may run for this script. 0 → 1.
	MaxProcs int
}

// RunMRSScript executes a skill and returns its final JSON object.
//
// The script writes a stream of JSON objects to stdout. Objects carrying
// "_progress" are progress reports and are forwarded to OnProgress; the last
// object without one is the result.
func RunMRSScript(ctx context.Context, scriptPath string, opt MRSRunOptions) (map[string]any, error) {
	if opt.CacheTTL > 0 {
		if cached, ok := mrsCacheGet(scriptPath, opt.Args, opt.CacheTTL); ok {
			return cached, nil
		}
	}

	var (
		result map[string]any
		err    error
	)
	if opt.Serve && !mrsServeUnsupported(scriptPath) {
		result, err = runMRSResident(ctx, scriptPath, opt)
		if err != nil && mrsShouldFallBackToSpawn(err) {
			// The script does not speak the resident protocol (or the process
			// is wedged). Spawning still works, so degrade rather than fail.
			result, err = runMRSSpawn(ctx, scriptPath, opt)
		}
	} else {
		result, err = runMRSSpawn(ctx, scriptPath, opt)
	}
	if err != nil {
		return nil, err
	}

	if opt.CacheTTL > 0 {
		mrsCachePut(scriptPath, opt.Args, result)
	}
	return result, nil
}

// ─── spawn path (the original behaviour) ────────────────────────────────────

// runMRSSpawn starts a fresh interpreter for one call. This is the superset of
// what the two old implementations did: stderr is captured so a traceback
// reaches the want's log, and progress lines are forwarded.
func runMRSSpawn(ctx context.Context, scriptPath string, opt MRSRunOptions) (map[string]any, error) {
	cmdArgs := append([]string{scriptPath}, opt.Args...)
	cmd := exec.CommandContext(ctx, "python3", cmdArgs...)
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	var finalResult map[string]any
	decoder := json.NewDecoder(stdout)
	for decoder.More() {
		var obj map[string]any
		if err := decoder.Decode(&obj); err != nil {
			break // unrecoverable parse error; check stderr below
		}
		if pct, ok := obj["_progress"]; ok {
			if opt.OnProgress != nil {
				opt.OnProgress(int(mrsFloat(pct)), mrsStr(obj["_message"]))
			}
			continue
		}
		finalResult = obj
	}

	if err := cmd.Wait(); err != nil {
		// Prefer the script's own structured error over the exit code.
		if finalResult != nil {
			if msg, ok := finalResult["error"].(string); ok && msg != "" {
				return nil, fmt.Errorf("%s", msg)
			}
		}
		if stderr := strings.TrimSpace(stderrBuf.String()); stderr != "" {
			return nil, fmt.Errorf("exit error: %w\nstderr: %s", err, stderr)
		}
		return nil, fmt.Errorf("exit error: %w", err)
	}

	if finalResult == nil {
		return nil, fmt.Errorf("skill produced no JSON output")
	}
	return finalResult, nil
}

// ─── resident path ──────────────────────────────────────────────────────────

// errMRSSpawnInstead marks failures where the resident process could not serve
// the request but a plain spawn still can.
var errMRSSpawnInstead = fmt.Errorf("mrs: resident unavailable, spawn instead")

func mrsShouldFallBackToSpawn(err error) bool {
	return err == errMRSSpawnInstead
}

// residentProc is one long-lived interpreter speaking the request/response
// protocol on its stdin/stdout.
type residentProc struct {
	scriptPath string

	mu    sync.Mutex // one in-flight request per process
	cmd   *exec.Cmd
	stdin io.WriteCloser
	// dec is created once per process, not per request: a json.Decoder buffers
	// ahead, so a fresh one would drop whatever the previous decoder had
	// already pulled out of the pipe.
	dec    *json.Decoder
	nextID int64
}

// residentPool holds the processes for one script. Requests take a process out
// of free, use it, and put it back — so MaxProcs bounds concurrency without
// any spinning.
type residentPool struct {
	free     chan *residentProc
	maxProcs int
}

var (
	mrsPoolMu sync.Mutex
	mrsPools  = map[string]*residentPool{}

	// Scripts that answered without an _id are not serve-capable. Recorded once
	// so a mistaken `serve: true` costs one failed attempt, not one per call.
	mrsNoServeMu sync.Mutex
	mrsNoServe   = map[string]bool{}
)

func mrsServeUnsupported(scriptPath string) bool {
	mrsNoServeMu.Lock()
	defer mrsNoServeMu.Unlock()
	return mrsNoServe[scriptPath]
}

func mrsMarkServeUnsupported(scriptPath string) {
	mrsNoServeMu.Lock()
	mrsNoServe[scriptPath] = true
	mrsNoServeMu.Unlock()
	ErrorLog("[MRS] %s did not answer with _id — falling back to spawn for this script", scriptPath)
}

func mrsGetPool(scriptPath string, maxProcs int) *residentPool {
	if maxProcs <= 0 {
		maxProcs = 1
	}
	mrsPoolMu.Lock()
	defer mrsPoolMu.Unlock()
	if p, ok := mrsPools[scriptPath]; ok {
		return p
	}
	p := &residentPool{free: make(chan *residentProc, maxProcs), maxProcs: maxProcs}
	for i := 0; i < maxProcs; i++ {
		p.free <- &residentProc{scriptPath: scriptPath} // started lazily
	}
	mrsPools[scriptPath] = p
	return p
}

func runMRSResident(ctx context.Context, scriptPath string, opt MRSRunOptions) (map[string]any, error) {
	pool := mrsGetPool(scriptPath, opt.MaxProcs)

	var proc *residentProc
	select {
	case proc = <-pool.free:
	case <-ctx.Done():
		return nil, errMRSSpawnInstead // all processes busy and we ran out of time
	}
	defer func() { pool.free <- proc }()

	proc.mu.Lock()
	defer proc.mu.Unlock()

	if err := proc.ensureStarted(); err != nil {
		return nil, errMRSSpawnInstead
	}

	result, err := proc.request(ctx, opt)
	if err != nil {
		// A wedged or crashed process must not poison the next caller.
		proc.kill()
		if err == errMRSSpawnInstead {
			return nil, err
		}
		return nil, err
	}
	return result, nil
}

// ensureStarted brings the interpreter up if it is not running. MYWANT_MRS_SERVE
// is how the script knows to enter its stdin loop instead of doing one job.
func (p *residentProc) ensureStarted() error {
	if p.cmd != nil && p.cmd.Process != nil {
		return nil
	}
	cmd := exec.Command("python3", p.scriptPath)
	cmd.Env = append(os.Environ(), "MYWANT_MRS_SERVE=1")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd, p.stdin = cmd, stdin
	p.dec = json.NewDecoder(bufio.NewReaderSize(stdout, 64*1024))
	DebugLog("[MRS] resident started: %s (PID %d)", p.scriptPath, cmd.Process.Pid)
	return nil
}

func (p *residentProc) kill() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Kill()
	_, _ = p.cmd.Process.Wait()
	p.cmd, p.stdin, p.dec = nil, nil, nil
}

// request writes one job and reads until the matching answer arrives.
//
// Reads happen on a goroutine so a script that stops answering cannot pin the
// caller past its context deadline; the process is killed by the caller in
// that case, which also unblocks the reader.
func (p *residentProc) request(ctx context.Context, opt MRSRunOptions) (map[string]any, error) {
	id := atomic.AddInt64(&p.nextID, 1)

	req := map[string]any{"_id": id}
	if len(opt.Args) > 0 {
		req["args"] = opt.Args
	}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := p.stdin.Write(append(line, '\n')); err != nil {
		return nil, errMRSSpawnInstead
	}

	type answer struct {
		obj map[string]any
		err error
	}
	dec := p.dec
	done := make(chan answer, 1)
	go func() {
		for {
			var obj map[string]any
			if err := dec.Decode(&obj); err != nil {
				done <- answer{err: errMRSSpawnInstead}
				return
			}
			rawID, hasID := obj["_id"]
			if !hasID {
				// Not the resident protocol — this is a one-shot script that
				// printed its result and will now exit.
				mrsMarkServeUnsupported(p.scriptPath)
				done <- answer{err: errMRSSpawnInstead}
				return
			}
			if int64(mrsFloat(rawID)) != id {
				continue // a late reply to an abandoned request
			}
			if pct, ok := obj["_progress"]; ok {
				if opt.OnProgress != nil {
					opt.OnProgress(int(mrsFloat(pct)), mrsStr(obj["_message"]))
				}
				continue
			}
			delete(obj, "_id")
			done <- answer{obj: obj}
			return
		}
	}()

	select {
	case a := <-done:
		if a.err != nil {
			return nil, a.err
		}
		if msg, ok := a.obj["error"].(string); ok && msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return a.obj, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("skill timed out: %w", ctx.Err())
	}
}

// StopAllMRSResidents kills every resident interpreter. Called on shutdown so
// no orphaned python3 survives the server.
func StopAllMRSResidents() {
	mrsPoolMu.Lock()
	pools := make([]*residentPool, 0, len(mrsPools))
	for _, p := range mrsPools {
		pools = append(pools, p)
	}
	mrsPools = map[string]*residentPool{}
	mrsPoolMu.Unlock()

	for _, pool := range pools {
		for i := 0; i < pool.maxProcs; i++ {
			select {
			case proc := <-pool.free:
				proc.mu.Lock()
				proc.kill()
				proc.mu.Unlock()
			case <-time.After(2 * time.Second):
			}
		}
	}
}

// ─── result cache ───────────────────────────────────────────────────────────

type mrsCacheEntry struct {
	at     time.Time
	result map[string]any
}

var (
	mrsCacheMu sync.Mutex
	mrsCache   = map[string]mrsCacheEntry{}
)

func mrsCacheKey(scriptPath string, args []string) string {
	return scriptPath + "\x00" + strings.Join(args, "\x00")
}

func mrsCacheGet(scriptPath string, args []string, ttl time.Duration) (map[string]any, bool) {
	mrsCacheMu.Lock()
	defer mrsCacheMu.Unlock()
	e, ok := mrsCache[mrsCacheKey(scriptPath, args)]
	if !ok || time.Since(e.at) > ttl {
		return nil, false
	}
	// Copy: the caller stores this into want state, and two wants sharing one
	// map would let a later mutation of one show up in the other.
	copied, _ := deepCopyValue(e.result).(map[string]any)
	return copied, true
}

func mrsCachePut(scriptPath string, args []string, result map[string]any) {
	stored, _ := deepCopyValue(result).(map[string]any)
	mrsCacheMu.Lock()
	mrsCache[mrsCacheKey(scriptPath, args)] = mrsCacheEntry{at: time.Now(), result: stored}
	mrsCacheMu.Unlock()
}

// ─── small helpers ──────────────────────────────────────────────────────────

func mrsFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

func mrsStr(v any) string {
	s, _ := v.(string)
	return s
}
