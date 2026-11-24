package server

// How a time in a request is read — 「10分後に国分寺から中野坂上」 — by Laya.
//
// The phone's model was asked to fill a route's time with the time now beside
// it, and left 「10分後」 out: the route was searched for now. Arithmetic is
// not a language model's to do, and Laya does none: it is asked which way the
// request's time is to be read — one of a fixed list of readings, each said
// in words (今から N 分後, 今日の H 時 M 分, …) — and whether it is a time to
// leave or to arrive. The reading it chooses is then worked out here, on the
// numbers the request has: 19:33 and 「10分後」 read as 今から N 分後 is 19:43.
//
// The readings are a fixed list, so the question is always the same one and
// a head trained on it (train-decider) keeps serving. Laya never sees a time
// it would have to judge by its value: it learns how a request is to be read,
// not which times come up. A reading that cannot be worked out on this
// request (no number of its kind, a time today already gone) gives way to
// the next likeliest.
//
// A want type asks for it by a label on the time parameter:
//
//	time-reading/<time param>: "<side param>=<leave value>|<arrive value>"
//
// transit_search's is time-reading/time: "arrive_type=出発|到着". The side
// part may be left out (a type whose time has no side). What Laya reads, when
// it is sure enough, is what is set — over what the model sent; when no
// worker answers or it is unsure, the model's stays as it was.
//
// Every reading is said as a line in the robot's chat (laya.params) and kept,
// one JSON line, in ~/.mywant/laya-params.jsonl — the rows to teach Laya with.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const timeReadingLabel = "time-reading/"

// timeReadingMinP is how sure Laya must be of a reading for it to be set.
const timeReadingMinP = 0.25

// timeReadingSideMinP is how sure of leave or arrive, else the model's (or,
// none sent, leaving).
const timeReadingSideMinP = 0.5

// A reading: what Laya is shown (its words), and how it is worked out on the
// request's numbers — ok false when this request has nothing it could read.
type timeReading struct {
	ID    string
	Words string
	Work  func(n timeNumbers, now time.Time) (string, bool)
}

// timeNumbers is what a request says of time, in numbers.
type timeNumbers struct {
	// A clock time: 9時40分, 9時半, 9:40, 午後3時.
	ClockH, ClockM int
	HasClock, PM   bool
	// A length of time: 2時間, 1時間30分.
	DurH, DurM int
	HasDurH    bool
	// Minutes said alone: 10分 (not the minutes of a clock time or a length).
	Min    int
	HasMin bool
}

func hhmm(t time.Time) string { return t.Format("15:04") }

// clockToday is the clock time said, as a time today (a 午後 or 夜 moving an
// hour before noon after it).
func clockToday(n timeNumbers, now time.Time) (time.Time, bool) {
	if !n.HasClock || n.ClockH > 24 || n.ClockM > 59 {
		return time.Time{}, false
	}
	h := n.ClockH
	if n.PM && h < 12 {
		h += 12
	}
	return time.Date(now.Year(), now.Month(), now.Day(), h%24, n.ClockM, 0, 0, now.Location()), true
}

// timeReadings are the ways a request's time can be read. The IDs are the
// labels Laya answers with and is trained on: keep them, and their words, as
// they are — a change of either is a new question with no head.
var timeReadings = []timeReading{
	{ID: "none", Words: "時刻の指定はない（今すぐ）", Work: func(timeNumbers, time.Time) (string, bool) { return "", true }},
	{ID: "after_min", Words: "今から N 分後", Work: func(n timeNumbers, now time.Time) (string, bool) {
		if !n.HasMin || n.Min <= 0 {
			return "", false
		}
		return hhmm(now.Add(time.Duration(n.Min) * time.Minute)), true
	}},
	{ID: "after_hour", Words: "今から N 時間（M 分）後", Work: func(n timeNumbers, now time.Time) (string, bool) {
		if !n.HasDurH {
			return "", false
		}
		return hhmm(now.Add(time.Duration(n.DurH)*time.Hour + time.Duration(n.DurM)*time.Minute)), true
	}},
	{ID: "at_today", Words: "今日の H 時（M 分）", Work: func(n timeNumbers, now time.Time) (string, bool) {
		t, ok := clockToday(n, now)
		// A time today already gone is not what was meant (tomorrow's is its
		// own reading).
		if !ok || t.Before(now.Add(-5*time.Minute)) {
			return "", false
		}
		return hhmm(t), true
	}},
	{ID: "at_tomorrow", Words: "明日の H 時（M 分）", Work: func(n timeNumbers, now time.Time) (string, bool) {
		t, ok := clockToday(n, now)
		if !ok {
			return "", false
		}
		return hhmm(t), true
	}},
	{ID: "this_hour_min", Words: "この時間の M 分（何時かは言わない）", Work: func(n timeNumbers, now time.Time) (string, bool) {
		if !n.HasMin || n.Min > 59 {
			return "", false
		}
		t := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), n.Min, 0, 0, now.Location())
		if t.Before(now.Add(-5 * time.Minute)) {
			return "", false
		}
		return hhmm(t), true
	}},
}

