package server

// Teaching Laya to read requests: which magic sentence one is (magic_laya.go),
// how its time is read and whether it is a time to leave or to arrive
// (time_reading.go).
//
// Laya untrained read 「10分後に国分寺から中野坂上に行きたい」's time as
// 今から N 時間後 and 「明日の9時40分までに荻窪から銀座」 as no request at all
// (2026-11-24). Its heads — fitted on Laya's own reading of labelled rows, and
// used only where they read held-out rows better than Laya alone — are what
// make it right. The rows come from the types themselves: each type's asks
// (want-spec asks: what is said, when, and the parameters it is to make),
// said again of other places and numbers; the robot's own (where, how); and
// talk that asks for nothing. Nothing of a type is written here: a type
// added with its asks is taught by the next training. What people correct
// later is to be added to them.
//
//	POST /api/v1/laya/train-readings   starts it (a training runs minutes)
//	GET  /api/v1/laya/train-readings   where it stands, and what it came to
//
// The heads are kept with the others (keepInitiativeHeads, merged). The
// magic sentence's options are the sentences on the board now: a type added
// or taken away is another question, untrained until this runs again.

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	mywant "mywant/engine/core"
)

var layaReadingsTraining atomic.Bool

// layaReadingsTrainedFile is what the last training came to.
func layaReadingsTrainedFile() string { return initiativeHomeFile("laya-readings-trained.json") }

// Things said to the robot that are none of the sentences: talk.
var notRequests = []string{
	"東京から来た友達と話した", "荻窪から歩いてきた", "ありがとう", "こんにちは", "今日は疲れた",
	"銀座で買い物した", "新宿から帰ってきたところ", "おやすみ", "9時に起きた", "昨日は渋谷から電車で帰った",
	"いいね", "そうなんだ", "すごい", "了解", "またあとで", "お腹すいた", "雨が降ってきた",
	"中野で友達に会った", "疲れたから休む", "今日は楽しかった",
}

// Where a thing is (point), and how a want is (look), as asked.
var pointSays = []string{"{x}はどこ？", "{x}どこ", "{x}に連れてって", "{x}の場所を見せて", "{x}ってどこにある？", "{x}はどこにあるの"}
var lookSays = []string{"{y}の天気は？", "{y}はどう？", "{y}の様子は？", "{y}は何て言ってる？", "{y}の予約は？", "{y}どうなった？", "{y}は終わった？"}
var lookNames = []string{"Nakano", "スマートゴルフ", "チェックリスト", "バックアップ", "49ers", "花火", "天気", "タイマー"}

// layaReadingsRows are the rows Laya is taught with, and the questions
// they answer: each type's asks (askRows), the robot's own (where, how), and
// talk that asks for nothing.
func (s *Server) layaReadingsRows() ([]map[string]any, map[string]decisionQuestion) {
	rng := rand.New(rand.NewSource(7))
	places := s.layaPlaces()
	sources, _ := s.magicSources()
	crit, byKey, labelOf := magicLayaOptions(sources)
	questions := timeReadingQuestions(true)
	questions[magicLayaQuestionID] = magicLayaQuestion(crit)
	questions[magicRequestQuestionID] = magicRequestQuestion()

	var rows []map[string]any
	add := func(state string, gold map[string]string) {
		// Anything said that is one of the sentences is a request; talk is
		// said so by its own rows.
		if _, sentence := gold[magicLayaQuestionID]; sentence {
			gold[magicRequestQuestionID] = magicLayaAsk
		}
		// Which sentence, and whether a request, are asked of what was said
		// with its numbers taken out (magicLayaSaid); how its time is read,
		// of it as said. One row each way.
		which, timed := map[string]string{}, map[string]string{}
		for k, v := range gold {
			switch k {
			case magicLayaQuestionID, magicRequestQuestionID:
				which[k] = v
			default:
				timed[k] = v
			}
		}
		if len(which) > 0 {
			said := magicLayaSaid(state)
			rows = append(rows, map[string]any{"state": said, "gold": which,
				"cues": map[string]map[string]float64{magicLayaQuestionID: keywordCues(said, byKey)}})
		}
		if len(timed) > 0 {
			rows = append(rows, map[string]any{"state": state, "gold": timed})
		}
	}
	for key, src := range byKey {
		if src.tool != "" || src.name != "" {
			continue
		}
		for _, r := range s.askRows(src.typ, places, rng) {
			gold := r.gold
			gold[magicLayaQuestionID] = key
			add(r.state, gold)
		}
		// Its aliases said alone, or asked for plainly — 「天気」, 「天気を
		// 出して」: the type, nothing else said. Not written in its asks, so
		// an alias added is taught by the next training.
		for _, a := range cleanAliases(src.aliases) {
			add(a, map[string]string{magicLayaQuestionID: key, "time_reading": "none"})
		}
	}
	// What the person said requests were, asked which of two (magic_resolve
	// .go): taught as said, weighed thrice — a person's word, not a made row.
	readJSONLines(initiativeHomeFile("laya-clarified.jsonl"), func(c struct {
		Say string `json:"say"`
		Key string `json:"key"`
	}) {
		if label, known := labelOf[c.Key]; known && c.Say != "" {
			for range 3 {
				add(c.Say, map[string]string{magicLayaQuestionID: label})
			}
		}
	})
	for _, said := range notRequests {
		add(said, map[string]string{magicRequestQuestionID: magicLayaTalk, "time_reading": "none"})
	}
	// The robot's own: where, and how.
	if label, ok := labelOf["tool:point"]; ok {
		for _, say := range pointSays {
			for range 2 {
				add(strings.ReplaceAll(say, "{x}", places[rng.Intn(len(places))]), map[string]string{magicLayaQuestionID: label})
			}
		}
	}
	if label, ok := labelOf["tool:look"]; ok {
		for _, say := range lookSays {
			for range 2 {
				add(strings.ReplaceAll(say, "{y}", lookNames[rng.Intn(len(lookNames))]), map[string]string{magicLayaQuestionID: label, "time_reading": "none"})
			}
		}
	}
	rng.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	return rows, questions
}

