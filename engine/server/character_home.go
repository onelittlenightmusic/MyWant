package server

// A character's home: where its person goes at this time of day.
//
// Home is the person's, not a type's: transit is a tool everyone on the board
// uses, and where it is taken in the evening depends on who takes it. So it is
// learned from the wants a character asked for (mywant.io/owner-character —
// stamped by the GUI on what it adds, and by CanvasNearHook on what the robot
// adds beside whoever is at the controls). A want nobody is recorded as asking
// for is nobody's habit and is not counted.
//
// Each such want was an ask at the time it first answered: the places it names
// (parameters with a subType, kept as things — never a secret) are where, the
// part of the day (朝・昼・夕方・夜) is when. The place asked most in a part of
// the day, at least twice and more than any other, is the character's home
// then: 中野坂上 in the evening. Nothing here names a type, a parameter or a
// place.
//
// A type then says it in its own words — the parameter of its own that takes
// it: the one the character fills with it in this part of the day (transit's
// `to` in the evening), else the first that takes its kind (a route's `to`
// takes a station), else, through a constellation holding the home, a member
// of the kind it does take (weather's `at` takes a city: 中野坂上 and nakano
// held together, nakano). Nothing found, no home for the type — never a guess.
//
// Learned from the character's ask ledger (core/character_homes.go), kept in
// characters.yaml beside the characters: a want enters it when it first
// answers, so nothing here reads the wants, and what was worked out from it is
// kept until it changes.
//
// GET /api/v1/characters/{id}/home[?types=<type>,<type>…][&at=<RFC3339>]

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"

	mywant "mywant/engine/core"
)

// homeMinAsks is how many asks in a part of the day make a habit.
const homeMinAsks = 2

// wantLabelOwnerCharacter is the character that asked for a want.
const wantLabelOwnerCharacter = "mywant.io/owner-character"

// dayParts are the parts of the day asks are sorted into, by the hour they
// start; the last runs past midnight to the first.
var dayParts = []struct {
	Name  string
	Start int
}{{"朝", 5}, {"昼", 10}, {"夕方", 16}, {"夜", 20}}

func dayPart(t time.Time) string {
	part := dayParts[len(dayParts)-1].Name
	for _, p := range dayParts {
		if t.Hour() >= p.Start {
			part = p.Name
		}
	}
	return part
}

type characterHome struct {
	Part string `json:"part"`           // the part of the day asked about
	Home string `json:"home,omitempty"` // the place, e.g. 中野坂上
	Kind string `json:"kind,omitempty"` // its subtype, e.g. station
	Asks int    `json:"asks,omitempty"` // asks naming it in this part of the day
	Of   int    `json:"of,omitempty"`   // asks in this part of the day
	// Says is the home in each asked type's words, by type: only the types
	// that have one.
	Says map[string]homeWord `json:"says,omitempty"`
}

// placeParams are a type's parameters that name a place the person goes: a
// subtype, kept as a thing, never a secret. In the definition's order.
func placeParams(def *mywant.WantTypeDefinition) [][2]string {
	var out [][2]string
	if def == nil {
		return out
	}
	for _, pd := range def.Parameters {
		if pd.SubType != "" && pd.SubType != "secret" && pd.ShouldRecordThing() {
			out = append(out, [2]string{pd.Name, pd.SubType})
		}
	}
	return out
}

// homeAsk is one place one want named.
type homeAsk struct{ typ, param, value, kind string }

// askOf is the ask a want is, by whom: the character that asked for it and the
// places it names, at the time given. ok is false for a want nobody is
// recorded as asking for, or one naming no place.
func (s *Server) askOf(w *mywant.Want, at time.Time) (string, mywant.CharacterAsk, bool) {
	owner := w.GetLabel(wantLabelOwnerCharacter)
	if owner == "" || w.Metadata.ID == "" {
		return "", mywant.CharacterAsk{}, false
	}
	ask := mywant.CharacterAsk{Want: w.Metadata.ID, Type: w.Metadata.Type, At: at}
	for _, pk := range placeParams(s.globalBuilder.GetWantTypeDefinition(w.Metadata.Type)) {
		if v, _ := w.Spec.Params[pk[0]].(string); strings.TrimSpace(v) != "" {
			ask.Places = append(ask.Places, mywant.AskPlace{Param: pk[0], Value: strings.TrimSpace(v), Kind: pk[1]})
		}
	}
	return owner, ask, len(ask.Places) > 0
}

// noteHomeAsk enters a want in its character's ledger, at the time it first
// answered (from OnWantOutput, on each new answer). Once per want: after
// that, nothing is read or written.
func (s *Server) noteHomeAsk(w *mywant.Want) {
	owner := w.GetLabel(wantLabelOwnerCharacter)
	if owner == "" || mywant.CharacterHasAsk(owner, w.Metadata.ID) {
		return
	}
	answers := w.ResultHistoryEntries()
	if len(answers) == 0 {
		return
	}
	if owner, ask, ok := s.askOf(w, answers[0].Timestamp); ok {
		mywant.RecordCharacterAsk(owner, ask)
	}
}

