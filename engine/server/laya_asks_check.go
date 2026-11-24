package server

// Each type's asks put to the request reading as it runs — which sentence
// Laya says it is, what Apple's model reads to fill it, how Laya reads its
// time — and what came of each set beside the parameters it is to be made
// with. Nothing is made: the reading is tested, not the want.
//
//	POST /api/v1/laya/test-asks   {"type": "transit_search"} (or every type with asks)
//	GET  /api/v1/laya/test-asks   where it stands, and the last results
//
// What a training (laya_readings_train.go) is checked by: run before and
// after, the same asks.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

var layaAsksTesting atomic.Bool

func layaAsksTestedFile() string { return initiativeHomeFile("laya-asks-tested.json") }

type askResult struct {
	Type     string            `json:"type"`
	Say      string            `json:"say"`
	At       string            `json:"at,omitempty"`
	Want     map[string]string `json:"want"`            // the parameters it is to be made with
	Got      map[string]string `json:"got,omitempty"`   // what the reading came to
	ReadAs   string            `json:"read_as"`         // Laya's three likeliest
	InTop    bool              `json:"in_top"`          // its type among them (right)
	Resolved string            `json:"resolved"`        // what they were settled on: a type, ask:a|b, none
	Wrong    []string          `json:"wrong,omitempty"` // what differs
	OK       bool              `json:"ok"`
	Seconds  float64           `json:"seconds"`
}

// testAsk reads one ask as a request would be read — asked by who, for what
// is filled from their home — and says what differs. A parameter the ask
// gives as <home> is to be who's home at that part of the day: nothing, when
// they have none then.
func (s *Server) testAsk(typ string, ask askOf, who string) askResult {
	started := time.Now()
	r := askResult{Type: typ, Say: ask.Say, At: ask.At, Want: map[string]string{}}
	for k, v := range ask.Params {
		r.Want[k] = strings.TrimSpace(fmt.Sprint(v))
	}
	if hasHome(r.Want) {
		home := s.characterHomeAt(who, []string{typ}, askAt(ask.At)).Says[typ]
		for k, v := range r.Want {
			if v != askHome {
				continue
			}
			if home.Param == k && home.Value != "" {
				r.Want[k] = home.Value
			} else {
				delete(r.Want, k)
			}
		}
	}
	defer func() { r.Seconds = time.Since(started).Seconds() }()

	q := strings.TrimRight(strings.TrimSpace(ask.Say), "。．.!！?？ ")
	sources, _ := s.magicSources()
	mm, ok, asked := s.magicLayaMatch(q, sources)
	var top []string
	for _, c := range mm.candidates {
		top = append(top, c.wantType+c.tool)
	}
	r.ReadAs = strings.Join(top, ",")
	switch {
	case !asked:
		r.Wrong = append(r.Wrong, "no Laya to ask")
		return r
	case !ok:
		r.Wrong = append(r.Wrong, "read as talk")
		return r
	}
	// Right when it is among the three likeliest; what fills it is read for
	// it, as it would be had it been settled on.
	var right *magicMatch
	for i, c := range mm.candidates {
		if c.wantType == typ && c.tool == "" {
			right = &mm.candidates[i]
			break
		}
	}
	if right == nil {
		r.Wrong = append(r.Wrong, "not among the three likeliest")
		return r
	}
	r.InTop = true
	// Which of the three it would be settled on (magicResolve), told apart.
	chosen, outcome, cands := s.magicResolve(q, mm)
	switch outcome {
	case resolveMatched:
		r.Resolved = chosen.wantType + chosen.tool
	case resolveAsk:
		var names []string
		for _, c := range cands {
			names = append(names, c.wantType+c.tool)
		}
		r.Resolved = "ask:" + strings.Join(names, "|")
	default:
		r.Resolved = resolveNone
	}
	mm = *right
	mm.params = map[string]string{}
	s.magicModelParams(ask.Say, &mm)
	params := map[string]any{}
	for k, v := range mm.params {
		if strings.TrimSpace(v) != "" {
			params[k] = v
		}
	}
	if def := s.typeDefinition(typ); def != nil {
		s.robotReadTimesAt(typ, def.Metadata.Labels, params, ask.Say, askAt(ask.At))
	}
	s.robotHomeParamsAt(typ, params, who, askAt(ask.At))
	r.Got = map[string]string{}
	for k, v := range params {
		r.Got[k] = strings.TrimSpace(fmt.Sprint(v))
	}
	for k, want := range r.Want {
		got := r.Got[k]
		if k == "time" || strings.HasSuffix(k, "_time") {
			if a, err := robotClock(want); err == nil {
				want = a
			}
		}
		// 「国分寺駅」 for 国分寺: the station either way.
		if got != want && strings.TrimSuffix(got, "駅") != strings.TrimSuffix(want, "駅") {
			r.Wrong = append(r.Wrong, fmt.Sprintf("%s=%q, not %q", k, got, want))
		}
	}
	for k, got := range r.Got {
		if _, said := r.Want[k]; !said {
			r.Wrong = append(r.Wrong, fmt.Sprintf("%s=%q, not said", k, got))
		}
	}
	sort.Strings(r.Wrong)
	r.OK = len(r.Wrong) == 0
	return r
}