type askRow struct {
	state string
	gold  map[string]string
}

// askRows are a type's asks as rows: what is said, and what of it Laya is to
// read — how its time is read (the reading that works out to the time it is
// to be made with, at the time it was said) and whether that is a time to
// leave or to arrive (its side parameter's value).
func (s *Server) askRows(typ string, places []string, rng *rand.Rand) []askRow {
	def := s.typeDefinition(typ)
	if def == nil || len(def.Asks) == 0 {
		return nil
	}
	param, sideParam, sideValues := timeReadingOf(def.Metadata.Labels)
	aliases := cleanAliases(strings.Split(def.Metadata.Labels[robotAliasLabel], ","))
	// What is learnt is each ask said its other ways (its variants); what is
	// tested on is the ask as said (laya_asks_check.go), so it is tested on
	// words it has not learnt. An ask with no variants is learnt as said.
	var said []mywant.AskDef
	for _, ask := range def.Asks {
		ways := ask.Variants
		if len(ways) == 0 {
			ways = []string{ask.Say}
		}
		for _, w := range ways {
			v := ask
			v.Say = w
			said = append(said, expandAsks([]mywant.AskDef{v}, aliases)...)
		}
	}
	rowOf := func(ask mywant.AskDef, state string) askRow {
		gold := map[string]string{}
		if param != "" {
			if reading, side, ok := askReading(ask, param, sideParam, sideValues); ok {
				gold["time_reading"] = reading
				if side != "" {
					gold["time_side"] = side
				}
			}
		}
		return askRow{state, gold}
	}
	// Every type as many rows (askRowsPerType): one asked many ways is not
	// what every request is taken for, one asked few ways not lost beside it.
	// Fewer are said again of other places and numbers (askVariant).
	var out []askRow
	order := rng.Perm(len(said))
	for i := 0; i < askRowsPerType; i++ {
		ask := said[order[i%len(said)]]
		if i < len(said) {
			out = append(out, rowOf(ask, ask.Say))
		} else {
			out = append(out, rowOf(ask, askVariant(ask, places, rng)))
		}
	}
	return out
}

// askRowsPerType is how many rows each type's asks make.
const askRowsPerType = 12

// timeReadingOf is a type's time-reading label: the time parameter, and its
// side parameter with its leave and arrive values ("" when it has none).
func timeReadingOf(labels map[string]string) (string, string, []string) {
	for key, spec := range labels {
		if !strings.HasPrefix(key, timeReadingLabel) {
			continue
		}
		param := strings.TrimPrefix(key, timeReadingLabel)
		if name, vals, ok := strings.Cut(strings.TrimSpace(spec), "="); ok {
			if v := strings.Split(vals, "|"); len(v) == 2 {
				return param, strings.TrimSpace(name), []string{strings.TrimSpace(v[0]), strings.TrimSpace(v[1])}
			}
		}
		return param, "", nil
	}
	return "", "", nil
}

// askAt is when an ask was said: its At (HH:MM, or a date and time), else
// noon today — a request with no time of its own does not turn on it.
func askAt(at string) time.Time {
	now := personNow()
	at = strings.TrimSpace(at)
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, at, now.Location()); err == nil {
			return t
		}
	}
	if t, err := time.Parse("15:04", at); err == nil {
		return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
}

// askReading is how an ask's time is to be read: the reading that works out,
// on what it says and when, to the time it is to be made with — none when it
// is made with no time. ok false when no reading does (an ask the readings do
// not cover: not taught as any of them).
func askReading(ask mywant.AskDef, param, sideParam string, sideValues []string) (reading, side string, ok bool) {
	want := strings.TrimSpace(fmt.Sprint(ask.Params[param]))
	if ask.Params[param] == nil || want == "" {
		return "none", "", true
	}
	hh, err := robotClock(want)
	if err != nil {
		return "", "", false
	}
	nums := readTimeNumbers(ask.Say)
	at := askAt(ask.At)
	for _, r := range timeReadings {
		if r.ID == "none" {
			continue
		}
		if v, ok := r.Work(nums, at); ok && v == hh {
			reading = r.ID
			break
		}
	}
	if reading == "" {
		return "", "", false
	}
	if sideParam != "" && len(sideValues) == 2 {
		switch strings.TrimSpace(fmt.Sprint(ask.Params[sideParam])) {
		case sideValues[1]:
			side = "arrive"
		case sideValues[0]:
			side = "leave"
		}
	}
	return reading, side, true
}

