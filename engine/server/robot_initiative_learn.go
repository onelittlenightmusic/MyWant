package server

// The robot learning from what came of its decisions.
//
// Every decision the robot makes on its own is kept (robot-initiative.jsonl).
// Some time after one about a want's new answer (MYWANT_ROBOT_LABEL_AFTER,
// 10m), what the person did says whether it was worth saying: they opened the
// want's notice, or spoke to the robot — it was (talk); they did neither — it
// was not (wait). That is the decision's label, appended to
// robot-initiative-labels.jsonl, whatever the robot did: a notice the robot
// kept quiet about and the person went and read anyway is a talk it missed.
// A label the person gives by hand (POST /api/v1/robot/initiative/label) is
// kept the same way and, being later, wins.
//
// Once MYWANT_LAYA_RETRAIN_EVERY (20) labels have gathered since the last
// training, a worker that can is handed the training (train-decider: the
// app's LayaDecider, or tools/laya_worker.py on a GPU server) and answers
// with the new heads already serving — the next decision is the retrained
// robot's. What each training came to is kept in
// robot-initiative-trained.json.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	mywant "mywant/engine/core"
)

const (
	initiativeLabelAfter   = 10 * time.Minute
	initiativeRetrainEvery = 20
)

type initiativeLabel struct {
	ID        string `json:"id"`
	At        string `json:"at"`
	Interrupt string `json:"interrupt,omitempty"`
	// Gold: the label of any question by its id (an offer's: offer →
	// accept / skip); Interrupt is the older form of {interrupt: …}.
	Gold   map[string]string `json:"gold,omitempty"`
	Reason string            `json:"reason"`
	By     string            `json:"by"` // outcome | person
}

// gold is the label as question id → label.
func (l initiativeLabel) gold() map[string]string {
	if len(l.Gold) > 0 {
		return l.Gold
	}
	return nil // the older interrupt-only labels were about another question
}

type initiativeTrained struct {
	Labels int             `json:"labels"`
	At     string          `json:"at"`
	Report json.RawMessage `json:"report,omitempty"`
}

var initiativeTraining atomic.Bool

func initiativeLabelWait() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("MYWANT_ROBOT_LABEL_AFTER")); err == nil && d > 0 {
		return d
	}
	return initiativeLabelAfter
}

func initiativeRetrainCount() int {
	if n, err := strconv.Atoi(os.Getenv("MYWANT_LAYA_RETRAIN_EVERY")); err == nil && n > 0 {
		return n
	}
	return initiativeRetrainEvery
}

func initiativeHomeFile(name string) string {
	return filepath.Join(mywant.MyWantHomeDir(), name)
}

// readJSONLines reads one JSON value a line into each.
func readJSONLines[T any](path string, each func(T)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var v T
		if json.Unmarshal(sc.Bytes(), &v) == nil {
			each(v)
		}
	}
}

func appendJSONLine(path string, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// initiativeLabels is the label of each decision, the last one given winning.
func initiativeLabels() map[string]initiativeLabel {
	out := map[string]initiativeLabel{}
	readJSONLines(initiativeHomeFile("robot-initiative-labels.jsonl"), func(l initiativeLabel) {
		out[l.ID] = l
	})
	return out
}

// robotLabelInitiatives labels the decisions old enough to have an outcome.
func (s *Server) robotLabelInitiatives(ri *robotInitiative, now time.Time) {
	labels := initiativeLabels()
	wait := initiativeLabelWait()
	spokenTo := s.robotLastSpokenTo()
	readJSONLines(ri.logPath, func(rec initiativeRecord) {
		if rec.ID == "" || rec.Event != initiativeOutput || rec.WantID == "" || rec.Injected {
			return
		}
		if _, done := labels[rec.ID]; done {
			return
		}
		at, err := time.Parse(time.RFC3339, rec.At)
		if err != nil || now.Sub(at) < wait {
			return
		}
		// Whether it was news to them: what the person did next says.
		l := initiativeLabel{ID: rec.ID, At: now.Format(time.RFC3339), By: "outcome",
			Gold:   map[string]string{newsQuestionID: "routine"},
			Reason: "the person neither opened the notice nor spoke to the robot"}
		if s.initiativeAlertRead(rec.WantID, at) {
			l.Gold[newsQuestionID], l.Reason = "news", "the person opened the want's notice"
		} else if !spokenTo.Before(at) && spokenTo.Sub(at) <= wait {
			l.Gold[newsQuestionID], l.Reason = "news", "the person spoke to the robot after it"
		}
		if err := appendJSONLine(initiativeHomeFile("robot-initiative-labels.jsonl"), l); err == nil {
			log.Printf("[initiative] %s (%s) labelled %s: %s", rec.ID, rec.Want, l.Interrupt, l.Reason)
		}
	})
}

// initiativeAlertRead: a notice of the want's from about then on has been read.
func (s *Server) initiativeAlertRead(wantID string, since time.Time) bool {
	if s.notifications == nil {
		return false
	}
	entries, _ := s.notifications.ForWant(wantID, 50)
	for _, e := range entries {
		at, err := time.Parse(time.RFC3339, e.At)
		if err != nil || at.Before(since.Add(-time.Minute)) {
			continue
		}
		if e.Kind == "alert" && e.Read {
			return true
		}
	}
	return false
}

func initiativeLastTrained() initiativeTrained {
	var t initiativeTrained
	raw, err := os.ReadFile(initiativeHomeFile("robot-initiative-trained.json"))
	if err == nil {
		_ = json.Unmarshal(raw, &t)
	}
	return t
}

// robotMaybeRetrain trains again once enough new labels have gathered.
func (s *Server) robotMaybeRetrain(ri *robotInitiative) {
	n := len(initiativeLabels())
	if n-initiativeLastTrained().Labels < initiativeRetrainCount() {
		return
	}
	s.robotRetrain(ri, n)
}

// initiativeWindow is how far back the person's replies are taught one by one
// (MYWANT_ROBOT_LEARN_WINDOW, a Go duration; a week). Older ones are
// compressed (initiativeCompress): what they came to, kept, in a few rows.
const initiativeWindow = 7 * 24 * time.Hour

func initiativeLearnWindow() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("MYWANT_ROBOT_LEARN_WINDOW")); err == nil && d > 0 {
		return d
	}
	return initiativeWindow
}

