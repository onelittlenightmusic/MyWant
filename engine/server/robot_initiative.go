package server

// The robot acting on its own.
//
// Until now the robot moved only when it was spoken to: a question from the
// phone, a turn, a walk. This lets it act on what happens on the board — a
// want arriving at a new answer, a long quiet — the way a guide standing in the
// room would: go and tell, or come back and wait, or leave the person be.
//
// What it can do on its own is a short list of its own capabilities
// (robotInitiatives). Each says, for an event, whether it has anything to do
// (Offers) and, in a sentence, what it is (Criterion). Which one it does is a
// typed decision: the event said in a few words, and two questions —
//
//	interrupt: may the robot talk to the person now?   (talk / wait)
//	action:    which of the offered capabilities?      (one of them)
//
// asked of Laya by whichever worker takes the job — the iPhone or Mac app,
// or a GPU server running tools/laya_worker.py — and of a few plain rules
// when no worker is there or none answers. Laya picks only among
// what was offered: it never names a want or a cell, the capabilities do.
//
// A capability that speaks is not run when the answer to interrupt is wait —
// the robot does the best one that keeps quiet instead. Every decision is kept,
// one JSON line each, in ~/.mywant/robot-initiative.jsonl: what the robot saw,
// what was asked, the probabilities, what it did — the rows to teach Laya
// with (agent.fit) once there are enough of them.
//
// Off unless MYWANT_ROBOT_INITIATIVE=1.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	mywant "mywant/engine/core"
)

const (
	initiativeOutput = "output" // a want arrived at a new answer
	initiativeIdle   = "idle"   // nothing has happened for a while

	// initiativeIdleAfter is the quiet that is an event, unless
	// MYWANT_ROBOT_IDLE_AFTER says otherwise (a Go duration, e.g. 3m).
	initiativeIdleAfter = 10 * time.Minute
	// initiativeTick is how often the quiet is looked at.
	initiativeTick = 30 * time.Second
	// initiativeSpeakGap is the least time between two things the robot
	// says unasked, whatever the decision: a burst of answers is not a burst
	// of remarks.
	initiativeSpeakGap = 20 * time.Second
	// initiativeNear is how close (cells) counts as already beside the person.
	initiativeNear = 2
	// initiativeMessageMax is how much of a notice the decision reads (runes).
	initiativeMessageMax = 40
	// initiativeWarmup: answers arriving this soon after the start are the
	// wants running again as the board comes back, not news — a restart
	// had the robot weigh six of them in two seconds.
	initiativeWarmup = 30 * time.Second
)

type initiativeEvent struct {
	Kind     string
	WantID   string
	WantName string
	WantType string
	Message  string // the notice, in the want's own voice
	At       time.Time
	// Injected: handed in through the API (a demonstration, a test) — not
	// news that reached the person, so what they did after it teaches
	// nothing (robotLabelInitiatives passes it over).
	Injected bool
}

// initiativeSituation is what the robot knows when it decides.
type initiativeSituation struct {
	Event initiativeEvent
	// SinceSpokenTo: since the person last said something to the robot
	// (-1: never). SinceSpoke: since the robot last said something unasked.
	SinceSpokenTo time.Duration
	SinceSpoke    time.Duration
	// Distance from the robot to the person, in cells (-1: not known).
	Distance int
	// The robot's cell and the person's, when known.
	RobotX, RobotY   int
	PersonX, PersonY int
	HasPerson        bool
	// The want's tile, for an event about a want.
	TileX, TileY int
	HasTile      bool
}

// initiativeCapability is one thing the robot can do on its own.
type initiativeCapability struct {
	Name string
	// Criterion is what Laya reads as this option: what doing it means.
	Criterion string
	// Speaks: doing it talks to the person, so it needs interrupt = talk.
	Speaks bool
	Offers func(sit initiativeSituation) bool
	Run    func(s *Server, sit initiativeSituation) (string, error)
}