var askNumber = regexp.MustCompile(`[0-9]+`)

// askVariant is an ask said of other places and other numbers: each of its
// parameters' values that it says, another place; each number, another of
// its size (a minute stays a minute, an hour an hour).
func askVariant(ask mywant.AskDef, places []string, rng *rand.Rand) string {
	said := ask.Say
	for _, v := range ask.Params {
		val := strings.TrimSpace(fmt.Sprint(v))
		if val == "" || !strings.Contains(said, val) || askNumber.MatchString(val) {
			continue
		}
		other := places[rng.Intn(len(places))]
		said = strings.ReplaceAll(said, val, other)
	}
	return askNumber.ReplaceAllStringFunc(said, func(n string) string {
		v, _ := strconv.Atoi(n)
		switch {
		case v >= 13 && v <= 23:
			return strconv.Itoa(13 + rng.Intn(10))
		case v >= 5 && v <= 59 && len(n) == 2:
			return strconv.Itoa([]int{10, 15, 20, 30, 40, 45, 50}[rng.Intn(7)])
		case v >= 1 && v <= 12:
			return strconv.Itoa(1 + rng.Intn(11))
		}
		return n
	})
}

// typeDefinition is a type as it is loaded (nil when it is not).
func (s *Server) typeDefinition(typ string) *mywant.WantTypeDefinition {
	if s.wantTypeLoader == nil {
		return nil
	}
	return s.wantTypeLoader.GetDefinition(typ)
}

// layaPlaces are the stations on the board, or a few when there are none.
func (s *Server) layaPlaces() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range s.thingFulls() {
		if t.Subtype == "station" && !seen[t.Value] {
			seen[t.Value] = true
			out = append(out, t.Value)
		}
	}
	if len(out) < 4 {
		out = append(out, "国分寺", "中野坂上", "荻窪", "銀座", "新宿", "横浜", "渋谷", "戸塚")
	}
	return out
}

// POST starts a training; GET says where it stands.
func (s *Server) handleLayaTrainReadings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		raw, _ := os.ReadFile(layaReadingsTrainedFile())
		robotWriteJSON(w, map[string]any{"training": layaReadingsTraining.Load(), "last": json.RawMessage(orJSON(raw))})
		return
	}
	if !layaCan(modelOpTrainDecider) {
		http.Error(w, "no Laya to train with: no device that trains has asked for work, and this Mac has none", http.StatusConflict)
		return
	}
	// Questions: only these trained (magic_sentence, magic_request,
	// time_reading, time_side) — the heads of the others kept as they are
	// (keepInitiativeHeads merges). None, all of them.
	var req struct {
		Questions []string `json:"questions"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if !layaReadingsTraining.CompareAndSwap(false, true) {
		http.Error(w, "a training is running", http.StatusConflict)
		return
	}
	rows, questions := s.layaReadingsRows()
	if len(req.Questions) > 0 {
		only := map[string]decisionQuestion{}
		for _, id := range req.Questions {
			if q, ok := questions[id]; ok {
				only[id] = q
			}
		}
		questions = only
		// Only the rows that answer one of them.
		var kept []map[string]any
		for _, row := range rows {
			gold, _ := row["gold"].(map[string]string)
			for id := range only {
				if _, has := gold[id]; has {
					kept = append(kept, row)
					break
				}
			}
		}
		rows = kept
	}
	go func() {
		defer layaReadingsTraining.Store(false)
		started := time.Now()
		log.Printf("[laya] training the readings on %d rows", len(rows))
		report, by, err := (&robotDecider{}).train(rows, questions)
		out := map[string]any{"at": time.Now().Format(time.RFC3339), "rows": len(rows), "by": by,
			"seconds": int(time.Since(started).Seconds())}
		if err != nil {
			out["error"] = err.Error()
			log.Printf("[laya] training the readings failed (%s): %v", by, err)
		} else {
			out["report"] = json.RawMessage(report)
			log.Printf("[laya] trained the readings by %s in %s: %s", by, time.Since(started).Round(time.Second), report)
		}
		raw, _ := json.MarshalIndent(out, "", " ")
		_ = os.WriteFile(layaReadingsTrainedFile(), raw, 0o644)
	}()
	robotWriteJSON(w, map[string]any{"started": true, "rows": len(rows)})
}

func orJSON(raw []byte) []byte {
	if json.Valid(raw) && len(raw) > 0 {
		return raw
	}
	return []byte("null")
}

// expandAsks are a type's asks with each <alias> said as each of its aliases
// (expandAlias): one written, as many as it has aliases.
func expandAsks(asks []mywant.AskDef, aliases []string) []mywant.AskDef {
	var out []mywant.AskDef
	for _, a := range asks {
		for _, say := range expandAlias(a.Say, aliases) {
			b := a
			b.Say = say
			out = append(out, b)
		}
	}
	return out
}
