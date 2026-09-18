package types

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// One on-device agent, kept alive, so a conversation is one conversation.
//
// Run as a command, fmtool is a process per question: a new session each time,
// nothing remembered. "新宿はどこ？" then "その隣は？" made the second question
// meaningless — there was nothing for it to be next to.
//
// `fmtool --serve` stays up and keeps one session, answering questions off its
// stdin as JSON lines. This is that process, from this side: started when the
// first question arrives, kept for as long as the server runs, and started
// again by itself if it ever dies. One question at a time, because one session
// is one thread of talk and two interleaved would be neither.

// fmServer talks to a live `fmtool --serve`.
type fmServer struct {
	mu      sync.Mutex
	binary  string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	nextID  int
	started bool
}

var (
	fmServers   = map[string]*fmServer{}
	fmServersMu sync.Mutex
)

// fmServerFor returns the live agent for one binary, starting nothing yet.
func fmServerFor(binary string) *fmServer {
	fmServersMu.Lock()
	defer fmServersMu.Unlock()
	if s, ok := fmServers[binary]; ok {
		return s
	}
	s := &fmServer{binary: binary}
	fmServers[binary] = s
	return s
}

// fmReply is one answer off the served agent.
type fmReply struct {
	ID      int    `json:"id"`
	Text    string `json:"text"`
	Tool    string `json:"tool"`
	Calls   int    `json:"calls"`
	Trimmed bool   `json:"trimmed"`
	Error   string `json:"error"`
}

// start brings the process up. The caller holds the lock.
func (s *fmServer) start(root string) error {
	args := []string{"--serve"}
	if root != "" {
		args = append(args, "--root", root)
	}
	cmd := exec.Command(s.binary, args...)
	cmd.Env = sanitizedSubprocessEnv()
	if root != "" {
		cmd.Dir = root
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	// Its stderr is the agent's own commentary — which tool it reached for, why
	// a session was dropped. Left to flow to this server's log rather than
	// collected: nothing here reads it, and a full pipe would wedge the agent.
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd, s.stdin, s.stdout, s.started = cmd, stdin, bufio.NewReaderSize(stdout, 1<<20), true
	go func() {
		_ = cmd.Wait()
		s.mu.Lock()
		if s.cmd == cmd {
			s.started = false
		}
		s.mu.Unlock()
	}()
	return nil
}

// stop closes the conversation down. The caller holds the lock.
func (s *fmServer) stop() {
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	s.started = false
	s.cmd, s.stdin, s.stdout = nil, nil, nil
}

// ask puts one question and waits for its answer, starting or restarting the
// agent as needed. Two attempts: a process that died between questions is not
// an error anybody asked about, it is one to recover from.
func (s *fmServer) ask(prompt, root string, timeout time.Duration) (fmReply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if !s.started {
			if err := s.start(root); err != nil {
				return fmReply{}, fmt.Errorf("cannot start the on-device agent: %w", err)
			}
		}
		s.nextID++
		request, err := json.Marshal(map[string]any{"id": s.nextID, "prompt": prompt})
		if err != nil {
			return fmReply{}, err
		}
		if _, err := s.stdin.Write(append(request, '\n')); err != nil {
			lastErr = err
			s.stop()
			continue
		}

		reply, err := s.readReply(timeout)
		if err != nil {
			lastErr = err
			s.stop()
			continue
		}
		if reply.Error != "" {
			return reply, fmt.Errorf("%s", reply.Error)
		}
		return reply, nil
	}
	return fmReply{}, fmt.Errorf("the on-device agent did not answer: %w", lastErr)
}

// readReply waits for one line, or gives up. A timeout kills the process: the
// session is mid-answer and no longer in step with this side, and a fresh one
// is cheaper than an interleaved one.
func (s *fmServer) readReply(timeout time.Duration) (fmReply, error) {
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	reader := s.stdout
	go func() {
		line, err := reader.ReadString('\n')
		done <- result{line, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			return fmReply{}, r.err
		}
		var reply fmReply
		if err := json.Unmarshal([]byte(strings.TrimSpace(r.line)), &reply); err != nil {
			return fmReply{}, fmt.Errorf("unreadable answer: %s", strings.TrimSpace(r.line))
		}
		return reply, nil
	case <-time.After(timeout):
		return fmReply{}, fmt.Errorf("no answer within %s", timeout)
	}
}

// resetSession forgets the conversation without restarting the process.
func (s *fmServer) resetSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return
	}
	s.nextID++
	request, _ := json.Marshal(map[string]any{"id": s.nextID, "reset": true})
	if _, err := s.stdin.Write(append(request, '\n')); err != nil {
		s.stop()
		return
	}
	// The acknowledgement is read so it is not mistaken for the next answer.
	if _, err := s.readReply(10 * time.Second); err != nil {
		s.stop()
	}
}
