package server

// story.go — a character's story: what the person playing it did on the board.
//
// Where they went from and to, what they opened, what they asked and what
// they were answered — so a chat's answers can be told good or bad by what
// the person did next (story_outcome.go, to come). Nothing is asked of the
// apps: everything here is what the server is sent anyway.
//
//	PUT /cursors/{character}   moves → canvas.move (stopped for storyDwell),
//	                           say; stepping onto or off a want's cell is
//	                           want.arrive / want.leave, told by the
//	                           intersections (intersection.go: storyRule)
//	PUT /gui/state             a character's keys → want.open / want.close,
//	                           want.expand, view.change
//	turns                      chat.ask, chat.answer (the cards shown)
//	acts                       act.done (what the plan did)
//
// Events are held per character and written every storyFlushEvery, a JSON
// line each, to ~/.mywant/stories/<character>/<YYYY-MM-DD>.jsonl. A day's
// file is gzipped after storyRawDays and let go after storyGzipDays (a day's
// digest, kept for good, is story_digest.go's, to come).
//
// The events are want-spec's StoryEvent (types_story.go).

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	mywant "mywant/engine/core"

	ws "github.com/onelittlenightmusic/want-spec"
)

const (
	storyFlushEvery = 30 * time.Second
	storyTickEvery  = 2 * time.Second
	// storyDwell: stopped this long somewhere that is not a tile, a move is
	// a move (a walk through is not).
	storyDwell    = 2 * time.Second
	storyRawDays  = 10
	storyGzipDays = 90
	// storyUnknown is whose story an event goes into when no character is
	// known (a turn from a device no character is assigned to).
	storyUnknown = "_unknown"
	// storySayMatch: a turn asking what a character said this recently is
	// that character's.
	storySayMatch = 2 * time.Minute
)

// storyState is what a character was last seen doing, to tell what changed.
type storyState struct {
	hasCell        bool
	x, y           int
	onWant         bool           // standing on a want (its arrival is the move)
	lastWant       *ws.StoryPlace // the last want stood on (where a walk came from)
	still          *ws.StoryPlace // the cell last stopped on
	movedAt        time.Time      // the last step, for a stop
	moved          bool           // stepped since last stopped
	gui            map[string]string
	lastSay        string
	lastSayAt      time.Time
	lastSayDevice  string
	lastDeviceSeen string
}

type storyRecorder struct {
	mu      sync.Mutex
	dir     string // "" is ~/.mywant/stories
	now     func() time.Time
	pending map[string][]ws.StoryEvent
	state   map[string]*storyState
	turns   map[string]string // turn → character, for the events that follow it
	tidied  string            // the day old files were last seen to
	start   sync.Once
	// judging: the turns waiting for what the person does next
	// (story_outcome.go); afterEvent, what an event decided, held after it.
	judging    map[string]*storyJudging
	afterEvent []func()
	// onOutcome is told each turn's outcome (the turn keeps it, turns.go).
	onOutcome func(turn, outcome string)
}

var story = newStoryRecorder("")

func newStoryRecorder(dir string) *storyRecorder {
	return &storyRecorder{
		dir: dir, now: time.Now,
		pending: map[string][]ws.StoryEvent{},
		state:   map[string]*storyState{},
		turns:   map[string]string{},
		judging: map[string]*storyJudging{},
	}
}

func (r *storyRecorder) root() string {
	if r.dir != "" {
		return r.dir
	}
	return filepath.Join(mywant.MyWantHomeDir(), "stories")
}

// running starts the recorder's clock once: stops noticed, events written,
// old days tidied.
func (r *storyRecorder) running() {
	r.start.Do(func() {
		go func() {
			tick := time.NewTicker(storyTickEvery)
			defer tick.Stop()
			last := r.now()
			for range tick.C {
				r.noticeStops()
				if r.now().Sub(last) >= storyFlushEvery {
					last = r.now()
					if err := r.flush(); err != nil {
						log.Printf("[story] %v", err)
					}
				}
			}
		}()
	})
}

func (r *storyRecorder) st(character string) *storyState {
	s := r.state[character]
	if s == nil {
		s = &storyState{gui: map[string]string{}}
		r.state[character] = s
	}
	return s
}