var robotInitiatives = []initiativeCapability{
	{
		Name:      "report",
		Criterion: "そのwantのところへ行って、新しい結果を本人に伝える",
		Speaks:    true,
		Offers: func(sit initiativeSituation) bool {
			return sit.Event.Kind == initiativeOutput && sit.HasTile && sit.Event.Message != ""
		},
		Run: func(s *Server, sit initiativeSituation) (string, error) {
			if err := s.robotWalkRobot(sit.TileX, sit.TileY); err != nil {
				return "", err
			}
			words := fmt.Sprintf("「%s」: %s", sit.Event.WantName, sit.Event.Message)
			if err := s.robotSayAs(words, speechNotice); err != nil {
				return "", err
			}
			return fmt.Sprintf("walked to %s (%d, %d) and said: %s", sit.Event.WantName, sit.TileX, sit.TileY, words), nil
		},
	},
	{
		Name:      "return",
		Criterion: "本人のそばに戻って待つ",
		Offers: func(sit initiativeSituation) bool {
			return sit.HasPerson && sit.Distance > initiativeNear
		},
		Run: func(s *Server, sit initiativeSituation) (string, error) {
			// Beside them, on the side the robot is coming from — not on top.
			x, y := sit.PersonX+sign(sit.RobotX-sit.PersonX), sit.PersonY+sign(sit.RobotY-sit.PersonY)
			if x == sit.PersonX && y == sit.PersonY {
				x++
			}
			if err := s.robotWalkRobot(x, y); err != nil {
				return "", err
			}
			return fmt.Sprintf("walked back beside the person, to (%d, %d)", x, y), nil
		},
	},
	{
		Name:      "stay",
		Criterion: "何もしないで、今の場所で静かにしている",
		Offers:    func(initiativeSituation) bool { return true },
		Run: func(*Server, initiativeSituation) (string, error) {
			return "stayed where it was", nil
		},
	},
}

// ── deciding ────────────────────────────────────────────────────────────────

// decisionQuestion is a Laya question: a choice among criteria.
type decisionQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type decisionAnswer struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// The model ops a worker may take (model_jobs.go), as fm's are: a typed
// decision, and a training of the decision's heads on the labels this server
// keeps. Whoever polls saying it can (pending-action?can=decide,train-decider)
// is handed them — the iPhone app, the Mac's, or a server with a GPU running
// tools/laya_worker.py beside this one or anywhere it reaches this from. This
// server only queues the work and keeps the data; it needs no model of its
// own (Fly).
const (
	modelOpDecide       = "decide"
	modelOpTrainDecider = "train-decider"
)

// initiativeDecideWait is how long a worker has to answer a decision; the
// rules decide when none did. The app asks for work every ten seconds or so
// (its BrowserRunner), and nobody is waiting on the robot's own initiative:
// long enough for the next ask, and for a worker slowed by a training.
const initiativeDecideWait = 15 * time.Second

// decisionItem is one state with its questions — a decision asks one, or
// several at once (the offers: one per candidate).
type decisionItem struct {
	State     string                      `json:"state"`
	Questions map[string]decisionQuestion `json:"questions"`
}

// robotDecider hands decisions and trainings to whichever worker can take
// them; errNoModel when none has asked lately (the rules decide then).
type robotDecider struct{}

// decide returns the answers and who gave them.
func (d *robotDecider) decide(state string, questions map[string]decisionQuestion) (map[string]decisionAnswer, string, error) {
	all, by, err := d.decideMany([]decisionItem{{State: state, Questions: questions}})
	if err != nil {
		return nil, by, err
	}
	return all[0], by, nil
}

// decideMany asks several at once: each its own job, queued together and
// waited on side by side, so every worker that is free takes the next — the
// more workers there are (the phone, the Mac, a GPU server), the sooner all
// are answered. `by` is the workers that answered.
func (d *robotDecider) decideMany(items []decisionItem) ([]map[string]decisionAnswer, string, error) {
	if !modelDeviceCan(modelOpDecide, "", time.Now()) {
		return nil, "", errNoModel
	}
	answers := make([]map[string]decisionAnswer, len(items))
	workers := make([]string, len(items))
	errs := make([]error, len(items))
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(i int, item decisionItem) {
			defer wg.Done()
			answers[i], workers[i], errs[i] = decideOne(item)
		}(i, item)
	}
	wg.Wait()
	seen := map[string]bool{}
	var by []string
	for i, err := range errs {
		if err != nil {
			return nil, strings.Join(by, ","), err
		}
		if !seen[workers[i]] {
			seen[workers[i]] = true
			by = append(by, workers[i])
		}
	}
	return answers, strings.Join(by, ","), nil
}

