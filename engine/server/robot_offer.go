package server

// The robot offering what it can do next.
//
// After it tells the person something (report), the robot looks at what it
// could do about it — its capabilities on the board: what the want it just
// read can look up (the type's own capabilities), and running again the other
// wants that find things (every want can restart) — and asks Laya, for each,
// whether the person would take it up, in the words of the situation and the
// candidate ("提案: 「NakanoのWeather」を最新にする"). The likeliest, when likely
// enough (MYWANT_ROBOT_OFFER_MIN, 0.5), it offers: 「〜しましょうか？」 with
// はい / いいえ beside it in the chat.
//
// Laya chooses only among what is offered here, and what is offered comes
// from the board: a type or capability added (a custom) is a candidate the
// next time, with its own description as the words Laya reads — the robot's
// repertoire grows with the board, with no change here.
//
// The reply is the person's next question (はい, お願い, いいえ…) — answered
// here, before any model is asked (planBrief → planBriefResponse.Answered):
// yes runs it, and what it finds may be news the robot tells and makes its
// next offer about. Every offer is kept with the decisions; the reply is its
// label (accept / skip — no reply is skip), so what the person takes up is
// what the robot learns to offer.

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	mywant "mywant/engine/core"
)

const (
	offerQuestionID = "offer"
	// offerOtherWants is how many other wants' restarts are candidates.
	offerOtherWants = 8
	// offerTTL is how long an offer waits for its reply.
	offerTTL = 10 * time.Minute
)

// offerQuestion is asked of each candidate — the same question every time, so
// a head fitted on the replies serves candidates it never saw: the candidate
// is in the state, not in the labels.
var offerQuestion = decisionQuestion{
	Type:         "choice",
	Instructions: "ロボットがこの提案をしたら、本人は受けるか",
	Criteria: map[string]string{
		"accept": "受ける。今それをしてほしい",
		"skip":   "断る。今は必要ない",
	},
}

func offerMin() float64 {
	if v, err := strconv.ParseFloat(os.Getenv("MYWANT_ROBOT_OFFER_MIN"), 64); err == nil && v > 0 {
		return v
	}
	return 0.5
}

// offerCandidate is one thing the robot could offer to do.
type offerCandidate struct {
	Key  string // restart:<want> | cap:<want>#<capability>
	Want string
	Text string // what Laya reads
	Ask  string // what the robot says
	run  func(s *Server) (string, error)
}

type pendingOffer struct {
	ID    string // the offer's decision record
	Cand  offerCandidate
	At    time.Time
	Asked string
	// Injected: made after an event handed in through the API — a reply
	// still teaches, silence does not (nobody may have been there).
	Injected bool
}

var robotOffers struct {
	sync.Mutex
	pending *pendingOffer
}

// offerCandidates are what the robot could do after telling sit.Event.
func (s *Server) offerCandidates(sit initiativeSituation) []offerCandidate {
	var out []offerCandidate
	// What the want it just read can look up.
	for _, c := range s.boardCapabilities() {
		if c.want.Metadata.ID != sit.Event.WantID {
			continue
		}
		c := c
		what := strings.TrimSpace(strings.SplitN(c.def.Description, "—", 2)[0])
		out = append(out, offerCandidate{
			Key:  "cap:" + c.ref(),
			Want: c.want.Metadata.Name,
			Text: fmt.Sprintf("「%s」で、%s", c.want.Metadata.Name, firstLine(what, 60)),
			Ask:  fmt.Sprintf("「%s」で、%s。調べましょうか？", c.want.Metadata.Name, firstLine(what, 40)),
			run: func(s *Server) (string, error) {
				return s.useCapability(map[string]string{"capability": c.ref()})
			},
		})
	}
	// Running it again (a delay: search again; a failure: try again).
	if w := s.findWantByIDOrName(sit.Event.WantID); w != nil {
		out = append(out, s.offerRestart(w, true))
	}
	// Running again the other wants that find things.
	for _, w := range s.offerFinders(sit.Event.WantID) {
		out = append(out, s.offerRestart(w, false))
	}
	return out
}

// offerRestart is running w again, as a candidate: the same words whether it
// is offered or made up as a starting row (offerSeedRows).
func (s *Server) offerRestart(w *mywant.Want, own bool) offerCandidate {
	name := w.Metadata.Name
	about := ""
	if def := s.wantTypeLoader.GetDefinition(w.Metadata.Type); def != nil {
		about = def.Metadata.Title
		if about == "" {
			about = robotFirstSentence(def.Metadata.Description, 30)
		}
	}
	text := fmt.Sprintf("「%s」をもう一度実行して最新にする", name)
	if about != "" {
		text = fmt.Sprintf("「%s」（%s）をもう一度実行して最新にする", name, about)
	}
	ask := fmt.Sprintf("「%s」も最新にしましょうか？", name)
	if own {
		ask = fmt.Sprintf("「%s」をもう一度調べましょうか？", name)
	}
	return offerCandidate{
		Key: "restart:" + name, Want: name, Text: text, Ask: ask,
		run: func(s *Server) (string, error) {
			return s.useCapability(map[string]string{"capability": "restart", "want": name})
		},
	}
}