// seedCharacterHomes enters the wants already answered — those from before
// the ledger, and any that answered while the server was down. The wants are
// restored by the reconcile loop after the server starts, with no word when
// it is done, so this waits for their number to stop changing first.
func (s *Server) seedCharacterHomes() {
	last := -1
	for i := 0; i < 30; i++ {
		time.Sleep(2 * time.Second)
		n := len(s.globalBuilder.GetWants())
		if n > 0 && n == last {
			break
		}
		last = n
	}
	before := 0
	for _, w := range s.globalBuilder.GetWants() {
		owner := w.GetLabel(wantLabelOwnerCharacter)
		if owner == "" || mywant.CharacterHasAsk(owner, w.Metadata.ID) {
			continue
		}
		before++
		s.noteHomeAsk(w)
	}
	if before > 0 {
		log.Printf("[character-home] looked at %d answered-or-not wants not yet in a ledger", before)
	}
}

// homeWord is how a type says the home: its parameter that takes it, and what
// goes in it.
type homeWord struct {
	Param   string `json:"param"`
	Value   string `json:"value"`
	Through string `json:"through,omitempty"` // the constellation it was found through
}

// learnedHome is a character's home in a part of the day and the asks it was
// learned from, kept until the ledgers change.
type learnedHome struct {
	version uint64
	home    characterHome
	perWant [][]homeAsk
}

var (
	learnedHomesMu sync.Mutex
	learnedHomes   = map[string]learnedHome{} // by "character\npart"
)

// homeIn is the character's home in the part of the day, worked out from the
// ledger once per change to it.
func homeIn(characterID, part string, loc *time.Location) (characterHome, [][]homeAsk) {
	ledger, version := mywant.CharacterAsks(characterID)
	key := characterID + "\n" + part + "\n" + loc.String()
	learnedHomesMu.Lock()
	defer learnedHomesMu.Unlock()
	if l, ok := learnedHomes[key]; ok && l.version == version {
		return l.home, l.perWant
	}
	home := characterHome{Part: part}
	var perWant [][]homeAsk
	for _, a := range ledger {
		if dayPart(a.At.In(loc)) != part {
			continue
		}
		asks := make([]homeAsk, 0, len(a.Places))
		for _, p := range a.Places {
			asks = append(asks, homeAsk{a.Type, p.Param, p.Value, p.Kind})
		}
		perWant = append(perWant, asks)
	}
	pickHome(&home, perWant)
	learnedHomes[key] = learnedHome{version, home, perWant}
	return home, perWant
}

// characterHomeAt is the character's home at the time, said in each of the
// types' words that has one.
func (s *Server) characterHomeAt(characterID string, types []string, at time.Time) characterHome {
	home := characterHome{Part: dayPart(at)}
	if characterID == "" {
		return home
	}
	home, perWant := homeIn(characterID, home.Part, at.Location())
	if home.Home == "" || len(types) == 0 {
		return home
	}
	var groups []homeGroup
	groupsRead := false
	home.Says = map[string]homeWord{}
	for _, typeName := range types {
		params := placeParams(s.globalBuilder.GetWantTypeDefinition(typeName))
		if len(params) == 0 {
			continue
		}
		// Constellations only when a type needs them, and read once.
		if !groupsRead && !saysDirectly(typeName, params, perWant, home) {
			groups, groupsRead = s.homeGroups(), true
		}
		if word, ok := sayHomeAs(home, typeName, params, perWant, groups); ok {
			home.Says[typeName] = word
		}
	}
	return home
}

// homeGroups are the constellations, as the things they hold.
func (s *Server) homeGroups() []homeGroup {
	byID := map[string][2]string{}
	if s.thingStore != nil {
		for _, e := range s.thingStore.Entries() {
			byID[e.ID] = [2]string{e.Catalog, e.Value}
		}
	}
	var groups []homeGroup
	for _, c := range s.collectAllConstellations() {
		g := homeGroup{name: c.Name}
		for _, m := range c.Members {
			if key, value, ok := strings.Cut(m, "::"); ok {
				g.things = append(g.things, [2]string{key, value})
			} else if t, ok := byID[m]; ok && t[1] != "" {
				g.things = append(g.things, t)
			}
		}
		groups = append(groups, g)
	}
	return groups
}