var (
	trDurH  = regexp.MustCompile(`(\d+)\s*時間(?:\s*(\d+)\s*分|(半))?`)
	trClock = regexp.MustCompile(`(\d{1,2})\s*時(?:\s*(\d{1,2})\s*分|(半))?`)
	trColon = regexp.MustCompile(`(\d{1,2})[:：](\d{2})`)
	trMin   = regexp.MustCompile(`(\d+)\s*分`)
	trWide  = strings.NewReplacer("０", "0", "１", "1", "２", "2", "３", "3", "４", "4", "５", "5", "６", "6", "７", "7", "８", "8", "９", "9")
	trKanji = regexp.MustCompile(`[一二三四五六七八九十]+`)
)

// kanjiNumber reads 十, 二十五, 三 as a number; -1 when it is not one.
func kanjiNumber(s string) int {
	digit := map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	n, cur := 0, 0
	for _, r := range s {
		if r == '十' {
			if cur == 0 {
				cur = 1
			}
			n += cur * 10
			cur = 0
			continue
		}
		d, ok := digit[r]
		if !ok {
			return -1
		}
		cur = d
	}
	return n + cur
}

// readTimeNumbers finds what a request says of time in numbers. A length
// (時間) and a clock time (時) are taken out first, so a 分 left is minutes
// said alone.
func readTimeNumbers(q string) timeNumbers {
	q = trWide.Replace(q)
	// 十分後, 二時間 — numbers in kanji, as a voice is often written.
	q = trKanji.ReplaceAllStringFunc(q, func(k string) string {
		if n := kanjiNumber(k); n >= 0 {
			return strconv.Itoa(n)
		}
		return k
	})
	var n timeNumbers
	n.PM = robotClockPM.MatchString(q)
	atoi := func(s string) int { v, _ := strconv.Atoi(s); return v }
	if m := trDurH.FindStringSubmatchIndex(q); m != nil {
		n.HasDurH, n.DurH = true, atoi(q[m[2]:m[3]])
		if m[4] >= 0 {
			n.DurM = atoi(q[m[4]:m[5]])
		} else if m[6] >= 0 {
			n.DurM = 30
		}
		q = q[:m[0]] + " " + q[m[1]:]
	}
	if m := trColon.FindStringSubmatchIndex(q); m != nil {
		n.HasClock, n.ClockH, n.ClockM = true, atoi(q[m[2]:m[3]]), atoi(q[m[4]:m[5]])
		q = q[:m[0]] + " " + q[m[1]:]
	} else if m := trClock.FindStringSubmatchIndex(q); m != nil {
		n.HasClock, n.ClockH = true, atoi(q[m[2]:m[3]])
		if m[4] >= 0 {
			n.ClockM = atoi(q[m[4]:m[5]])
		} else if m[6] >= 0 {
			n.ClockM = 30
		}
		q = q[:m[0]] + " " + q[m[1]:]
	}
	if m := trMin.FindStringSubmatch(q); m != nil {
		n.HasMin, n.Min = true, atoi(m[1])
	}
	return n
}

// timeReadingQuestions are the two questions, the same every time.
func timeReadingQuestions(withSide bool) map[string]decisionQuestion {
	crit := map[string]string{}
	for _, r := range timeReadings {
		crit[r.ID] = r.Words
	}
	qs := map[string]decisionQuestion{
		"time_reading": {Type: "choice", Instructions: "この依頼の時刻はどう読む？", Criteria: crit},
	}
	if withSide {
		qs["time_side"] = decisionQuestion{Type: "choice", Instructions: "この依頼の時刻は出発と到着のどちら？",
			Criteria: map[string]string{"leave": "その時刻に出発する", "arrive": "その時刻までに到着する"}}
	}
	return qs
}

// personNow is now where the person is: MYWANT_TIMEZONE (an IANA name — a
// server on Fly runs in UTC) else the server's own.
func personNow() time.Time {
	now := time.Now()
	if tz := strings.TrimSpace(os.Getenv("MYWANT_TIMEZONE")); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return now.In(loc)
		}
	}
	return now
}

// timeReadingResult is one reading, as it was asked and what came of it.
type timeReadingResult struct {
	At       string             `json:"at"`
	WantType string             `json:"want_type"`
	Question string             `json:"question"`
	Now      string             `json:"now"`
	Param    string             `json:"param"`
	Model    string             `json:"model,omitempty"` // what the model had sent
	Reading  string             `json:"reading,omitempty"`
	Probs    map[string]float64 `json:"probabilities,omitempty"`
	Side     string             `json:"side,omitempty"`
	SideP    map[string]float64 `json:"side_probabilities,omitempty"`
	Set      string             `json:"set,omitempty"` // what was set: a time, "none", or "" (the model's kept)
	By       string             `json:"by,omitempty"`
	Ms       int64              `json:"ms"`
	Error    string             `json:"error,omitempty"`
}

