package server

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// What the robot would offer now, as pills in its chat when the chat is
// opened — the browser's, the phone's, the Mac's alike:
//
//	GET  /api/v1/robot/suggestions[?fresh=1][&max=6] → {suggestions: [{id, pill, ask, p}], by}
//	POST /api/v1/robot/suggestions/{id}   → {pill}: the pill pressed, noted
//
// A pill pressed is said in the chat as its words, as if typed: the client
// sends them as any question, and the robot's chat does the rest as it does
// for anything asked — the plan, what is done, the answer in its words, the
// wants it is about as cards. Nothing here does it.
//
// The candidates are the offers' own (robot_offer.go) with no news to follow:
// the way to where the person goes (offerHomes, every part of the day) and
// running again the wants that find things. Each is read by Laya with the
// time and where the person is, as an offer is, after "the chat was opened";
// the three likeliest it would take up are the pills. Without Laya, by the
// rules: the home of this part of the day first, then the wants with news
// lately.
//
// A pill pressed is a yes to it and a no to the others shown with it, kept as
// the offer's replies are (robot-initiative-labels.jsonl). A pill not pressed
// teaches nothing: the chat may have been opened for something else.

const (
	suggestEvent = "出来事: ロボットとの会話を開いた。"
	suggestMax   = 3 // the chat's pills; asked for more with ?max= (the widget's cards)
	// suggestMaxAll is the most worked out, and shown on any one ask.
	suggestMaxAll = 6
	// suggestFresh is how long the pills worked out stand for the same
	// situation: a chat opened and closed again is not asked of Laya twice.
	suggestFresh = 2 * time.Minute
	// suggestKept is how long a pill shown can still be pressed: a widget
	// shows what it fetched a while ago (mywant-ios MyWantWidgets).
	suggestKept = time.Hour
	// suggestFloor: a candidate less likely than this is not shown even to
	// make up the chat's pills.
	suggestFloor = 0.1
)

type robotSuggestion struct {
	ID   string  `json:"id"`
	Pill string  `json:"pill"`
	Ask  string  `json:"ask"`
	P    float64 `json:"p,omitempty"`
	// The want it is about (none when it makes one) and its type: a client
	// draws the pill as it draws that want, its icon and colour.
	WantID string `json:"wantId,omitempty"`
	Type   string `json:"type,omitempty"`
}

type robotSuggestionsView struct {
	Suggestions []robotSuggestion `json:"suggestions"`
	By          string            `json:"by,omitempty"` // laya | rules
	// Wants: the wants worth a look now, each with its latest answer in a
	// line — beside the pills where there is room (the widget's large size).
	Wants []suggestWant `json:"wants"`
}

// suggestWant is a want worth a look now, and what it last found.
type suggestWant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Say  string `json:"say"`
	At   string `json:"at,omitempty"` // when it found it (RFC 3339)
}

// suggestWantsMax is how many wants are offered beside the pills.
const suggestWantsMax = 4

type shownSuggestion struct {
	robotSuggestion
	cand  offerCandidate
	batch string // the pills worked out together
	at    time.Time
	// served: shown to someone (within an ask's max) — only those are a no
	// when another of the batch is pressed.
	served bool
}

var robotSuggests struct {
	sync.Mutex
	state string // the situation they were worked out for
	at    time.Time
	by    string
	shown []shownSuggestion
	// kept: every pill shown in the last suggestKept, which can still be
	// pressed.
	kept []shownSuggestion
}

// suggestBase is the situation as Laya reads it, before the candidate.
func (s *Server) suggestBase(now time.Time) string {
	px, py, ok := 0, 0, false
	if s.globalBuilder != nil {
		px, py, ok = resolveCanvasNear(canvasNearCursor, s.globalBuilder.GetWants())
	}
	tiles, _ := s.robotTiles()
	return suggestEvent + offerNow(now) + offerWhere(px, py, ok, tiles)
}

// pickLikely is the candidates to show, likeliest first: every one Laya finds
// likely (at least min), and when those are fewer than the chat's pills
// (want), the next likeliest to make them up — a chat opened is offered
// something even when nothing is sure — but none below floor.
func pickLikely(ps []float64, min, floor float64, want int) []int {
	order := make([]int, len(ps))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return ps[order[a]] > ps[order[b]] })
	var out []int
	for _, i := range order {
		if ps[i] >= min || (len(out) < want && ps[i] >= floor) {
			out = append(out, i)
		}
	}
	return out
}

// suggestCandidates: running again the wants that find things, and the way
// to where the person goes in place of the same want's plain restart.
func (s *Server) suggestCandidates(now time.Time) []offerCandidate {
	var out []offerCandidate
	for _, w := range s.offerFinders("") {
		out = append(out, s.offerRestart(w, false))
	}
	return withHomes(out, s.offerHomes(newestCursorCharacter(), now))
}