// pickHome sets the place named by the most wants (a want naming it twice,
// from and to, counts once): at least homeMinAsks, and more than any other.
func pickHome(home *characterHome, perWant [][]homeAsk) {
	count := map[string]int{}
	kindOf := map[string]string{}
	for _, asks := range perWant {
		named := map[string]bool{}
		for _, a := range asks {
			kindOf[a.value] = a.kind
			if !named[a.value] {
				named[a.value] = true
				count[a.value]++
			}
		}
	}
	home.Of = len(perWant)
	values := make([]string, 0, len(count))
	for v := range count {
		values = append(values, v)
	}
	sort.Strings(values)
	best, second := "", 0
	for _, v := range values {
		if n := count[v]; best == "" || n > count[best] {
			if best != "" {
				second = max(second, count[best])
			}
			best = v
		} else {
			second = max(second, n)
		}
	}
	if best == "" || count[best] < homeMinAsks || count[best] == second {
		return
	}
	home.Home, home.Kind, home.Asks = best, kindOf[best], count[best]
}

// homeGroup is a constellation's things: catalog key and value each.
type homeGroup struct {
	name   string
	things [][2]string
}

// sayHomeAs finds the parameter of typeName (params, in order) that takes the
// home, and what goes in it; false when it has none.
func sayHomeAs(home characterHome, typeName string, params [][2]string, perWant [][]homeAsk, groups []homeGroup) (homeWord, bool) {
	if p := paramFilledWithHome(typeName, params, perWant, home); p != "" {
		return homeWord{Param: p, Value: home.Home}, true
	}
	// Through a constellation holding it: a member of a kind a parameter takes.
	homeKey := subtypeToKey(home.Kind)
	for _, g := range groups {
		holds := false
		for _, t := range g.things {
			if t[0] == homeKey && t[1] == home.Home {
				holds = true
			}
		}
		if !holds {
			continue
		}
		for _, pk := range params {
			for _, t := range g.things {
				if t[0] == subtypeToKey(pk[1]) && t[1] != home.Home {
					return homeWord{Param: pk[0], Value: t[1], Through: g.name}, true
				}
			}
		}
	}
	return homeWord{}, false
}

// paramFilledWithHome is the type's parameter that takes the home as it is:
// the one the character fills with it in this part of the day, else the first
// that takes its kind; "" for none.
func paramFilledWithHome(typeName string, params [][2]string, perWant [][]homeAsk, home characterHome) string {
	used := map[string]int{}
	for _, asks := range perWant {
		for _, a := range asks {
			if a.typ == typeName && a.value == home.Home {
				used[a.param]++
			}
		}
	}
	best := ""
	for _, pk := range params {
		if used[pk[0]] > used[best] {
			best = pk[0]
		}
	}
	if best != "" {
		return best
	}
	for _, pk := range params {
		if pk[1] == home.Kind {
			return pk[0]
		}
	}
	return ""
}

// saysDirectly is whether the type takes the home as it is, with no
// constellation to look through.
func saysDirectly(typeName string, params [][2]string, perWant [][]homeAsk, home characterHome) bool {
	return paramFilledWithHome(typeName, params, perWant, home) != ""
}

func (s *Server) getCharacterHome(w http.ResponseWriter, r *http.Request) {
	at := time.Now()
	if v := r.URL.Query().Get("at"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			http.Error(w, "at: "+err.Error(), http.StatusBadRequest)
			return
		}
		at = t
	}
	var types []string
	for _, t := range strings.Split(r.URL.Query().Get("types"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			types = append(types, t)
		}
	}
	robotWriteJSON(w, s.characterHomeAt(mux.Vars(r)["id"], types, at))
}

// wantHome is the home of the character that asked for w, now, in w's type's
// words (mywant.WantHomeOf — what its voice script is handed); nil for none.
func (s *Server) wantHome(w *mywant.Want) map[string]any {
	owner := w.GetLabel(wantLabelOwnerCharacter)
	if owner == "" {
		return nil
	}
	typ := w.Metadata.Type
	h := s.characterHomeAt(owner, []string{typ}, time.Now())
	word, ok := h.Says[typ]
	if !ok {
		return nil
	}
	cur, _ := w.Spec.Params[word.Param].(string)
	return map[string]any{
		"part": h.Part, "place": h.Home, "param": word.Param, "value": word.Value,
		"through": word.Through, "asks": h.Asks, "of": h.Of,
		"here": strings.TrimSpace(cur) == word.Value,
	}
}

// typeSpokenName is what a person calls a type: its first alias that is not
// ASCII (乗り換え), else its first alias, else its name.
func (s *Server) typeSpokenName(typ string) string {
	first := ""
	if s.wantTypeLoader != nil {
		if def := s.wantTypeLoader.GetDefinition(typ); def != nil {
			for _, a := range strings.Split(def.Metadata.Labels[robotAliasLabel], ",") {
				if a = strings.TrimSpace(a); a == "" {
					continue
				}
				if first == "" {
					first = a
				}
				for _, r := range a {
					if r > 0x7f {
						return a
					}
				}
			}
		}
	}
	if first != "" {
		return first
	}
	return typ
}