// decideOne is one item as one job: {"items": [item]} → {"items": [{"answers"}], "worker"}.
func decideOne(item decisionItem) (map[string]decisionAnswer, string, error) {
	// The heads go with the job: whichever worker takes it decides with what
	// was learnt, wherever it was learnt (initiativeHeads).
	in, _ := json.Marshal(map[string]any{"items": []decisionItem{item}, "heads": initiativeHeads()})
	res, ok := waitForModelDevice(context.Background(),
		modelClaim{RequestID: generateWantID(), Op: modelOpDecide, Input: in}, initiativeDecideWait)
	if !ok {
		return nil, "", fmt.Errorf("no worker answered within %s", initiativeDecideWait)
	}
	if res.Error != "" {
		return nil, "laya-worker", fmt.Errorf("%s", res.Error)
	}
	var out struct {
		Worker string `json:"worker"`
		Items  []struct {
			Answers map[string]decisionAnswer `json:"answers"`
		} `json:"items"`
	}
	if err := json.Unmarshal(res.Output, &out); err != nil {
		return nil, "laya-worker", err
	}
	by := "laya-worker"
	if out.Worker != "" {
		by = "laya@" + out.Worker
	}
	if len(out.Items) != 1 {
		return nil, by, fmt.Errorf("asked 1, answered %d", len(out.Items))
	}
	return out.Items[0].Answers, by, nil
}