// robotSuggestionsNow is the pills for now, worked out again only when the
// situation has changed or they are stale — or when asked afresh (a ↻).
func (s *Server) robotSuggestionsNow(fresh bool, max int) robotSuggestionsView {
	if max <= 0 {
		max = suggestMax
	}
	if max > suggestMaxAll {
		max = suggestMaxAll
	}
	now := time.Now()
	base := s.suggestBase(now)
	cands := s.suggestCandidates(now)
	wants := s.suggestWants(cands, now)
	robotSuggests.Lock()
	if !fresh && robotSuggests.state == base && now.Sub(robotSuggests.at) < suggestFresh {
		v := suggestView(robotSuggests.shown, robotSuggests.by, max)
		robotSuggests.Unlock()
		v.Wants = wants
		return v
	}
	robotSuggests.Unlock()

	shown, by := s.pickSuggestions(base, cands, now)
	robotSuggests.Lock()
	robotSuggests.state, robotSuggests.at, robotSuggests.by, robotSuggests.shown = base, now, by, shown
	kept := robotSuggests.kept[:0]
	for _, x := range robotSuggests.kept {
		if now.Sub(x.at) < suggestKept {
			kept = append(kept, x)
		}
	}
	robotSuggests.kept = append(kept, shown...)
	robotSuggests.shown = robotSuggests.kept[len(kept):]
	v := suggestView(robotSuggests.shown, by, max)
	robotSuggests.Unlock()
	v.Wants = wants
	return v
}

// suggestWants are the wants worth a look now: the one that goes where the
// person goes at this time of day first, then those that find things the
// person looks at most (wantLookedAt: opened, stood on — their stories) —
// each with an answer to show.
func (s *Server) suggestWants(cands []offerCandidate, now time.Time) []suggestWant {
	out := []suggestWant{}
	seen := map[string]bool{}
	add := func(nameOrID string) {
		if len(out) >= suggestWantsMax {
			return
		}
		w := s.findWantByIDOrName(nameOrID)
		if w == nil || seen[w.Metadata.ID] {
			return
		}
		answers := w.ResultHistoryEntries()
		if len(answers) == 0 {
			return
		}
		last := answers[len(answers)-1]
		seen[w.Metadata.ID] = true
		sw := suggestWant{ID: w.Metadata.ID, Name: w.Metadata.Name, Type: w.Metadata.Type, Say: outputMessage(last)}
		if !last.Timestamp.IsZero() {
			sw.At = last.Timestamp.Format(time.RFC3339)
		}
		out = append(out, sw)
	}
	part := "home:" + dayPart(now) + ":"
	for _, c := range cands {
		if strings.HasPrefix(c.Key, part) {
			add(c.Want)
		}
	}
	finders := s.boardFinders("")
	score := wantLookedAt(now)
	sort.SliceStable(finders, func(i, j int) bool {
		return score[finders[i].Metadata.ID] > score[finders[j].Metadata.ID]
	})
	for _, w := range finders {
		if score[w.Metadata.ID] <= 0 {
			break // never looked at: not worth a look for that
		}
		add(w.Metadata.ID)
	}
	// Too few looked at: those that told something lately, as before.
	for _, w := range s.offerFinders("") {
		add(w.Metadata.ID)
	}
	return out
}

// suggestView is the first max of the pills, marked served. Called with
// robotSuggests held: shown is robotSuggests' own.
func suggestView(shown []shownSuggestion, by string, max int) robotSuggestionsView {
	v := robotSuggestionsView{Suggestions: []robotSuggestion{}, By: by, Wants: []suggestWant{}}
	for i := range shown {
		if i >= max {
			break
		}
		shown[i].served = true
		v.Suggestions = append(v.Suggestions, shown[i].robotSuggestion)
	}
	return v
}