// addLocked holds an event for its character (r.mu held).
func (r *storyRecorder) addLocked(ev ws.StoryEvent) {
	if ev.Character == "" {
		ev.Character = storyUnknown
	}
	if ev.At == "" {
		ev.At = r.now().Format(time.RFC3339)
	}
	r.judgeLocked(&ev)
	r.pending[ev.Character] = append(r.pending[ev.Character], ev)
	after := r.afterEvent
	r.afterEvent = nil
	for _, decide := range after {
		decide()
	}
}

func (r *storyRecorder) add(ev ws.StoryEvent) {
	r.running()
	r.mu.Lock()
	r.addLocked(ev)
	r.mu.Unlock()
}

func storyCell(x, y int) *ws.StoryPlace {
	return &ws.StoryPlace{Kind: "cell", X: &x, Y: &y}
}

func samePlace(a, b *ws.StoryPlace) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	if a.ID != "" || b.ID != "" {
		return a.ID == b.ID
	}
	return a.X != nil && b.X != nil && a.Y != nil && b.Y != nil && *a.X == *b.X && *a.Y == *b.Y
}

// cursor is a character standing at x, y (cells), said to be there by a
// device, saying message (when newly said). Wants stepped onto or off are
// told by arrive and leave (intersection.go), not worked out here.
func (r *storyRecorder) cursor(character string, x, y int, device, message string) {
	r.running()
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.st(character)
	if device != "" {
		s.lastDeviceSeen = device
	}
	if message != "" {
		s.lastSay, s.lastSayAt, s.lastSayDevice = message, now, device
		r.addLocked(ws.StoryEvent{Character: character, Kind: ws.StoryKindSay, Device: device,
			To: storyCell(x, y), Detail: map[string]string{"text": message}})
	}
	if s.hasCell && s.x == x && s.y == y {
		return
	}
	first := !s.hasCell
	s.hasCell, s.x, s.y = true, x, y
	if first {
		s.still = storyCell(x, y)
		return
	}
	s.moved, s.movedAt = true, now
}

// arrive is a character stepping onto a want's cell (intersection.go): from
// the last want stood on, else from where they last stopped.
func (r *storyRecorder) arrive(character string, want *ws.StoryPlace) {
	r.running()
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.st(character)
	from := s.lastWant
	if from == nil {
		from = s.still
	}
	r.addLocked(ws.StoryEvent{Character: character, Kind: ws.StoryKindWantArrive, Device: s.lastDeviceSeen, From: from, To: want})
	s.lastWant, s.onWant, s.moved = want, true, false
	if want.X != nil && want.Y != nil {
		s.still = storyCell(*want.X, *want.Y)
	}
}

// leave is a character stepping off a want's cell (intersection.go).
func (r *storyRecorder) leave(character string, want *ws.StoryPlace) {
	r.running()
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.st(character)
	r.addLocked(ws.StoryEvent{Character: character, Kind: ws.StoryKindWantLeave, Device: s.lastDeviceSeen, From: want})
	s.onWant = false
}

// noticeStops writes a canvas.move for each character that has stood still
// for storyDwell somewhere that is not a tile.
func (r *storyRecorder) noticeStops() {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked() // and the turns whose time to be judged is up
	for character, s := range r.state {
		if !s.moved || s.onWant || now.Sub(s.movedAt) < storyDwell {
			continue
		}
		to := storyCell(s.x, s.y)
		if !samePlace(s.still, to) {
			r.addLocked(ws.StoryEvent{Character: character, Kind: ws.StoryKindCanvasMove,
				At: s.movedAt.Format(time.RFC3339), Device: s.lastDeviceSeen, From: s.still, To: to})
		}
		s.still, s.moved = to, false
	}
}

// The GUI's per-character keys a story reads (PUT /gui/state), each
// <prefix><character>.
var storyGUIKeys = []string{"sidebar_want_id_", "sidebar_open_", "expanded_want_id_", "sidebar_active_tab_", "canvas_mode_"}

