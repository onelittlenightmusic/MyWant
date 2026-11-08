package server

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	mywant "mywant/engine/core"
	"mywant/engine/labels"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Importance (prototype): how much each thing in view matters to the person
// now, read by Laya — the client sends what it has on screen, by id, and paints
// the scores as a heat map over its own view, never touching what it shows.
//
//	POST /api/v1/importance {"context": "...", "items": [{"id": "...", "kind": "want"|"thing"}]}
//	  -> {"items": [{"id", "text", "score", "probabilities"}], "by", "ms"}
//
// score is p(important) + p(normal)/2, in [0, 1]. The words Laya reads are
// built here from the board, so every client asks with ids alone.

const importanceQuestionID = "importance"

var importanceQuestion = decisionQuestion{
	Type:         "choice",
	Instructions: "この項目は、本人が今見るべき重要なものか",
	Criteria: map[string]string{
		"important":   "重要。本人がさっき・今日見に行った、予定や行動に今すぐ関わる、変化や知らせがある、時間が迫っている",
		"normal":      "普通。ときどき見ればよい、いつも通りの内容",
		"unimportant": "不要。しばらく見ていない、今は関係ない、空、終わったもの",
	},
}

type importanceItem struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type importanceResult struct {
	ID            string             `json:"id"`
	Text          string             `json:"text"`
	Score         float64            `json:"score"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Cached        bool               `json:"cached,omitempty"`
	// Stale: kept from before its words changed, within importanceMinInterval.
	Stale bool `json:"stale,omitempty"`
}

// Answers are kept with the item itself, so they outlive the server and every
// client sees the same: a thing's in its labels (mywant.io/importance), a want's in
// a file of the same shape by want id (its metadata is left alone — writing it
// would go through the want). The value is "<score>@<hash>", the hash of the
// exact words Laya read: when the item's facts change (a status, a use), the
// words change, the hash no longer matches and it is asked again; until then
// a pan, a reload or another device asks nothing.
const importanceLabel = "mywant.io/importance"

// importanceVersion is in every hash: changing the question or how an item is
// worded asks everything again.
const importanceVersion = "kv2" // kv2: visits in the criteria and the words

// Opened by New, and not under go test: a test's wants and steps must never
// land in the person's ~/.mywant.
var importanceWantStore *labels.FileStore

func openImportanceStore() {
	if testing.Testing() {
		return
	}
	importanceWantStore = labels.NewFileStore(thingPath("want-importance.yaml"))
}

func importanceHash(state string) string {
	h := fnv.New32a()
	h.Write([]byte(importanceVersion + "\x00" + state))
	return fmt.Sprintf("%08x", h.Sum32())
}

func (s *Server) importanceStore(kind string) *labels.FileStore {
	if kind == "thing" && s.thingLabels != nil {
		return s.thingLabels.FileStore
	}
	if importanceWantStore == nil {
		return labels.NewFileStore("") // reads nothing; a write fails quietly
	}
	return importanceWantStore
}

// importanceMinInterval is how long a score stands once Laya has given it:
// an item whose words change again within it (a want stepped on, a status
// passing through) keeps its last score until then, rather than a Laya read
// at every change. An item never scored is always asked.
const importanceMinInterval = 10 * time.Minute

// storedImportance is the score kept for an item: "<score>@<hash>@<asked at>".
// fresh is whether it was read from these very words.
type storedScore struct {
	score float64
	hash  string
	at    time.Time
}

func (s *Server) storedImportance(it importanceItem) (storedScore, bool) {
	parts := strings.Split(s.importanceStore(it.Kind).Get(it.ID)[importanceLabel], "@")
	if len(parts) < 2 {
		return storedScore{}, false
	}
	f, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return storedScore{}, false
	}
	st := storedScore{score: f, hash: parts[1]}
	if len(parts) > 2 {
		st.at, _ = time.Parse(time.RFC3339, parts[2])
	}
	return st, true
}

// Stepped on: a want a character has just stood on is one the person is
// looking at now — kept as its last-visited, and read by Laya (visited).
const importanceVisitedLabel = "mywant.io/last-visited"

func noteWantVisited(wantID string) {
	if importanceWantStore == nil {
		return
	}
	_ = importanceWantStore.Set(wantID, importanceVisitedLabel, time.Now().Format(time.RFC3339))
}

// Laya reads a visit in two states only: visited:さっき within visitedRecent of
// the last step on it, nothing after. Tried finer (今日, 最近, しばらく前), Laya
// did not keep them in order; さっき against nothing it did, every time — with
// the criteria naming it (本人がさっき・今日見に行った as important). Two states
// also mean a visit asks Laya at most twice: when it comes, when it passes.
const visitedRecent = time.Hour

func (s *Server) visitedRecently(wantID string) bool {
	t, err := time.Parse(time.RFC3339, s.importanceStore("want").Get(wantID)[importanceVisitedLabel])
	return err == nil && time.Since(t) < visitedRecent
}

// What Laya reads of an item: not only its name, but what it is for. A want
// says its type's purpose, what it was asked (its parameters) and how it
// stands; a thing says which wants use it, as what, and how those stand — a
// time alone ("10:36") means nothing, the departure time of a finished route
// search does.
func (s *Server) importanceText(it importanceItem) string {
	switch it.Kind {
	case "thing":
		for _, t := range s.thingFulls() {
			if t.ID == it.ID {
				return s.importanceThingText(t)
			}
		}
	default:
		if w := s.findWantByIDInAll(it.ID); w != nil {
			return s.importanceWantText(w)
		}
	}
	return ""
}

// The status as the person would say it, with what it means for looking.
var importanceStatusWords = map[mywant.WantStatus]string{
	mywant.WantStatusIdle:                "準備中",
	mywant.WantStatusInitializing:        "準備中",
	mywant.WantStatusPrepareAgent:        "準備中",
	mywant.WantStatusReaching:            "動いている",
	mywant.WantStatusReachingWithWarning: "動いている",
	mywant.WantStatusAchieved:            "完了",
	mywant.WantStatusAchievedWithWarning: "完了",
	mywant.WantStatusSuspended:           "一時停止中",
	mywant.WantStatusWaitingUserAction:   "本人の返事待ち",
	mywant.WantStatusFailed:              "失敗",
	mywant.WantStatusConfigError:         "設定の誤りで止まっている",
	mywant.WantStatusModuleError:         "不具合で止まっている",
	mywant.WantStatusCancelled:           "取り消し済み",
	mywant.WantStatusTerminated:          "終了",
	mywant.WantStatusArchived:            "しまわれた",
}

func importanceStatus(st mywant.WantStatus) string {
	if w, ok := importanceStatusWords[st]; ok {
		return w
	}
	return string(st)
}

func clipRunes(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// The want's type, by the person's word for it (its first alias, else its
// title), and the first sentence of what it does.
func (s *Server) importanceTypePhrase(typ string) (word, purpose string) {
	word = typ
	if s.wantTypeLoader == nil {
		return
	}
	def := s.wantTypeLoader.GetDefinition(typ)
	if def == nil {
		return
	}
	if a := def.Metadata.Labels["aliases"]; a != "" {
		word = strings.Split(a, ",")[0]
	} else if def.Metadata.Title != "" {
		word = def.Metadata.Title
	}
	d := def.Metadata.Description
	if k := strings.IndexAny(d, ".。\n"); k > 0 {
		d = d[:k]
	}
	return word, clipRunes(d, 60)
}

func wantDisplayName(w *mywant.Want) string {
	if w.Metadata.Name != "" {
		return w.Metadata.Name
	}
	return w.Metadata.ID
}

// Laya reads an item as key:value pairs on one line: no sentence around them,
// no JSON punctuation — about half the tokens of a sentence for the same facts.
type kvLine []string

func (l *kvLine) add(k, v string) {
	if v = strings.TrimSpace(v); v != "" {
		*l = append(*l, k+":"+v)
	}
}
func (l kvLine) String() string { return strings.Join(l, " ") }

// How lately, in three steps: finer would change the words (and ask again)
// every day.
func agoWords(rfc string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return ""
	}
	switch d := now.Sub(t); {
	case d < 3*24*time.Hour:
		return "最近"
	case d < 31*24*time.Hour:
		return "この1か月"
	default:
		return "しばらく前"
	}
}

func (s *Server) importanceWantText(w *mywant.Want) string {
	word, purpose := s.importanceTypePhrase(w.Metadata.Type)
	var l kvLine
	l.add("kind", "want")
	l.add("name", wantDisplayName(w))
	l.add("type", word)
	l.add("status", importanceStatus(w.GetStatus()))
	if s.visitedRecently(w.Metadata.ID) {
		l.add("visited", "さっき")
	}
	l.add("usage", purpose)
	// What it was asked: the parameters given a value, a few.
	keys := make([]string, 0, len(w.Spec.Params))
	for k := range w.Spec.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var asked []string
	for _, k := range keys {
		v := strings.TrimSpace(fmt.Sprint(w.Spec.Params[k]))
		if v == "" || v == "<nil>" || strings.HasPrefix(v, "map[") || strings.HasPrefix(v, "[") {
			continue
		}
		asked = append(asked, k+"="+clipRunes(v, 16))
		if len(asked) == 4 {
			break
		}
	}
	l.add("params", strings.Join(asked, ","))
	l.add("result", clipRunes(robotCardSummary(w.GetAllState()["final_result"]), 50))
	return l.String()
}

// paramMeaning is what a type's parameter is, in a few words: its label, else
// its description.
func (s *Server) paramMeaning(typ, name string) string {
	if s.wantTypeLoader == nil {
		return ""
	}
	if def := s.wantTypeLoader.GetDefinition(typ); def != nil {
		for _, p := range def.Parameters {
			if p.Name == name {
				if p.Label != "" {
					return p.Label
				}
				return clipRunes(p.Description, 24)
			}
		}
	}
	return ""
}

func (s *Server) importanceThingText(t ThingFull) string {
	var l kvLine
	l.add("kind", t.Subtype)
	l.add("value", t.Value)
	// The wants naming it now: as what, by which, and how they stand.
	var usage, by, byStatus []string
	for _, id := range t.WantIDs {
		w := s.findWantByIDInAll(id)
		if w == nil {
			continue
		}
		word, _ := s.importanceTypePhrase(w.Metadata.Type)
		as := word
		for k, v := range w.Spec.Params {
			if fmt.Sprint(v) == t.Value {
				as += "の" + k
				if m := s.paramMeaning(w.Metadata.Type, k); m != "" {
					as += "(" + m + ")"
				}
				break
			}
		}
		usage = append(usage, as)
		by = append(by, wantDisplayName(w))
		byStatus = append(byStatus, importanceStatus(w.GetStatus()))
		if len(by) == 2 {
			break
		}
	}
	if len(by) == 0 {
		l.add("used_by", "なし")
	} else {
		l.add("usage", strings.Join(usage, "/"))
		l.add("used_by", strings.Join(by, "/"))
		l.add("used_by_status", strings.Join(byStatus, "/"))
	}
	// How often, and how lately, wants have used it.
	if st := t.Stats; st != nil {
		l.add("uses", fmt.Sprintf("%d回", st.Count))
		l.add("last_used", agoWords(st.LastUsed, time.Now()))
		var with []string
		for _, tc := range st.TopWantTypes {
			w, _ := s.importanceTypePhrase(tc.Type)
			with = append(with, w)
		}
		l.add("often_with", strings.Join(with, ","))
	} else {
		l.add("uses", "0回")
	}
	return l.String()
}

func (s *Server) importance(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var req struct {
		Context string           `json:"context"`
		Items   []importanceItem `json:"items"`
		// Mode "each" (the default) asks Laya of every item on its own (is it
		// important?); "group" asks it to choose among them (which matters most?).
		Mode    string `json:"mode"`
		NoCache bool   `json:"nocache"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "bad body", err.Error())
		return
	}
	if len(req.Items) > 100 {
		req.Items = req.Items[:100]
	}
	// The item's own words alone, no hour: what it is does not change by the
	// clock, and a state with the hour in it would be a new question every
	// hour. The person's situation comes in when the client gives one.
	ctx := strings.TrimSpace(req.Context)
	if req.Mode == "group" {
		situation := "いま " + time.Now().Format("1月2日 15時")
		if ctx != "" {
			situation += "。" + ctx
		}
		s.importanceGrouped(w, r, situation, req.Items, req.NoCache, started)
		return
	}

	out := make([]importanceResult, len(req.Items))
	var ask []decisionItem
	var askAt []int
	var askHash []string
	for i, it := range req.Items {
		text := s.importanceText(it)
		out[i] = importanceResult{ID: it.ID, Text: text}
		if text == "" {
			continue
		}
		state := "項目: " + text
		if ctx != "" {
			state = "状況: " + ctx + "\n" + state
		}
		hash := importanceHash(state)
		if !req.NoCache {
			if st, ok := s.storedImportance(it); ok &&
				(st.hash == hash || time.Since(st.at) < importanceMinInterval) {
				out[i].Score, out[i].Cached = st.score, true
				out[i].Stale = st.hash != hash
				continue
			}
		}
		ask = append(ask, decisionItem{State: state, Questions: map[string]decisionQuestion{importanceQuestionID: importanceQuestion}})
		askAt = append(askAt, i)
		askHash = append(askHash, hash)
	}

	by := ""
	if len(ask) > 0 {
		answers, who, err := (&robotDecider{}).decideMany(ask)
		by = who
		if err != nil {
			s.JSONError(w, r, http.StatusServiceUnavailable, "laya did not answer", err.Error())
			return
		}
		for k, a := range answers {
			i := askAt[k]
			p := a[importanceQuestionID].Probabilities
			out[i].Probabilities = p
			out[i].Score = p["important"] + p["normal"]/2
			it := req.Items[i]
			_ = s.importanceStore(it.Kind).Set(it.ID, importanceLabel, fmt.Sprintf("%.3f@%s@%s", out[i].Score, askHash[k], time.Now().Format(time.RFC3339)))
		}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"items": out, "by": by, "asked": len(ask), "ms": time.Since(started).Milliseconds(),
	})
}

