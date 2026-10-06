package mywant

// mrs_shared.go — every MRS script through one Python: the MRS agent service.
//
// A plugin's script used to cost a python3 per call, or with `serve: true` a
// resident interpreter per plugin — ~25MB apiece, most of it the interpreter
// itself. On the 256mb Fly Machine two of those, the engine and the GUI left
// ~10MB free, and the Machine thrashed (mywant-deploy README, 2026-10-06).
//
// So the scripts go to the engine's external-agent service instead
// (ExecuteRequest in, ExecuteResponse out — webhook.go): mrs_agent_service.py,
// one process the engine starts on first use and keeps, which loads each
// script once and runs it as a spawn would. No plugin is changed for it.
//
// A script that cannot share an interpreter declares `isolated: true` in its
// agent.yaml (skill_isolated in a skill_path want's state) and is run as
// before; MYWANT_MRS_SHARED=0 turns the service off altogether. When the
// service cannot be reached the call is spawned instead — only when it never
// started, never once it may have: a script half run is not run twice.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed mrs_agent_service.py
var mrsAgentServicePy []byte

// mrsSharedEnabled: on unless MYWANT_MRS_SHARED says otherwise.
func mrsSharedEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MYWANT_MRS_SHARED"))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

type mrsSharedService struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan struct{} // closed when cmd exits
	base string
	// After a failed start, spawn for a while rather than retry on every call.
	failedAt time.Time
}

var (
	mrsShared       mrsSharedService
	mrsSharedClient = &http.Client{} // no client timeout: the caller's context is the deadline
)

const mrsSharedRetryAfter = 30 * time.Second

// url is the running service's address, starting it if it is not running.
func (s *mrsSharedService) url() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil {
		select {
		case <-s.done:
			s.cmd = nil // it exited; start another below
		default:
			return s.base, nil
		}
	}
	if !s.failedAt.IsZero() && time.Since(s.failedAt) < mrsSharedRetryAfter {
		return "", fmt.Errorf("mrs agent service failed to start %s ago", time.Since(s.failedAt).Round(time.Second))
	}
	if err := s.start(); err != nil {
		s.failedAt = time.Now()
		ErrorLog("[MRS-SHARED] could not start the agent service, spawning instead: %v", err)
		return "", err
	}
	s.failedAt = time.Time{}
	return s.base, nil
}

// start runs the embedded service. Called with mu held.
func (s *mrsSharedService) start() error {
	script, err := mrsSharedScriptPath()
	if err != nil {
		return err
	}
	cmd := exec.Command("python3", script)
	// MYWANT_MRS_SERVE would send a plugin into its own stdin loop.
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "MYWANT_MRS_SERVE=") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// The first line is {"port": n}.
	type hello struct {
		Port int `json:"port"`
	}
	got := make(chan hello, 1)
	go func() {
		r := bufio.NewReader(stdout)
		line, _ := r.ReadBytes('\n')
		var h hello
		_ = json.Unmarshal(line, &h)
		got <- h
		_, _ = io.Copy(io.Discard, r)
	}()
	var h hello
	select {
	case h = <-got:
	case <-time.After(10 * time.Second):
	}
	if h.Port == 0 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("the service did not say its port")
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
		InfoLog("[MRS-SHARED] agent service (PID %d) exited", cmd.Process.Pid)
	}()
	s.cmd, s.done, s.base = cmd, done, fmt.Sprintf("http://127.0.0.1:%d", h.Port)
	InfoLog("[MRS-SHARED] agent service started (PID %d) at %s", cmd.Process.Pid, s.base)
	return nil
}

// stop ends the service — on shutdown, and when it stops answering.
func (s *mrsSharedService) stop() {
	s.mu.Lock()
	cmd, done := s.cmd, s.done
	s.cmd = nil
	s.mu.Unlock()
	if cmd == nil {
		return
	}
	_ = cmd.Process.Kill()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

// mrsSharedScriptPath writes the embedded service where python3 can run it,
// named by its content so a new engine never runs an old service.
func mrsSharedScriptPath() (string, error) {
	sum := sha256.Sum256(mrsAgentServicePy)
	path := filepath.Join(os.TempDir(), "mywant-mrs-agent-service-"+hex.EncodeToString(sum[:6])+".py")
	if b, err := os.ReadFile(path); err == nil && bytes.Equal(b, mrsAgentServicePy) {
		return path, nil
	}
	tmp := path + fmt.Sprintf(".%d.tmp", os.Getpid())
	if err := os.WriteFile(tmp, mrsAgentServicePy, 0o644); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// runMRSShared runs one script call on the service.
func runMRSShared(ctx context.Context, scriptPath string, opt MRSRunOptions) (map[string]any, error) {
	base, err := mrsShared.url()
	if err != nil {
		return nil, errMRSSpawnInstead
	}
	body, err := json.Marshal(ExecuteRequest{
		AgentName: filepath.Base(filepath.Dir(scriptPath)),
		Operation: "mrs",
		Params:    map[string]any{"script": scriptPath, "args": opt.Args},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/execute", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := mrsSharedClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("skill timed out: %w", ctx.Err())
		}
		if errors.Is(err, syscall.ECONNREFUSED) {
			// Never reached it: nothing ran, so spawning is safe.
			mrsShared.stop()
			return nil, errMRSSpawnInstead
		}
		mrsShared.stop()
		return nil, fmt.Errorf("mrs agent service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("mrs agent service: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("skill timed out: %w", ctx.Err())
			}
			// The service went away mid-call. The script may have done part of
			// its work, so it is not run again here.
			mrsShared.stop()
			return nil, fmt.Errorf("mrs agent service ended before answering: %w", err)
		}
		if pct, ok := obj["_progress"]; ok {
			if opt.OnProgress != nil {
				opt.OnProgress(int(mrsFloat(pct)), mrsStr(obj["_message"]))
			}
			continue
		}
		switch mrsStr(obj["status"]) {
		case "completed":
			result, _ := obj["state_updates"].(map[string]any)
			if result == nil {
				return nil, fmt.Errorf("skill produced no JSON output")
			}
			return result, nil
		case "failed":
			return nil, fmt.Errorf("%s", mrsStr(obj["error"]))
		}
	}
}