// train is one job, taken by whichever worker that can train asks first: it
// fits the heads on the rows and hands them back, and they are kept here
// (initiativeHeads) and sent with every decision from then on — so one
// worker's training serves them all, and no worker keeps anything of its own.
func (d *robotDecider) train(rows []map[string]any, questions map[string]decisionQuestion) (json.RawMessage, string, error) {
	if !modelDeviceCan(modelOpTrainDecider, "", time.Now()) {
		return nil, "", errNoModel
	}
	// seeded: the rows are the whole set (initiativeTrainingSet).
	in, _ := json.Marshal(map[string]any{"rows": rows, "questions": questions, "seeded": true})
	res, ok := waitForModelDevice(context.Background(),
		modelClaim{RequestID: generateWantID(), Op: modelOpTrainDecider, Input: in}, 20*time.Minute)
	if !ok {
		return nil, "", fmt.Errorf("no worker finished the training")
	}
	if res.Error != "" {
		return nil, "laya-worker", fmt.Errorf("%s", res.Error)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(res.Output, &out); err != nil {
		return nil, "laya-worker", err
	}
	by := "laya-worker"
	var worker string
	if json.Unmarshal(out["worker"], &worker) == nil && worker != "" {
		by = "laya@" + worker
	}
	if heads, ok := out["heads"]; ok {
		if err := keepInitiativeHeads(heads); err != nil {
			return nil, by, err
		}
		delete(out, "heads") // kept apart; the report is what it came to
	}
	report, _ := json.Marshal(out)
	return report, by, nil
}

// The heads the last training fitted, by question (LayaDecider.headKey), in
// the JSON every worker reads (LayaHead): kept in
// ~/.mywant/robot-initiative-heads.json, read again when the file changes.
var initiativeHeadsCache struct {
	sync.Mutex
	stamp fileStamp
	heads json.RawMessage
}

func initiativeHeadsFile() string { return initiativeHomeFile("robot-initiative-heads.json") }

func initiativeHeads() json.RawMessage {
	initiativeHeadsCache.Lock()
	defer initiativeHeadsCache.Unlock()
	path := initiativeHeadsFile()
	if st := stampOf(path); st != initiativeHeadsCache.stamp || initiativeHeadsCache.heads == nil {
		raw, err := os.ReadFile(path)
		if err != nil || !json.Valid(raw) {
			raw = json.RawMessage("{}")
		}
		initiativeHeadsCache.heads, initiativeHeadsCache.stamp = raw, st
	}
	return initiativeHeadsCache.heads
}

func keepInitiativeHeads(heads json.RawMessage) error {
	if !json.Valid(heads) {
		return fmt.Errorf("the worker's heads are not JSON")
	}
	tmp := initiativeHeadsFile() + ".tmp"
	if err := os.WriteFile(tmp, heads, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, initiativeHeadsFile())
}

// initiativeRules is the decision without a model: tell a new answer unless
// the robot spoke a moment ago; in a quiet, come back to the person.
func initiativeRules(sit initiativeSituation, offered []initiativeCapability) map[string]decisionAnswer {
	interrupt := "talk"
	if sit.SinceSpoke >= 0 && sit.SinceSpoke < 5*time.Minute {
		interrupt = "wait"
	}
	pick := initiativeAction(interrupt, offered)
	return map[string]decisionAnswer{
		"interrupt": {Choice: interrupt},
		"action":    {Choice: pick},
	}
}

// initiativeAction is what follows from interrupt: tell it when there is
// something to tell, else come back when far, else stay.
func initiativeAction(interrupt string, offered []initiativeCapability) string {
	has := map[string]bool{}
	for _, c := range offered {
		has[c.Name] = true
	}
	switch {
	case interrupt == "talk" && has["report"]:
		return "report"
	case has["return"]:
		return "return"
	}
	return "stay"
}

// newsQuestionID is the one thing Laya is asked about an event: is what the
// want found news the person would want to hear now? Words, which Laya reads
// well. When the robot spoke last and how far away the person is are numbers
// ("10秒", "12マス"), which it reads badly — asked of it as one question with
// them in the state, a fitted head told the golf slot not to be said with the
// robot silent all along. Those the code weighs (initiativeInterrupt).
const newsQuestionID = "news"

var newsQuestion = decisionQuestion{
	Type:         "choice",
	Instructions: "この結果は、本人がすぐ知りたい新しい知らせか",
	Criteria: map[string]string{
		"news":    "知らせる価値がある。空きが出た、雨、遅れ、失敗、確定など、本人の予定や行動に関わる",
		"routine": "いつも通り。件数だけ、変化なし、確認しただけ",
	},
}

// initiativeQuestions are what Laya is asked — the same every time, so a head
// fitted on past replies is fitted to exactly these.
func initiativeQuestions([]initiativeCapability) map[string]decisionQuestion {
	return map[string]decisionQuestion{newsQuestionID: newsQuestion}
}

// initiativeRecent is how soon after a remark the robot keeps quiet.
const initiativeRecent = 3 * time.Minute

// initiativeInterrupt: talk when there is news and the robot has not spoken
// lately.
func initiativeInterrupt(news bool, sit initiativeSituation) string {
	if news && (sit.SinceSpoke < 0 || sit.SinceSpoke >= initiativeRecent) {
		return "talk"
	}
	return "wait"
}

// initiativeEventText is the event in words, without the numbers: what Laya
// reads ("" for a quiet, which has no words to read).
func initiativeEventText(sit initiativeSituation) string {
	if sit.Event.Kind != initiativeOutput {
		return ""
	}
	msg := []rune(sit.Event.Message)
	if len(msg) > initiativeMessageMax {
		msg = append(msg[:initiativeMessageMax], '…')
	}
	return fmt.Sprintf("出来事: wantの新しい結果。want: %s。結果: %s。", sit.Event.WantName, string(msg))
}

// initiativeState says the situation in a few words — short, so it fits the
// Neural Engine's 96 tokens with the questions.
func initiativeState(sit initiativeSituation) string {
	var b strings.Builder
	switch sit.Event.Kind {
	case initiativeOutput:
		msg := []rune(sit.Event.Message)
		if len(msg) > initiativeMessageMax {
			msg = append(msg[:initiativeMessageMax], '…')
		}
		fmt.Fprintf(&b, "出来事: wantの新しい結果。want: %s。結果: %s。", sit.Event.WantName, string(msg))
	case initiativeIdle:
		fmt.Fprintf(&b, "出来事: 何もない時間が%s続いた。", initiativeAgo(time.Since(sit.Event.At)+initiativeIdleWait()))
	}
	fmt.Fprintf(&b, "本人が最後に話してから: %s。", initiativeAgo(sit.SinceSpokenTo))
	fmt.Fprintf(&b, "ロボットが最後に話しかけてから: %s。", initiativeAgo(sit.SinceSpoke))
	if sit.Distance >= 0 {
		fmt.Fprintf(&b, "本人までの距離: %dマス。", sit.Distance)
	}
	return b.String()
}

// initiativeAgo is a duration as the decision reads it: coarse.
func initiativeAgo(d time.Duration) string {
	switch {
	case d < 0:
		return "なし"
	case d < time.Minute:
		return fmt.Sprintf("%d秒", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d分", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d時間", int(d.Hours()))
	}
}

// initiativeChoose is what the robot does: the likeliest action on offer —
// the decision's own choice when it is — leaving out one that speaks when
// the decision said wait.
func initiativeChoose(offered []initiativeCapability, answers map[string]decisionAnswer) initiativeCapability {
	quiet := answers["interrupt"].Choice == "wait"
	action := answers["action"]
	rank := func(c initiativeCapability) float64 {
		if c.Name == action.Choice {
			return 2 // above any probability
		}
		return action.Probabilities[c.Name]
	}
	// Likeliest first; the rules give no probabilities, so their list order.
	rest := append([]initiativeCapability(nil), offered...)
	sort.SliceStable(rest, func(i, j int) bool { return rank(rest[i]) > rank(rest[j]) })
	for _, c := range rest {
		if !(quiet && c.Speaks) {
			return c
		}
	}
	return robotInitiatives[len(robotInitiatives)-1] // stay
}

// ── the loop ────────────────────────────────────────────────────────────────

type robotInitiative struct {
	events  chan initiativeEvent
	decider *robotDecider

	started time.Time

	mu        sync.Mutex
	lastSpoke time.Time // the robot said something unasked
	lastEvent time.Time // the last event, or the start
	idleFired bool      // the current quiet has had its event
	logPath   string
}

func robotInitiativeEnabled() bool {
	return os.Getenv("MYWANT_ROBOT_INITIATIVE") == "1"
}

func initiativeIdleWait() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("MYWANT_ROBOT_IDLE_AFTER")); err == nil && d > 0 {
		return d
	}
	return initiativeIdleAfter
}