// pickSuggestions asks Laya about each candidate and keeps the likeliest
// it would take up; by the rules when there is no Laya to ask.
func (s *Server) pickSuggestions(base string, cands []offerCandidate, now time.Time) ([]shownSuggestion, string) {
	if len(cands) == 0 {
		return nil, ""
	}
	ri := s.initiative
	items := make([]decisionItem, len(cands))
	for i, c := range cands {
		items[i] = decisionItem{State: base + "提案: " + c.Text + "。",
			Questions: map[string]decisionQuestion{offerQuestionID: offerQuestion}}
	}
	type scored struct {
		i int
		p float64
	}
	var picked []scored
	by := "rules"
	var answers []map[string]decisionAnswer
	if ri != nil && ri.decider != nil {
		started := time.Now()
		a, who, err := ri.decider.decideMany(items)
		if err == nil {
			answers, by = a, who
			ps := make([]float64, len(a))
			// What the person looks at most (want_looked_at.go) leans its
			// suggestions up a little.
			looked := wantLookedAt(now)
			for i, x := range a {
				ps[i] = x[offerQuestionID].Probabilities["accept"] + lookBoost(looked[cands[i].WantID])
			}
			for _, i := range pickLikely(ps, offerMin(), suggestFloor, suggestMax) {
				picked = append(picked, scored{i, ps[i]})
			}
			likely := 0
			for _, x := range picked {
				if x.p >= offerMin() {
					likely++
				}
			}
			log.Printf("[suggest] %d candidates by %s in %s; %d likely, %d shown", len(cands), who,
				time.Since(started).Round(time.Millisecond), likely, len(picked))
			if likely == 0 && len(a) > 0 {
				// None likely: the nearest misses, so why none is not a guess.
				order := make([]int, len(a))
				for i := range order {
					order[i] = i
				}
				pOf := func(i int) float64 { return a[i][offerQuestionID].Probabilities["accept"] }
				sort.SliceStable(order, func(x, y int) bool { return pOf(order[x]) > pOf(order[y]) })
				var near []string
				for _, i := range order[:min(3, len(order))] {
					near = append(near, fmt.Sprintf("%s=%.2f", cands[i].Key, pOf(i)))
				}
				log.Printf("[suggest] none at %.2f or more; likeliest: %s; asked as: %s", offerMin(),
					strings.Join(near, ", "), strings.SplitN(items[0].State, "提案:", 2)[0])
			}
		} else if err != errNoModel {
			log.Printf("[suggest] no decision (%v); by the rules", err)
		}
	}
	if answers == nil {
		for _, i := range suggestByRules(cands, now) {
			picked = append(picked, scored{i, 0})
		}
		// What the person looks at most first, the rules' order otherwise.
		looked := wantLookedAt(now)
		sort.SliceStable(picked, func(x, y int) bool {
			return looked[cands[picked[x].i].WantID] > looked[cands[picked[y].i].WantID]
		})
	}
	if len(picked) > suggestMaxAll {
		picked = picked[:suggestMaxAll]
	}
	var shown []shownSuggestion
	batch := fmt.Sprintf("suggest-%d", now.UnixNano())
	for _, x := range picked {
		c := cands[x.i]
		pill := c.Pill
		if pill == "" {
			pill = strings.TrimSuffix(c.Ask, "？")
		}
		rec := initiativeRecord{
			ID: fmt.Sprintf("%s-%d", batch, x.i), At: now.Format(time.RFC3339),
			Event: "suggest", Want: c.Want, State: items[x.i].State, Questions: items[x.i].Questions,
			By: by, Did: c.Key, Result: pill,
		}
		if answers != nil {
			rec.Answers = answers[x.i]
		}
		if ri != nil {
			ri.keep(rec)
		}
		shown = append(shown, shownSuggestion{
			robotSuggestion: robotSuggestion{ID: rec.ID, Pill: pill, Ask: c.Ask, P: x.p, WantID: c.WantID, Type: c.Type},
			cand:            c, batch: batch, at: now,
		})
	}
	return shown, by
}

// suggestByRules orders the candidates without Laya: the home of this part
// of the day, then the wants that find things as offerFinders ranked them
// (news lately first). Homes of other parts of the day are left out.
func suggestByRules(cands []offerCandidate, now time.Time) []int {
	part := "home:" + dayPart(now) + ":"
	var homes, rest []int
	for i, c := range cands {
		switch {
		case strings.HasPrefix(c.Key, part):
			homes = append(homes, i)
		case strings.HasPrefix(c.Key, "home:"):
		default:
			rest = append(rest, i)
		}
	}
	return append(homes, rest...)
}

// takeSuggestion notes the pill pressed — a yes to it and a no to the rest
// shown beside it — and hands its words back for the client to say in the
// chat as the person's. It does nothing itself: the chat does what is said.
func (s *Server) takeSuggestion(id string) (string, bool) {
	robotSuggests.Lock()
	var taken *shownSuggestion
	for i, x := range robotSuggests.kept {
		if x.ID == id && time.Since(x.at) < suggestKept {
			taken = &robotSuggests.kept[i]
			break
		}
	}
	if taken == nil {
		robotSuggests.Unlock()
		return "", false
	}
	t := *taken
	taken = &t
	// Each batch is answered once: its pills go, and the next look works
	// them out afresh.
	var others []shownSuggestion
	kept := robotSuggests.kept[:0]
	for _, x := range robotSuggests.kept {
		switch {
		case x.batch != taken.batch:
			kept = append(kept, x)
		case x.ID != taken.ID && x.served:
			others = append(others, x)
		}
	}
	robotSuggests.kept = kept
	robotSuggests.shown, robotSuggests.state = nil, ""
	robotSuggests.Unlock()
	at := time.Now().Format(time.RFC3339)
	labels := initiativeHomeFile("robot-initiative-labels.jsonl")
	_ = appendJSONLine(labels, initiativeLabel{ID: taken.ID, At: at, By: "person",
		Gold: map[string]string{offerQuestionID: "accept"}, Reason: "the person pressed the pill: " + taken.Pill})
	for _, o := range others {
		_ = appendJSONLine(labels, initiativeLabel{ID: o.ID, At: at, By: "person",
			Gold: map[string]string{offerQuestionID: "skip"}, Reason: "another pill was pressed: " + taken.Pill})
	}
	return taken.Pill, true
}