// offerSeeds are made events the offers start from, before the person's
// replies have taught them: which follow-ups fit which news, by the types on
// the board. A golf slot: the reservation and the weather; rain: the routes;
// a delay or a failure: the same want again. Anything else is not offered.
var offerSeeds = []struct {
	eventType, message string
	accept             []string // types whose re-run fits
	own                bool     // running the same want again fits
}{
	{"smartgolf_list_available", "北新宿店の12:00が空いてるよ", []string{"smartgolf_check_reserved", "weather"}, false},
	{"smartgolf_list_available", "中野郵便局前店の15:30が空いたよ", []string{"smartgolf_check_reserved", "weather"}, false},
	{"weather", "午後から雨、傘を持っていって", []string{"transit_search"}, false},
	{"weather", "強風注意報が出ています", []string{"transit_search"}, false},
	{"transit_search", "中央線が遅延中、15分遅れ", nil, true},
	{"transit_search", "運転見合わせ、振替輸送あり", nil, true},
	{"backup to google", "バックアップに失敗しました", nil, true},
}

// offerSeedRows are the offer's starting rows, made from what is on the board
// now so their words are the words offered.
func (s *Server) offerSeedRows() []map[string]any {
	if s.globalBuilder == nil || s.wantTypeLoader == nil {
		return nil
	}
	finders := s.offerFinders("")
	var rows []map[string]any
	for _, seed := range offerSeeds {
		var event *mywant.Want
		for _, w := range s.globalBuilder.GetWants() {
			if w.Metadata.Type == seed.eventType && w.GetLabels()[robotArchived] != "true" {
				event = w
				break
			}
		}
		if event == nil {
			continue
		}
		text := fmt.Sprintf("出来事: wantの新しい結果。want: %s。結果: %s。", event.Metadata.Name, seed.message)
		add := func(c offerCandidate, fits bool) {
			gold := "skip"
			if fits {
				gold = "accept"
			}
			rows = append(rows, map[string]any{"state": text + "提案: " + c.Text + "。",
				"gold": map[string]string{offerQuestionID: gold}})
		}
		add(s.offerRestart(event, true), seed.own)
		for _, w := range finders {
			if w.Metadata.ID == event.Metadata.ID {
				continue
			}
			fits := false
			for _, t := range seed.accept {
				fits = fits || w.Metadata.Type == t
			}
			add(s.offerRestart(w, false), fits)
		}
	}
	return rows
}