// initiativeExample is one labelled decision: what was read, its label, and
// what it was about — the group an old one is compressed into.
type initiativeExample struct {
	State string
	Gold  map[string]string
	At    time.Time
	Group string
}

// initiativeTrainingRows are the labelled decisions as training rows: those
// within the window as they are, the older compressed — and every question
// asked, as last worded.
func initiativeTrainingRows(logPath string, now time.Time) (recent, older []map[string]any, questions map[string]decisionQuestion) {
	labels := initiativeLabels()
	questions = initiativeQuestions(nil)
	questions[offerQuestionID] = offerQuestion
	var examples []initiativeExample
	readJSONLines(logPath, func(rec initiativeRecord) {
		for id, q := range rec.Questions {
			questions[id] = q
		}
		l, ok := labels[rec.ID]
		if !ok || rec.State == "" || len(l.gold()) == 0 {
			return
		}
		at, _ := time.Parse(time.RFC3339, rec.At)
		// An offer: after which want's news, which candidate. The news
		// question: the words read.
		group := "news|" + rec.State
		if rec.Event == "offer" {
			group = "offer|" + rec.WantID + "|" + rec.Did
		}
		examples = append(examples, initiativeExample{State: rec.State, Gold: l.gold(), At: at, Group: group})
	})
	cutoff := now.Add(-initiativeLearnWindow())
	var old []initiativeExample
	for _, e := range examples {
		if e.At.Before(cutoff) {
			old = append(old, e)
			continue
		}
		recent = append(recent, map[string]any{"state": e.State, "gold": e.Gold})
	}
	return recent, initiativeCompress(old), questions
}

// initiativeCompress folds old examples into a few rows: per group and
// question, the label most of them had, in the latest words, as many times as
// log2(1 + how many) — a hundred old replies weigh as about seven, one as one;
// an even split teaches nothing and is left out.
func initiativeCompress(old []initiativeExample) []map[string]any {
	type tally struct {
		counts map[string]int
		state  string
		at     time.Time
	}
	groups := map[string]*tally{}
	var order []string
	for _, e := range old {
		for qid, label := range e.Gold {
			key := e.Group + "|" + qid
			t, ok := groups[key]
			if !ok {
				t = &tally{counts: map[string]int{}}
				groups[key] = t
				order = append(order, key)
			}
			t.counts[label]++
			if !e.At.Before(t.at) {
				t.state, t.at = e.State, e.At
			}
		}
	}
	var rows []map[string]any
	for _, key := range order {
		t := groups[key]
		qid := key[strings.LastIndex(key, "|")+1:]
		best, bestN, total, tie := "", 0, 0, false
		for label, n := range t.counts {
			total += n
			switch {
			case n > bestN:
				best, bestN, tie = label, n, false
			case n == bestN:
				tie = true
			}
		}
		if tie {
			continue
		}
		copies := int(math.Round(math.Log2(1 + float64(total))))
		for range max(1, copies) {
			rows = append(rows, map[string]any{"state": t.state, "gold": map[string]string{qid: best}})
		}
	}
	return rows
}

// robotRetrain has the heads trained again on the labels, in the background
// — by a device that can, else the Laya service here; false when neither is
// there or a training is under way.
func (s *Server) robotRetrain(ri *robotInitiative, labels int) bool {
	d := ri.decider
	if d == nil || !modelDeviceCan(modelOpTrainDecider, "", time.Now()) {
		return false
	}
	if !initiativeTraining.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer initiativeTraining.Store(false)
		recent, older, questions := initiativeTrainingRows(ri.logPath, time.Now())
		rows := initiativeTrainingSet(recent, older, s.offerSeedRows())
		log.Printf("[initiative] retraining on %d recent replies, %d rows from older ones (%d rows with the made ones)",
			len(recent), len(older), len(rows))
		started := time.Now()
		report, by, err := d.train(rows, questions)
		if err != nil {
			log.Printf("[initiative] retrain failed (%s): %v", by, err)
			return
		}
		t := initiativeTrained{Labels: labels, At: time.Now().Format(time.RFC3339), Report: report}
		raw, _ := json.MarshalIndent(t, "", " ")
		_ = os.WriteFile(initiativeHomeFile("robot-initiative-trained.json"), raw, 0o644)
		log.Printf("[initiative] retrained by %s in %s: %s", by, time.Since(started).Round(time.Second), string(report))
	}()
	return true
}

// initiativeStatus is where the learning stands.
func (s *Server) initiativeStatus() map[string]any {
	decisions := 0
	if s.initiative != nil {
		readJSONLines(s.initiative.logPath, func(initiativeRecord) { decisions++ })
	}
	labels := len(initiativeLabels())
	last := initiativeLastTrained()
	return map[string]any{
		"decisions":       decisions,
		"labels":          labels,
		"trainedOnLabels": last.Labels,
		"trainedAt":       last.At,
		"lastReport":      last.Report,
		"retrainEvery":    initiativeRetrainCount(),
		"untilRetrain":    max(0, initiativeRetrainCount()-(labels-last.Labels)),
		"training":        initiativeTraining.Load(),
		"labelAfter":      fmt.Sprint(initiativeLabelWait()),
	}
}