// guiState is a PUT /gui/state's updates: what changed in a character's own
// keys. want names a want by id.
func (r *storyRecorder) guiState(updates map[string]any, want func(id string) *ws.StoryPlace) {
	type change struct{ character, prefix, value string }
	var changes []change
	for key, v := range updates {
		for _, p := range storyGUIKeys {
			if c, ok := strings.CutPrefix(key, p); ok && c != "" {
				val := ""
				if v != nil {
					val = fmt.Sprint(v)
				}
				changes = append(changes, change{c, p, val})
			}
		}
	}
	if len(changes) == 0 {
		return
	}
	r.running()
	sort.Slice(changes, func(i, j int) bool { return changes[i].prefix < changes[j].prefix })
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range changes {
		s := r.st(c.character)
		was, seen := s.gui[c.prefix]
		s.gui[c.prefix] = c.value
		if !seen || was == c.value {
			continue // the first seen is where they are, not a change
		}
		ev := ws.StoryEvent{Character: c.character, Device: s.lastDeviceSeen}
		switch c.prefix {
		case "sidebar_want_id_", "expanded_want_id_":
			kind := ws.StoryKindWantOpen
			if c.prefix == "expanded_want_id_" {
				kind = ws.StoryKindWantExpand
			}
			if was != "" {
				ev.From = want(was)
			}
			if c.value == "" {
				if kind == ws.StoryKindWantExpand {
					continue
				}
				ev.Kind = ws.StoryKindWantClose
			} else {
				ev.Kind, ev.To = kind, want(c.value)
			}
		case "sidebar_open_":
			continue // said by the want's id
		default:
			ev.Kind = ws.StoryKindViewChange
			view := strings.TrimSuffix(c.prefix, "_")
			ev.From = &ws.StoryPlace{Kind: "view", ID: view, Name: was}
			ev.To = &ws.StoryPlace{Kind: "view", ID: view, Name: c.value}
		}
		r.addLocked(ev)
	}
}

// characterOf is whose a turn is: the character named, else the device's,
// else the character who just said its question.
func (r *storyRecorder) characterOf(character, device, question string, byDevice func(string) string) string {
	if character != "" {
		return character
	}
	if device != "" {
		if c := byDevice(device); c != "" {
			return c
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	q := strings.TrimSpace(question)
	best, bestAt := "", time.Time{}
	for c, s := range r.state {
		if s.lastSay == "" || r.now().Sub(s.lastSayAt) > storySayMatch {
			continue
		}
		said := strings.TrimSpace(s.lastSay)
		if q != "" && (strings.Contains(said, q) || strings.Contains(q, said)) && s.lastSayAt.After(bestAt) {
			best, bestAt = c, s.lastSayAt
		}
	}
	return best
}

// ask is a turn opened: its character remembered for what follows it.
func (r *storyRecorder) ask(character, turn, device, question string) {
	r.running()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.turns[turn] = character
	if len(r.turns) > 500 {
		r.turns = map[string]string{turn: character}
	}
	if character == "" {
		character = storyUnknown
	}
	r.addLocked(ws.StoryEvent{Character: character, Kind: ws.StoryKindChatAsk, Ref: turn, Device: device,
		Detail: map[string]string{"question": question}})
	r.askedLocked(character, turn, question)
}

// turnCharacter is whose a turn was, as it was asked.
func (r *storyRecorder) turnCharacter(turn string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.turns[turn]
}

// answer is a turn answered, with the cards it showed.
func (r *storyRecorder) answer(turn string, cards []robotCard, failed bool) {
	shown := make([]string, 0, len(cards))
	for _, c := range cards {
		shown = append(shown, c.Kind+":"+c.ID)
	}
	detail := map[string]string{"cards": strings.Join(shown, ",")}
	if failed {
		detail["error"] = "true"
	}
	r.running()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addLocked(ws.StoryEvent{Character: r.turns[turn], Kind: ws.StoryKindChatAnswer, Ref: turn, Detail: detail})
	if failed {
		delete(r.judging, turn) // nothing was answered to judge
		return
	}
	r.answeredLocked(turn, cards)
}

// actDone is an act finished: what its plan did, in the turn's story.
func (r *storyRecorder) actDone(a ws.Act, plan ws.PlanSpec) {
	detail := map[string]string{"phase": a.Status.Phase, "plan": a.Spec.Plan, "action": plan.Action}
	// The turn it answered, and what a new want was given: the day's digest
	// keeps the plan whole beside the question (story_digest.go).
	if a.Spec.Turn != "" {
		detail["turn"] = a.Spec.Turn
	}
	for k, v := range plan.Params {
		if v != "" {
			detail["param."+k] = v
		}
	}
	for k, v := range map[string]string{"target": plan.Target, "capability": plan.Capability, "type": plan.Type} {
		if v != "" {
			detail[k] = v
		}
	}
	if r := a.Status.Message; r != "" {
		detail["message"] = r
	}
	r.running()
	r.mu.Lock()
	defer r.mu.Unlock()
	ev := ws.StoryEvent{Character: r.turns[a.Spec.Turn], Kind: ws.StoryKindActDone, Ref: a.Metadata.Name, Detail: detail}
	for _, c := range a.Status.Cards {
		if c != nil {
			ev.To = &ws.StoryPlace{Kind: c.Kind, ID: c.ID, Name: c.Name, Type: c.Type}
			break
		}
	}
	r.addLocked(ev)
	if a.Status.Phase == "Succeeded" {
		r.actedLocked(a.Spec.Turn, plan, a.Status.Cards)
	}
}

// ── written, read, tidied ──────────────────────────────────────────────────

var storyCharacterOK = regexp.MustCompile(`^[A-Za-z0-9_.@-]+$`)

func storyDay(at string, loc *time.Location) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		t = time.Now()
	}
	return t.In(loc).Format("2006-01-02")
}