// startRobotInitiative sets the robot going on its own, when it is asked to.
func (s *Server) startRobotInitiative() {
	if !robotInitiativeEnabled() {
		return
	}
	ri := &robotInitiative{
		events:    make(chan initiativeEvent, 16),
		started:   time.Now(),
		lastEvent: time.Now(),
		logPath:   filepath.Join(mywant.MyWantHomeDir(), "robot-initiative.jsonl"),
	}
	ri.decider = &robotDecider{}
	s.initiative = ri
	log.Printf("[initiative] on (decider: %s, idle after %s)", ri.deciderName(), initiativeIdleWait())
	go s.runRobotInitiative(ri)
}

func (ri *robotInitiative) deciderName() string {
	return "a worker that can (the app, or a GPU server's laya), else the rules"
}

// robotInitiativeNotify hands the loop an event; never blocks (a full queue
// drops it — the robot has plenty to do already).
func (s *Server) robotInitiativeNotify(ev initiativeEvent) {
	ri := s.initiative
	if ri == nil {
		return
	}
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	if ev.Kind == initiativeOutput && ev.At.Sub(ri.started) < initiativeWarmup {
		return
	}
	select {
	case ri.events <- ev:
	default:
		log.Printf("[initiative] queue full, dropped %s for %s", ev.Kind, ev.WantName)
	}
}

