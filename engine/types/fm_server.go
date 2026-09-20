package types

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
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
	// What the binary looked like when this process was started. A resident
	// agent outlives every install: the robot was answering from a build made
	// eleven minutes before the one on disk, with the day's fixes in the file
	// and not in the process, and nothing said so. Compared before each
	// question; a changed binary means a new process.
	stamp string

	// Where the agent's running commentary goes while it works. Set for the
	// duration of one question (ask holds the lock, so there is only ever
	// one), and called from the goroutine draining the agent's stderr — which
	// is why what it writes to must be safe to write to from anywhere.
	activityMu sync.Mutex
	onActivity func(kind, text string)
}

// watch installs the commentary handler for one question.
func (s *fmServer) watch(handler func(kind, text string)) {
	s.activityMu.Lock()
	s.onActivity = handler
	s.activityMu.Unlock()
}

func (s *fmServer) say(kind, text string) {
	s.activityMu.Lock()
	handler := s.onActivity
	s.activityMu.Unlock()
	if handler != nil {
		handler(kind, text)
	}
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
	// A command the agent will not run until a person says yes, written as
	// they would read it. Empty when nothing is waiting.
	Pending string `json:"pending"`
	// A request the agent handed back rather than answering: making or
	// finding something takes several commands in an order that depends on
	// what the earlier ones found, and that is worked out here (see
	// runGoalInline) rather than in an 8k model. The words are the person's
	// own, because the model paraphrasing them is the first thing to go wrong.
	Goal string `json:"goal"`
	Error   string `json:"error"`
}

// binaryStamp identifies the build on disk: when it was written, and how big
// it is. Enough to notice an install, cheap enough to check every time.
func (s *fmServer) binaryStamp() string {
	info, err := os.Stat(s.binary)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())
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
	// Its stderr is the agent's own commentary — which tool it reached for,
	// which command it ran, why a session was dropped. Read, not left to the
	// log: "考えています" is all anybody could see while it worked, and every
	// line of what it was actually doing was going past on a pipe nobody was
	// holding. Drained continuously, so a full pipe can never wedge the agent.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd, s.stdin, s.stdout, s.started = cmd, stdin, bufio.NewReaderSize(stdout, 1<<20), true
	s.stamp = s.binaryStamp()
	go s.readCommentary(stderr)
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

// readCommentary turns the agent's stderr into activity, line by line, until
// the process ends.
//
// Two kinds of line are worth passing on: the command it ran, and the tool it
// reached for. The rest is its own bookkeeping — how many commands it was
// offered, when a session was trimmed — which belongs in the log it is already
// going to.
func (s *fmServer) readCommentary(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		log.Printf("[fmtool] %s", line)
		switch {
		case strings.HasPrefix(line, "[mywant WOULD RUN"):
			s.say("note", strings.TrimSuffix(strings.TrimPrefix(line, "[mywant WOULD RUN "), "]"))
		case strings.HasPrefix(line, "[mywant "):
			s.say("tool", strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
		case strings.HasPrefix(line, "[tool:"):
			s.say("tool", strings.Trim(line, "[]"))
		}
	}
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

// ask puts one question to the conversation and waits for its answer.
func (s *fmServer) ask(prompt, root string, timeout time.Duration) (fmReply, error) {
	return s.request(prompt, root, timeout, false)
}

// askPlain puts a question to the model alone: no tools, and no memory of the
// chat going on beside it.
//
// For a caller that is not chatting. MyWant works out which of its commands
// carry out a request and asks this only for the language part — and the first
// time it asked through the ordinary session, the agent went off and used its
// own tools to answer a question that was never addressed to it, reporting that
// "the search results did not provide the required information" and planning
// nothing.
func (s *fmServer) askPlain(prompt, root string, timeout time.Duration) (fmReply, error) {
	return s.request(prompt, root, timeout, true)
}

// request does the talking, starting or restarting the agent as needed. Two
// attempts: a process that died between questions is not an error anybody asked
// about, it is one to recover from.
func (s *fmServer) request(prompt, root string, timeout time.Duration, plain bool) (fmReply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// A new build on disk retires the running one. The conversation goes with
	// it, which is the right trade: an agent that answers from a binary
	// nobody has any more is worse than one that forgets what was just said.
	if s.started && s.stamp != "" && s.binaryStamp() != s.stamp {
		log.Printf("[fmtool] the binary changed; starting the new one")
		s.stop()
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if !s.started {
			if err := s.start(root); err != nil {
				return fmReply{}, fmt.Errorf("cannot start the on-device agent: %w", err)
			}
		}
		s.nextID++
		body := map[string]any{"id": s.nextID, "prompt": prompt}
		if plain {
			body["plain"] = true
		}
		request, err := json.Marshal(body)
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