// robotReadTimes asks Laya how the request's time is read for each parameter
// the type marks (time-reading/<param>), and sets what it reads when it is
// sure enough. Says what it set, for the robot to tell; "" when nothing.
func (s *Server) robotReadTimes(typ string, labels map[string]string, params map[string]any, question string) string {
	return s.robotReadTimesAt(typ, labels, params, question, personNow())
}

// robotReadTimesAt is robotReadTimes as if asked at now — an ask tested as
// it was said (laya_asks_check.go).
func (s *Server) robotReadTimesAt(typ string, labels map[string]string, params map[string]any, question string, now time.Time) string {
	if strings.TrimSpace(question) == "" {
		return ""
	}
	var said strings.Builder
	for key, spec := range labels {
		if !strings.HasPrefix(key, timeReadingLabel) {
			continue
		}
		param := strings.TrimPrefix(key, timeReadingLabel)
		sideParam, sideValues := "", []string{}
		if name, vals, ok := strings.Cut(strings.TrimSpace(spec), "="); ok {
			sideParam, sideValues = strings.TrimSpace(name), strings.Split(vals, "|")
		}
		withSide := sideParam != "" && len(sideValues) == 2
		r := timeReadingResult{
			At: time.Now().Format(time.RFC3339), WantType: typ, Question: question,
			Now: now.Format("2006-01-02 15:04"), Param: param, Model: strings.TrimSpace(fmt.Sprint(params[param])),
		}
		if params[param] == nil {
			r.Model = ""
		}
		started := time.Now()
		answers, by, err := (&robotDecider{}).decide(question, timeReadingQuestions(withSide))
		r.Ms, r.By = time.Since(started).Milliseconds(), by
		if err != nil {
			r.Error = err.Error()
			s.noteTimeReading(r)
			continue
		}
		reading := answers["time_reading"]
		r.Reading, r.Probs = reading.Choice, reading.Probabilities
		// The likeliest reading this request can be worked out by.
		ids := make([]string, 0, len(reading.Probabilities))
		for id := range reading.Probabilities {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return reading.Probabilities[ids[i]] > reading.Probabilities[ids[j]] })
		nums := readTimeNumbers(question)
		var chosen *timeReading
		value := ""
		for _, id := range ids {
			if reading.Probabilities[id] < timeReadingMinP {
				break
			}
			for i := range timeReadings {
				if timeReadings[i].ID != id {
					continue
				}
				if v, ok := timeReadings[i].Work(nums, now); ok {
					chosen, value = &timeReadings[i], v
				}
			}
			if chosen != nil {
				break
			}
		}
		if chosen == nil {
			s.noteTimeReading(r)
			continue
		}
		r.Reading = chosen.ID
		if chosen.ID == "none" {
			delete(params, param)
			if withSide {
				delete(params, sideParam)
			}
			r.Set = "none"
			// Only worth saying when the model had put a time there.
			if r.Model != "" {
				fmt.Fprintf(&said, " (%s left out: Laya read no time in the request)", param)
			}
			s.noteTimeReading(r)
			continue
		}
		params[param] = value
		r.Set = value
		if withSide {
			side := answers["time_side"]
			r.Side, r.SideP = side.Choice, side.Probabilities
			v := sideValues[0]
			switch {
			case side.Probabilities[side.Choice] >= timeReadingSideMinP && side.Choice == "arrive":
				v = sideValues[1]
			case side.Probabilities[side.Choice] >= timeReadingSideMinP:
			case strings.TrimSpace(fmt.Sprint(params[sideParam])) != "" && params[sideParam] != nil:
				v = strings.TrimSpace(fmt.Sprint(params[sideParam])) // unsure: the model's
			}
			params[sideParam] = strings.TrimSpace(v)
			fmt.Fprintf(&said, " (%s=%s, %s=%s: Laya read the request's time as %s)", param, value, sideParam, params[sideParam], chosen.Words)
		} else {
			fmt.Fprintf(&said, " (%s=%s: Laya read the request's time as %s)", param, value, chosen.Words)
		}
		s.noteTimeReading(r)
	}
	return said.String()
}

// noteTimeReading says the reading in the robot's chat and keeps it.
func (s *Server) noteTimeReading(r timeReadingResult) {
	fields := map[string]any{
		"want": r.WantType, "param": r.Param, "reading": r.Reading, "p": "", "set": r.Set,
		"model": r.Model, "side": r.Side, "by": r.By, "ms": r.Ms, "error": r.Error,
	}
	if p, ok := r.Probs[r.Reading]; ok {
		fields["p"] = fmt.Sprintf("%.2f", p)
	}
	if r.Set == "" {
		fields["set"] = "kept"
	}
	s.robotChatTrace("laya.params", fields, []string{"want", "param", "reading", "p", "set", "model", "side", "by", "ms", "error"})
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	line, _ := json.Marshal(r)
	f, err := os.OpenFile(filepath.Join(home, ".mywant", "laya-params.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(line, '\n'))
}