func (s *Server) runRobotInitiative(ri *robotInitiative) {
	tick := time.NewTicker(initiativeTick)
	defer tick.Stop()
	for {
		select {
		case ev := <-ri.events:
			s.robotTakeInitiative(ri, ev)
		case now := <-tick.C:
			if ev, ok := s.robotQuiet(ri, now); ok {
				s.robotTakeInitiative(ri, ev)
			}
			// What the person did after the robot's decisions, and a retrain
			// once enough of that has gathered (robot_initiative_learn.go).
			s.robotLabelInitiatives(ri, now)
			if robotOfferExpire(now) {
				s.showOffer("", "")
			}
			s.robotMaybeRetrain(ri)
		}
	}
}

// robotQuiet is the idle event, once per quiet: nothing from the person and
// no event for initiativeIdleWait.
func (s *Server) robotQuiet(ri *robotInitiative, now time.Time) (initiativeEvent, bool) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	last := ri.lastEvent
	if t := s.robotLastSpokenTo(); t.After(last) {
		last = t
		ri.idleFired = false
	}
	if ri.idleFired || now.Sub(last) < initiativeIdleWait() {
		return initiativeEvent{}, false
	}
	ri.idleFired = true
	return initiativeEvent{Kind: initiativeIdle, At: now}, true
}