// flush writes what is held, a line an event, to each character's day.
func (r *storyRecorder) flush() error {
	r.mu.Lock()
	pending := r.pending
	r.pending = map[string][]ws.StoryEvent{}
	r.mu.Unlock()
	var firstErr error
	for character, events := range pending {
		if !storyCharacterOK.MatchString(character) {
			continue
		}
		byDay := map[string][]ws.StoryEvent{}
		for _, ev := range events {
			d := storyDay(ev.At, time.Local)
			byDay[d] = append(byDay[d], ev)
		}
		dir := filepath.Join(r.root(), character)
		if err := os.MkdirAll(dir, 0755); err != nil {
			firstErr = err
			continue
		}
		for day, evs := range byDay {
			if err := appendStoryLines(filepath.Join(dir, day+".jsonl"), evs); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	// Once a day (a little past midnight): the days over are digested
	// (story_digest.go), kept for good, before old days are gzipped or let go.
	if due := r.digestDue(); due != r.tidied {
		r.tidied = due
		if err := r.digestDays(); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := r.tidy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func appendStoryLines(path string, events []ws.StoryEvent) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// tidy gzips each day older than storyRawDays and lets go of the gzipped
// days older than storyGzipDays.
func (r *storyRecorder) tidy() error {
	root := r.root()
	characters, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	today := r.now().In(time.Local)
	rawBefore := today.AddDate(0, 0, -storyRawDays).Format("2006-01-02")
	gzBefore := today.AddDate(0, 0, -storyGzipDays).Format("2006-01-02")
	for _, c := range characters {
		if !c.IsDir() {
			continue
		}
		dir := filepath.Join(root, c.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			name := f.Name()
			switch {
			case strings.HasSuffix(name, ".jsonl"):
				day := strings.TrimSuffix(name, ".jsonl")
				if day < gzBefore {
					// Past keeping even gzipped (a server long stopped): let go —
					// its digest, written before this, keeps the day.
					_ = os.Remove(filepath.Join(dir, name))
				} else if day < rawBefore {
					if err := gzipFile(filepath.Join(dir, name)); err != nil {
						log.Printf("[story] gzip %s: %v", name, err)
					}
				}
			case strings.HasSuffix(name, ".jsonl.gz"):
				if day := strings.TrimSuffix(name, ".jsonl.gz"); day < gzBefore {
					_ = os.Remove(filepath.Join(dir, name))
				}
			}
		}
	}
	return nil
}

// gzipFile writes path.gz beside path and then lets path go.
func gzipFile(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(path + ".gz.tmp")
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	if _, err := io.Copy(zw, in); err != nil {
		out.Close()
		os.Remove(out.Name())
		return err
	}
	if err := zw.Close(); err != nil {
		out.Close()
		os.Remove(out.Name())
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(out.Name())
		return err
	}
	if err := os.Rename(path+".gz.tmp", path+".gz"); err != nil {
		return err
	}
	return os.Remove(path)
}

// storyQuery is what a read asks for.
type storyQuery struct {
	since, until time.Time
	kinds        map[string]bool
	limit        int
}

// read is a character's events in [since, until), oldest first — the last
// limit of them — from its days on disk (plain or gzipped) and what is held.
func (r *storyRecorder) read(character string, q storyQuery) ([]ws.StoryEvent, error) {
	if !storyCharacterOK.MatchString(character) {
		return nil, fmt.Errorf("bad character %q", character)
	}
	var out []ws.StoryEvent
	keep := func(ev ws.StoryEvent) {
		t, err := time.Parse(time.RFC3339, ev.At)
		if err != nil || t.Before(q.since) || !t.Before(q.until) {
			return
		}
		if len(q.kinds) > 0 && !q.kinds[ev.Kind] {
			return
		}
		out = append(out, ev)
	}
	dir := filepath.Join(r.root(), character)
	// A day at a time from the start of since's day — not since's time of
	// day, or the last day (until's, earlier in the day than since) is skipped.
	s := q.since.In(time.Local)
	for d := time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, time.Local); d.Before(q.until); d = d.AddDate(0, 0, 1) {
		day := d.Format("2006-01-02")
		for _, name := range []string{day + ".jsonl", day + ".jsonl.gz"} {
			if err := readStoryFile(filepath.Join(dir, name), keep); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		}
	}
	r.mu.Lock()
	for _, ev := range r.pending[character] {
		keep(ev)
	}
	r.mu.Unlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	if q.limit > 0 && len(out) > q.limit {
		out = out[len(out)-q.limit:]
	}
	return out, nil
}

func readStoryFile(path string, each func(ws.StoryEvent)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var src io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer zr.Close()
		src = zr
	}
	sc := bufio.NewScanner(src)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var ev ws.StoryEvent
		if json.Unmarshal(sc.Bytes(), &ev) == nil {
			each(ev)
		}
	}
	return sc.Err()
}

// ── from the server's own records ──────────────────────────────────────────

// storyWant is a want named by its id, as a story place.
func (s *Server) storyWant(id string) *ws.StoryPlace {
	p := &ws.StoryPlace{Kind: "want", ID: id}
	if s.globalBuilder != nil {
		if w, _, ok := s.globalBuilder.FindWantByID(id); ok && w != nil {
			p.Name, p.Type = w.Metadata.Name, w.Metadata.Type
		}
	}
	return p
}

// storyDeviceCharacter is the character a device is assigned to, or "".
func storyDeviceCharacter(device string) string {
	for _, c := range mywant.ListCharacters() {
		for _, d := range c.AssignedDeviceIDs {
			if d == device {
				return c.ID
			}
		}
	}
	return ""
}

// storyCursor is a cursor PUT, as its story sees it.
func (s *Server) storyCursor(character string, x, y float64, device, message string) {
	story.cursor(character, int(math.Round(x)), int(math.Round(y)), device, message)
}

// storyRule puts a character's stepping onto and off a want into their story
// — one more rule of the intersections (intersection.go), which already work
// out, once, what a mover arrived on and left. Characters only, every want
// but one that is a character itself; not announced.
var storyRule = intersectionRule{
	name:  "story",
	quiet: true,
	applies: func(_ intersectionContext, w *mywant.Want, m mover) bool {
		// Not a want that IS a character (the robot stands on its own want
		// wherever it goes), nor a system one.
		return m.kind == moverCharacter && w != nil &&
			!characterWantTypes[w.Metadata.Type] && !w.Metadata.IsSystemWant
	},
	enter: func(_ intersectionContext, w *mywant.Want, m mover) { story.arrive(m.id, storyWantPlace(w)) },
	leave: func(_ intersectionContext, w *mywant.Want, m mover) { story.leave(m.id, storyWantPlace(w)) },
}

// storyWantPlace is a want as a story place, with its cell when it has one.
func storyWantPlace(w *mywant.Want) *ws.StoryPlace {
	p := &ws.StoryPlace{Kind: "want", ID: w.Metadata.ID, Name: w.Metadata.Name, Type: w.Metadata.Type}
	labels := w.GetLabels()
	if x, err := strconv.Atoi(labels[canvasXLabelKey]); err == nil {
		if y, err := strconv.Atoi(labels[canvasYLabelKey]); err == nil {
			p.X, p.Y = &x, &y
		}
	}
	return p
}