// ── grouped: Laya chooses among the items ───────────────────────────────────
//
// One Laya read takes a whole window (256 tokens) however short the text, and
// an item is ~14 tokens: asked one by one, most of every read is padding. And
// what the screen wants is a comparison — where to look — which a choice among
// the items gives directly, as Laya's own probabilities.
//
// At most importanceGroupMax options a read (the calibration's choice:6-10
// bucket; more flattens the probabilities). More items than that are split
// into groups that all hold the same anchor item, so they can be joined: in a
// choice p_i / p_anchor = exp((z_i − z_anchor) / T), the logit difference
// itself, which does not depend on what else is in the group (as far as Laya
// reads each option apart) — so every item's ratio to the anchor is on one
// scale. The groups are asked side by side, in one round. A last group too
// small for the same bucket is filled up with items already in another group,
// so every read has the same temperature.

const (
	importanceGroupMax = 10
	importanceGroupMin = 6
)

var importanceGroupQuestion = struct {
	Type, Instructions string
}{"choice", "本人が今いちばん見るべきものはどれか"}

// Grouped answers, kept by situation + the group's texts in order.
var (
	importanceGroupCacheMu sync.Mutex
	importanceGroupCache   = map[string]map[string]float64{}
)

func (s *Server) importanceGrouped(w http.ResponseWriter, r *http.Request, situation string, items []importanceItem, noCache bool, started time.Time) {
	out := make([]importanceResult, len(items))
	var idx []int // items with words, in the order sent (the screen's)
	for i, it := range items {
		out[i] = importanceResult{ID: it.ID, Text: s.importanceText(it)}
		if out[i].Text != "" {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		s.JSONResponse(w, http.StatusOK, map[string]any{"items": out, "by": "", "asked": 0, "reads": 0, "mode": "group", "ms": time.Since(started).Milliseconds()})
		return
	}

	// Groups of item indexes. One group when they fit; otherwise the anchor
	// (the first item) in every group, with up to max-1 others each.
	var groups [][]int
	if len(idx) <= importanceGroupMax {
		groups = [][]int{idx}
	} else {
		anchor, rest := idx[0], idx[1:]
		per := importanceGroupMax - 1
		for start := 0; start < len(rest); start += per {
			end := start + per
			if end > len(rest) {
				end = len(rest)
			}
			g := append([]int{anchor}, rest[start:end]...)
			// Fill a short last group from the items before it, for the same bucket.
			for k := start - 1; len(g) < importanceGroupMin && k >= 0; k-- {
				g = append(g, rest[k])
			}
			groups = append(groups, g)
		}
	}

	// Laya favours the option read first (a group's winner followed whatever
	// was put first, and turned round with the order). So each group is read
	// twice, as shown and turned round, and an item's ratio to the anchor is
	// the geometric mean of the two: the position's pull cancels.
	type read struct {
		key   string
		item  decisionItem
		order []int // item indexes in the order Laya reads them
		probs map[string]float64
	}
	type groupReads struct {
		anchor int
		reads  [2]*read
	}
	gs := make([]groupReads, len(groups))
	var ask []decisionItem
	var askRead []*read
	importanceGroupCacheMu.Lock()
	if noCache {
		importanceGroupCache = map[string]map[string]float64{}
	}
	for gi, g := range groups {
		gs[gi].anchor = g[0]
		rev := make([]int, len(g))
		for k := range g {
			rev[k] = g[len(g)-1-k]
		}
		for ri, order := range [][]int{g, rev} {
			crit := map[string]string{}
			keyParts := []string{situation}
			for k, i := range order {
				// Keys sort in the order given: Laya reads the options in that order.
				crit[fmt.Sprintf("i%02d", k)] = out[i].Text
				keyParts = append(keyParts, out[i].Text)
			}
			q := decisionQuestion{Type: importanceGroupQuestion.Type, Instructions: importanceGroupQuestion.Instructions, Criteria: crit}
			rd := &read{key: strings.Join(keyParts, "\x1f"), order: order,
				item: decisionItem{State: situation, Questions: map[string]decisionQuestion{importanceQuestionID: q}}}
			if p, ok := importanceGroupCache[rd.key]; ok {
				rd.probs = p
			} else {
				ask = append(ask, rd.item)
				askRead = append(askRead, rd)
			}
			gs[gi].reads[ri] = rd
		}
	}
	importanceGroupCacheMu.Unlock()

	by := ""
	if len(ask) > 0 {
		answers, who, err := (&robotDecider{}).decideMany(ask)
		by = who
		if err != nil {
			s.JSONError(w, r, http.StatusServiceUnavailable, "laya did not answer", err.Error())
			return
		}
		importanceGroupCacheMu.Lock()
		for k, a := range answers {
			askRead[k].probs = a[importanceQuestionID].Probabilities
			importanceGroupCache[askRead[k].key] = askRead[k].probs
		}
		importanceGroupCacheMu.Unlock()
	}

	// Each item's ratio to its group's anchor, the two reads' geometric mean;
	// an item in more than one group keeps its first.
	ratio := map[int]float64{}
	prob := map[int]float64{}
	for _, g := range gs {
		per := [2]map[int]float64{}
		for ri, rd := range g.reads {
			per[ri] = map[int]float64{}
			pa := 0.0
			for k, i := range rd.order {
				p := rd.probs[fmt.Sprintf("i%02d", k)]
				per[ri][i] = p
				if i == g.anchor {
					pa = p
				}
			}
			if pa > 0 {
				for i, p := range per[ri] {
					per[ri][i] = p / pa
				}
			}
		}
		for i, a := range per[0] {
			if _, seen := ratio[i]; seen {
				continue
			}
			ratio[i] = math.Sqrt(a * per[1][i])
			prob[i] = (g.reads[0].probs[fmt.Sprintf("i%02d", indexOf(g.reads[0].order, i))] +
				g.reads[1].probs[fmt.Sprintf("i%02d", indexOf(g.reads[1].order, i))]) / 2
		}
	}
	top := 0.0
	for _, v := range ratio {
		if v > top {
			top = v
		}
	}
	for _, i := range idx {
		if top > 0 {
			out[i].Score = ratio[i] / top // the most important in view is 1
		}
		out[i].Probabilities = map[string]float64{"chosen": prob[i]}
		out[i].Cached = len(ask) == 0
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"items": out, "by": by, "asked": len(idx), "reads": len(ask), "groups": len(groups),
		"mode": "group", "ms": time.Since(started).Milliseconds(),
	})
}

func indexOf(xs []int, v int) int {
	for k, x := range xs {
		if x == v {
			return k
		}
	}
	return -1
}