// robotLastSpokenTo is when the person last said something to the robot —
// the last message in its chat, from the phone or the board alike.
func (s *Server) robotLastSpokenTo() time.Time {
	robot, err := s.robotWantNow()
	if err != nil {
		return time.Time{}
	}
	messages := mywant.GetCurrent(robot, "cc_messages", []any{})
	for i := len(messages) - 1; i >= 0; i-- {
		m, _ := messages[i].(map[string]any)
		if ts, _ := m["timestamp"].(string); ts != "" {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// robotSituation is what the robot knows about the event and the board.
func (s *Server) robotSituation(ri *robotInitiative, ev initiativeEvent) (initiativeSituation, bool) {
	sit := initiativeSituation{Event: ev, SinceSpokenTo: -1, SinceSpoke: -1, Distance: -1}
	robot, err := s.robotWantNow()
	if err != nil {
		return sit, false
	}
	labels := robot.GetLabels()
	rx, errX := strconv.Atoi(labels[robotCanvasX])
	ry, errY := strconv.Atoi(labels[robotCanvasY])
	if errX != nil || errY != nil {
		return sit, false // a robot not on the board has nowhere to go from
	}
	sit.RobotX, sit.RobotY = rx, ry
	if t := s.robotLastSpokenTo(); !t.IsZero() {
		sit.SinceSpokenTo = time.Since(t)
	}
	ri.mu.Lock()
	if !ri.lastSpoke.IsZero() {
		sit.SinceSpoke = time.Since(ri.lastSpoke)
	}
	ri.mu.Unlock()
	if s.globalBuilder != nil {
		if px, py, ok := resolveCanvasNear(canvasNearCursor, s.globalBuilder.GetWants()); ok {
			sit.PersonX, sit.PersonY, sit.HasPerson = px, py, true
			sit.Distance = chebyshev(px-rx, py-ry)
		}
	}
	if ev.WantID != "" {
		if w := s.findWantByIDOrName(ev.WantID); w != nil {
			l := w.GetLabels()
			x, errX := strconv.Atoi(l[robotCanvasX])
			y, errY := strconv.Atoi(l[robotCanvasY])
			if errX == nil && errY == nil && l[robotArchived] != "true" {
				sit.TileX, sit.TileY, sit.HasTile = x, y, true
			}
		}
	}
	return sit, true
}

// initiativeRecord is one decision, as kept.
type initiativeRecord struct {
	ID        string                      `json:"id,omitempty"`
	WantID    string                      `json:"wantId,omitempty"`
	At        string                      `json:"at"`
	Event     string                      `json:"event"`
	Want      string                      `json:"want,omitempty"`
	WantType  string                      `json:"wantType,omitempty"`
	Injected  bool                        `json:"injected,omitempty"`
	State     string                      `json:"state"`             // what Laya read
	Context   string                      `json:"context,omitempty"` // the situation in full, numbers and all
	Offered   []string                    `json:"offered"`
	Answers   map[string]decisionAnswer   `json:"answers"`
	By        string                      `json:"by"`
	Did       string                      `json:"did"`
	Result    string                      `json:"result,omitempty"`
	Error     string                      `json:"error,omitempty"`
	Questions map[string]decisionQuestion `json:"questions"`
}

func (s *Server) robotTakeInitiative(ri *robotInitiative, ev initiativeEvent) initiativeRecord {
	ri.mu.Lock()
	ri.lastEvent = ev.At
	if ev.Kind != initiativeIdle {
		ri.idleFired = false // a new quiet starts after it
	}
	ri.mu.Unlock()

	sit, ok := s.robotSituation(ri, ev)
	if !ok {
		return initiativeRecord{Event: ev.Kind, Did: "none", Error: "the robot is not on the board"}
	}
	var offered []initiativeCapability
	for _, c := range robotInitiatives {
		if c.Offers(sit) {
			offered = append(offered, c)
		}
	}
	text := initiativeEventText(sit)
	questions := initiativeQuestions(offered)
	rec := initiativeRecord{
		ID: fmt.Sprintf("init-%d", time.Now().UnixNano()), WantID: ev.WantID,
		At: ev.At.Format(time.RFC3339), Event: ev.Kind, Want: ev.WantName, WantType: ev.WantType,
		State: text, Context: initiativeState(sit), By: "rules", Injected: ev.Injected,
	}
	for _, c := range offered {
		rec.Offered = append(rec.Offered, c.Name)
	}

	// Laya reads the words (is it news?); the rest is the code's.
	answers := initiativeRules(sit, offered)
	if text != "" {
		started := time.Now()
		if a, by, err := ri.decider.decide(text, questions); err == nil {
			rec.By, rec.Questions = by, questions
			news := a[newsQuestionID].Choice == "news"
			interrupt := initiativeInterrupt(news, sit)
			answers = map[string]decisionAnswer{
				newsQuestionID: a[newsQuestionID],
				"interrupt":    {Choice: interrupt},
				"action":       {Choice: initiativeAction(interrupt, offered)},
			}
			log.Printf("[initiative] %s read it in %s: %s", rec.By, time.Since(started).Round(time.Millisecond), a[newsQuestionID].Choice)
		} else if err != errNoModel {
			log.Printf("[initiative] laya did not answer (%v); deciding by the rules", err)
		}
	}
	rec.Answers = answers

	chosen := initiativeChoose(offered, answers)
	// However the decision went: not two remarks in a breath.
	if chosen.Speaks && sit.SinceSpoke >= 0 && sit.SinceSpoke < initiativeSpeakGap {
		chosen = initiativeChoose(offered, map[string]decisionAnswer{"interrupt": {Choice: "wait"}, "action": answers["action"]})
	}
	rec.Did = chosen.Name
	result, err := chosen.Run(s, sit)
	rec.Result = result
	if err != nil {
		rec.Error = err.Error()
	} else if chosen.Speaks {
		ri.mu.Lock()
		ri.lastSpoke = time.Now()
		ri.mu.Unlock()
	}
	log.Printf("[initiative] %s %s → %s (%s)", ev.Kind, ev.WantName, rec.Did, rec.By)
	ri.keep(rec)
	// Told: and what could it do about it (robot_offer.go)?
	if chosen.Name == "report" && err == nil {
		s.robotOffer(ri, sit)
	}
	return rec
}

// keep appends the decision to the log: the rows to teach Laya with.
func (ri *robotInitiative) keep(rec initiativeRecord) {
	if ri.logPath == "" {
		return
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(ri.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("[initiative] could not keep the decision: %v", err)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// chebyshev is the distance in cells, diagonals counting one.
func chebyshev(dx, dy int) int {
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