type askOf struct {
	Say    string
	At     string
	Params map[string]any
}

// asksOf is every ask to test: of one type, or of every type that has some.
func (s *Server) asksOf(only string) map[string][]askOf {
	out := map[string][]askOf{}
	sources, _ := s.magicSources()
	for _, src := range sources {
		if src.tool != "" || src.name != "" || (only != "" && src.typ != only) {
			continue
		}
		def := s.typeDefinition(src.typ)
		if def == nil {
			continue
		}
		aliases := cleanAliases(strings.Split(def.Metadata.Labels[robotAliasLabel], ","))
		for _, a := range expandAsks(def.Asks, aliases) {
			out[src.typ] = append(out[src.typ], askOf{Say: a.Say, At: a.At, Params: a.Params})
		}
	}
	return out
}

// POST starts a test; GET says where it stands and what the last came to.
func (s *Server) handleLayaTestAsks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		raw, _ := os.ReadFile(layaAsksTestedFile())
		robotWriteJSON(w, map[string]any{"testing": layaAsksTesting.Load(), "last": json.RawMessage(orJSON(raw))})
		return
	}
	var req struct {
		Type string `json:"type"`
		// Character: who asks them, for a <home> (home_params.go).
		Character string `json:"character"`
		// Failed: only the asks the last test got wrong (their type not among
		// the three likeliest) — a change checked on what it was for, in
		// seconds, not on every ask again.
		Failed bool `json:"failed"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	asks := s.asksOf(strings.TrimSpace(req.Type))
	if req.Failed {
		asks = lastFailedAsks(asks)
	}
	n := 0
	for _, a := range asks {
		n += len(a)
	}
	if n == 0 {
		http.Error(w, "no asks to test: no type with a magic sentence says how it is asked for (want-spec asks)", http.StatusNotFound)
		return
	}
	if !layaAsksTesting.CompareAndSwap(false, true) {
		http.Error(w, "a test is running", http.StatusConflict)
		return
	}
	go func() {
		defer layaAsksTesting.Store(false)
		started := time.Now()
		var results []askResult
		passed, inTop, resolvedRight, asked := 0, 0, 0, 0
		types := make([]string, 0, len(asks))
		for t := range asks {
			types = append(types, t)
		}
		sort.Strings(types)
		for _, t := range types {
			for _, a := range asks[t] {
				res := s.testAsk(t, a, req.Character)
				if res.OK {
					passed++
				}
				if res.InTop {
					inTop++
				}
				switch {
				case res.Resolved == t:
					resolvedRight++
				case strings.HasPrefix(res.Resolved, "ask:") && strings.Contains(res.Resolved, t):
					asked++
				}
				results = append(results, res)
			}
		}
		out := map[string]any{"at": time.Now().Format(time.RFC3339), "asks": len(results), "passed": passed,
			"in_top": inTop, "resolved_right": resolvedRight, "asked_with_it": asked,
			"seconds": int(time.Since(started).Seconds()), "results": results}
		raw, _ := json.MarshalIndent(out, "", " ")
		_ = os.WriteFile(layaAsksTestedFile(), raw, 0o644)
		log.Printf("[laya] asks: %d of %d read right", passed, len(results))
	}()
	robotWriteJSON(w, map[string]any{"started": true, "asks": n})
}

// askHome is an ask's parameter that is to be the asker's home (home_params.go).
const askHome = "<home>"

func hasHome(params map[string]string) bool {
	for _, v := range params {
		if v == askHome {
			return true
		}
	}
	return false
}

// lastFailedAsks are those of asks the last test got wrong: their type not
// among the three likeliest.
func lastFailedAsks(asks map[string][]askOf) map[string][]askOf {
	var last struct {
		Results []askResult `json:"results"`
	}
	raw, err := os.ReadFile(layaAsksTestedFile())
	if err != nil || json.Unmarshal(raw, &last) != nil {
		return asks
	}
	wrong := map[string]bool{}
	for _, r := range last.Results {
		if !r.InTop {
			wrong[r.Type+"\x00"+r.Say] = true
		}
	}
	out := map[string][]askOf{}
	for t, list := range asks {
		for _, a := range list {
			if wrong[t+"\x00"+a.Say] {
				out[t] = append(out[t], a)
			}
		}
	}
	return out
}