// offerFinders are the wants on the board that find things (their type keeps
// a result), but the one just told — those with news lately first.
func (s *Server) offerFinders(except string) []*mywant.Want {
	if s.globalBuilder == nil || s.wantTypeLoader == nil {
		return nil
	}
	recent := map[string]int{}
	if s.notifications != nil {
		for i, n := range s.notifications.All(200) {
			if _, seen := recent[n.TargetID]; !seen && n.TargetType == "want" {
				recent[n.TargetID] = i
			}
		}
	}
	var out []*mywant.Want
	for _, w := range s.globalBuilder.GetWants() {
		if w.Metadata.ID == except || characterWantTypes[w.Metadata.Type] || w.Metadata.Name == robotWantName {
			continue
		}
		l := w.GetLabels()
		if l[robotArchived] == "true" || l[robotCanvasX] == "" {
			continue
		}
		if def := s.wantTypeLoader.GetDefinition(w.Metadata.Type); def == nil || def.FinalResultField == "" {
			continue
		}
		out = append(out, w)
	}
	rank := func(w *mywant.Want) int {
		if i, ok := recent[w.Metadata.ID]; ok {
			return i
		}
		return 1 << 20
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	if len(out) > offerOtherWants {
		out = out[:offerOtherWants]
	}
	return out
}

// robotOffer asks Laya about each candidate and, when one is likely enough,
// offers it. Called after the robot has told sit.Event.
func (s *Server) robotOffer(ri *robotInitiative, sit initiativeSituation) {
	cands := s.offerCandidates(sit)
	if len(cands) == 0 {
		return
	}
	base := initiativeEventText(sit) // words, as the news question reads them
	items := make([]decisionItem, len(cands))
	for i, c := range cands {
		items[i] = decisionItem{State: base + "提案: " + c.Text + "。",
			Questions: map[string]decisionQuestion{offerQuestionID: offerQuestion}}
	}
	started := time.Now()
	answers, by, err := ri.decider.decideMany(items)
	if err != nil {
		if err != errNoModel {
			log.Printf("[offer] no decision (%v)", err)
		}
		return
	}
	best, bestP := -1, 0.0
	for i, a := range answers {
		if p := a[offerQuestionID].Probabilities["accept"]; p > bestP {
			best, bestP = i, p
		}
	}
	log.Printf("[offer] %d candidates by %s in %s; best %v at %.2f", len(cands), by,
		time.Since(started).Round(time.Millisecond), func() string {
			if best < 0 {
				return "-"
			}
			return cands[best].Key
		}(), bestP)
	if best < 0 || bestP < offerMin() {
		return
	}
	c := cands[best]
	rec := initiativeRecord{
		ID: fmt.Sprintf("offer-%d", time.Now().UnixNano()), WantID: sit.Event.WantID,
		At: time.Now().Format(time.RFC3339), Event: "offer", Want: c.Want,
		State: items[best].State, Questions: items[best].Questions, By: by,
		Answers: answers[best], Did: c.Key, Result: c.Ask, Injected: sit.Event.Injected,
	}
	for _, x := range cands {
		rec.Offered = append(rec.Offered, x.Key)
	}
	ri.keep(rec)
	robotOffers.Lock()
	robotOffers.pending = &pendingOffer{ID: rec.ID, Cand: c, At: time.Now(), Asked: c.Ask, Injected: sit.Event.Injected}
	robotOffers.Unlock()
	// In the chat with its replies beside it, and said (sayTurnAnswer: the
	// bubble and the conversation) — the bubble keeps it, with the replies as
	// buttons, while it waits (robot_offer).
	s.showOffer(rec.ID, c.Ask)
	s.sayTurnAnswer(c.Ask, nil, robotYesNo)
}

// showOffer puts the offer waiting in the robot's state, where the board's
// bubble reads it; showOffer("", "") takes it down.
func (s *Server) showOffer(id, ask string) {
	robot, err := s.robotWantNow()
	if err != nil {
		return
	}
	offer := map[string]any{}
	if id != "" {
		offer = map[string]any{"id": id, "ask": ask, "choices": robotYesNo, "at": time.Now().Format(time.RFC3339)}
	}
	robot.SetCurrent("robot_offer", offer)
}

var (
	offerYes = []string{"はい", "うん", "お願い", "おねがい", "頼む", "たのむ", "やって", "いいよ", "ok", "yes", "よろしく", "そうして"}
	offerNo  = []string{"いいえ", "いや", "いらない", "結構", "けっこう", "やめて", "大丈夫", "no", "不要", "しない"}
)

// offerReplyKind says whether said is a yes or a no to an offer ("" neither):
// short, and starting with one of the words.
func offerReplyKind(said string) string {
	t := strings.ToLower(strings.TrimSpace(strings.Trim(said, "。！!、,. ")))
	if t == "" || len([]rune(t)) > 12 {
		return ""
	}
	for _, w := range offerNo { // 「いいえ」 before 「いいよ」
		if strings.HasPrefix(t, w) {
			return "skip"
		}
	}
	for _, w := range offerYes {
		if strings.HasPrefix(t, w) {
			return "accept"
		}
	}
	return ""
}

// robotOfferReply answers said when it replies to the offer waiting: runs it
// on yes, and labels the offer either way. false when said is no such reply.
func (s *Server) robotOfferReply(said string) (string, bool) {
	kind := offerReplyKind(said)
	if kind == "" {
		return "", false
	}
	robotOffers.Lock()
	p := robotOffers.pending
	if p == nil || time.Since(p.At) > offerTTL {
		robotOffers.Unlock()
		return "", false
	}
	robotOffers.pending = nil
	robotOffers.Unlock()
	s.showOffer("", "")
	_ = appendJSONLine(initiativeHomeFile("robot-initiative-labels.jsonl"), initiativeLabel{
		ID: p.ID, At: time.Now().Format(time.RFC3339), By: "person",
		Gold: map[string]string{offerQuestionID: kind}, Reason: "the person answered the offer: " + said,
	})
	if kind == "skip" {
		log.Printf("[offer] %s declined", p.Cand.Key)
		return "わかりました。やめておきます。", true
	}
	out, err := p.Cand.run(s)
	if err != nil {
		log.Printf("[offer] %s failed: %v", p.Cand.Key, err)
		return fmt.Sprintf("「%s」でうまくいきませんでした：%v", p.Cand.Want, err), true
	}
	log.Printf("[offer] %s accepted", p.Cand.Key)
	if strings.HasPrefix(p.Cand.Key, "restart:") {
		return fmt.Sprintf("「%s」をもう一度実行しました。新しい結果が出たらお知らせします。", p.Cand.Want), true
	}
	return fmt.Sprintf("「%s」で調べました。%s", p.Cand.Want, firstLine(out, 160)), true
}

// robotOfferExpire labels an offer nobody answered as skipped, once it is old;
// true when one went.
func robotOfferExpire(now time.Time) bool {
	robotOffers.Lock()
	p := robotOffers.pending
	if p == nil || now.Sub(p.At) <= offerTTL {
		robotOffers.Unlock()
		return false
	}
	robotOffers.pending = nil
	robotOffers.Unlock()
	if p.Injected {
		return true
	}
	_ = appendJSONLine(initiativeHomeFile("robot-initiative-labels.jsonl"), initiativeLabel{
		ID: p.ID, At: now.Format(time.RFC3339), By: "outcome",
		Gold: map[string]string{offerQuestionID: "skip"}, Reason: "no reply to the offer",
	})
	return true
}