// homePhrase says the home as the parameter takes it: going there (to),
// leaving it (from), or about it.
func homePhrase(param, value string) string {
	switch param {
	case "to":
		return value + "への"
	case "from":
		return value + "からの"
	}
	return value + "の"
}

// offerNow is the time as Laya reads it with an offer: the part of the day
// and the hour — "今: 夜 21時。". Coarse on purpose: 21:07 and 21:40 are one
// situation, and the words should say so.
func offerNow(t time.Time) string {
	return fmt.Sprintf("今: %s %d時。", dayPart(t), t.Hour())
}

// offerNearCells is how near, in cells, a place on the board must be to the
// person to be where they are; offerNearMax how many are said.
const (
	offerNearCells = 3
	offerNearMax   = 2
)

// offerWhere is where the person is as Laya reads it with an offer: the
// places on the board nearest them (things — 国分寺, not a cell),
// "本人の近く: 国分寺。", or "本人の近く: なし。" when none is near or the
// person is not on the board. Words, not coordinates: a cell means nothing to
// Laya, and a place is what a home offer is about.
func offerWhere(px, py int, ok bool, tiles []robotTile) string {
	if !ok {
		return "本人の近く: なし。"
	}
	type near struct {
		name string
		d    int
	}
	var ns []near
	for _, t := range tiles {
		if t.cardKind != "thing" {
			continue
		}
		if d := chebyshev(t.x-px, t.y-py); d <= offerNearCells {
			ns = append(ns, near{t.name, d})
		}
	}
	if len(ns) == 0 {
		return "本人の近く: なし。"
	}
	sort.SliceStable(ns, func(i, j int) bool {
		if ns[i].d != ns[j].d {
			return ns[i].d < ns[j].d
		}
		return ns[i].name < ns[j].name
	})
	var names []string
	for _, n := range ns {
		if len(names) == offerNearMax {
			break
		}
		names = append(names, n.name)
	}
	return "本人の近く: " + strings.Join(names, "、") + "。"
}

// dayPartAt is a time in the part of the day (two hours into it), on the day
// of t: what a part's home is asked at.
func dayPartAt(part string, t time.Time) time.Time {
	for _, p := range dayParts {
		if p.Name == part {
			return time.Date(t.Year(), t.Month(), t.Day(), (p.Start+2)%24, 0, 0, 0, t.Location())
		}
	}
	return t
}

// offerHomes are the person's homes, as offers: for each part of the day it
// has one, and each type on the board with a word for it, a want of that type
// there — or, when one is already on the board, running it again. Every part,
// not only this one: the offer says when ("夜にいつも行く中野坂上"), the time
// is beside it (offerNow), and whether it fits now is Laya's to weigh.
func (s *Server) offerHomes(who string, now time.Time) []offerCandidate {
	if who == "" || s.globalBuilder == nil {
		return nil
	}
	seen := map[string]bool{}
	var types []string
	for _, w := range s.globalBuilder.GetWants() {
		t := w.Metadata.Type
		if seen[t] || characterWantTypes[t] || w.GetLabels()[robotArchived] == "true" {
			continue
		}
		seen[t] = true
		types = append(types, t)
	}
	sort.Strings(types)
	var out []offerCandidate
	for _, part := range dayParts {
		h := s.characterHomeAt(who, types, dayPartAt(part.Name, now))
		for _, typ := range types {
			word, ok := h.Says[typ]
			if !ok {
				continue
			}
			what := homePhrase(word.Param, word.Value) + s.typeSpokenName(typ)
			text := fmt.Sprintf("%sにいつも行く%s。%sを調べる", h.Part, h.Home, what)
			// One already there: run it again.
			var there *mywant.Want
			for _, w := range s.globalBuilder.GetWants() {
				if v, _ := w.Spec.Params[word.Param].(string); w.Metadata.Type == typ && strings.TrimSpace(v) == word.Value &&
					w.GetLabels()[robotArchived] != "true" {
					there = w
					break
				}
			}
			if there != nil {
				name := there.Metadata.Name
				out = append(out, offerCandidate{
					Key: "home:" + h.Part + ":" + name, Want: name, Text: text,
					Ask: fmt.Sprintf("いつもの%sを最新にしましょうか？", what),
					run: func(s *Server) (string, error) {
						return s.useCapability(map[string]string{"capability": "restart", "want": name})
					},
				})
				continue
			}
			typ, params := typ, word.Param+"="+word.Value
			out = append(out, offerCandidate{
				Key: "home:" + h.Part + ":" + typ, Want: typ, Text: text,
				Ask: fmt.Sprintf("いつもの%sを調べましょうか？", what),
				run: func(s *Server) (string, error) {
					return s.robotDeployWant(map[string]string{"type": typ, "params": params})
				},
			})
		}
	}
	return out
}
