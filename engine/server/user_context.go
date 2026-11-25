package server

// user_context.go — what the person is about now, read from what they asked.
//
// 「何時に着く？」「天気は？」 said a minute after 「国分寺までの乗り換え」 are
// about 国分寺. Each person's last requests (userContextKeep, this hour) are
// read, after one is answered, by Apple's model into kinds of context
// (want-spec ContextKinds: place.destination, time.date, people.party_size, …
// — those some type's parameter takes, and interest.topic):
//
//	{"place.destination": "国分寺", "place.origin": "中野坂上", "time.arrive_by": "10:00", "interest.topic": "移動"}
//
// kept in the global state, a key a kind under the character's name
// (Hero.place.destination, Hero.context_at) — read and set there by anyone. A value is kept
// only as the person said it or a want was made with it, in the kind's form
// (contextValueOK): the model is small and fills a field it was asked for.
// Used two ways:
//
//   - a parameter not said is filled from it, by the parameter's context
//     (want-spec ParameterDef.Context — transit_search's to is
//     place.destination), before home's (home_params.go): what was said, then
//     this, then home;
//   - the robot's brief (plan.go) carries it as a line, for the model that
//     answers.
//
// It is worked out after a request, not while one waits: the next one uses it.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	mywant "mywant/engine/core"

	ws "github.com/onelittlenightmusic/want-spec"
)

const (
	userContextKeep   = 10               // requests read
	userContextWindow = 60 * time.Minute // older than this, not now's
	userContextAfter  = 4 * time.Second  // quiet this long after a request, it is read
	userContextWait   = 30 * time.Second // the model, waited for
)

// userContextAlways are asked for whatever the types take.
var userContextAlways = []string{"interest.topic"}

// userContextKinds are the kinds asked for: those some type's parameter is
// filled from, and userContextAlways, in want-spec's order.
func (s *Server) userContextKinds() []string {
	used := map[string]bool{}
	for _, k := range userContextAlways {
		used[k] = true
	}
	if s.wantTypeLoader != nil {
		for _, def := range s.wantTypeLoader.GetAll() {
			for _, p := range def.Parameters {
				if p.Context != "" {
					used[p.Context] = true
				}
			}
		}
	}
	var out []string
	for _, k := range ws.ContextKinds {
		if used[k.Name] {
			out = append(out, k.Name)
		}
	}
	return out
}

// paramContextKind is the kind a type's parameter is filled from ("" none).
func (s *Server) paramContextKind(typ, param string) string {
	if def := s.typeDefinition(typ); def != nil {
		for _, p := range def.Parameters {
			if p.Name == param {
				return p.Context
			}
		}
	}
	return ""
}

// userRequest is one thing a person asked, and the want made for it.
type userRequest struct {
	At     time.Time         `json:"at"`
	Said   string            `json:"said"`
	Type   string            `json:"type,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

// userContext is what a person is about now: kind → value.
type userContext struct {
	Fields    map[string]string
	UpdatedAt time.Time
}

// The requests read are kept here, for the next reading; what was read is
// kept in the global state, a key a kind (userContextKey), for anyone to read
// and change.
var userContexts = struct {
	sync.Mutex
	asked   map[string][]userRequest
	pending map[string]*time.Timer
}{asked: map[string][]userRequest{}, pending: map[string]*time.Timer{}}

// userContextAt is the key beside the kinds that says when they were read.
const userContextAt = "context_at"

// userContextName is who's name in the global state: the character's name
// (Hero), its id when it has none.
func userContextName(who string) string {
	if c, ok := mywant.GetCharacter(who); ok && strings.TrimSpace(c.Name) != "" {
		return strings.TrimSpace(c.Name)
	}
	return who
}

// userContextKey is where a kind of who's context is kept in the global
// state: <character name>.<kind> — Hero.place.destination.
func userContextKey(who, kind string) string {
	return userContextName(who) + "." + kind
}

// noteAsked keeps a request of who's, and reads their context again soon.
func (s *Server) noteAsked(who, said string) {
	s.noteRequest(who, userRequest{Said: said})
}

// noteMade adds the want made for a request of who's (what was said for it).
func (s *Server) noteMade(who, said, typ string, params map[string]any) {
	r := userRequest{Said: said, Type: typ, Params: map[string]string{}}
	for k, v := range params {
		if sv := strings.TrimSpace(fmt.Sprint(v)); v != nil && sv != "" && !strings.HasPrefix(k, "_") {
			r.Params[k] = sv
		}
	}
	s.noteRequest(who, r)
}

func (s *Server) noteRequest(who string, r userRequest) {
	who, r.Said = strings.TrimSpace(who), strings.TrimSpace(r.Said)
	if who == "" || r.Said == "" || !storyCharacterOK.MatchString(who) {
		return
	}
	r.At = time.Now()
	userContexts.Lock()
	defer userContexts.Unlock()
	list := userContexts.asked[who]
	// The same request again (its want made after it was asked): one.
	if n := len(list); n > 0 && list[n-1].Said == r.Said && r.At.Sub(list[n-1].At) < time.Minute {
		if r.Type != "" {
			list[n-1].Type, list[n-1].Params = r.Type, r.Params
		}
	} else {
		list = append(list, r)
	}
	if len(list) > userContextKeep {
		list = list[len(list)-userContextKeep:]
	}
	userContexts.asked[who] = list
	if t := userContexts.pending[who]; t != nil {
		t.Stop()
	}
	userContexts.pending[who] = time.AfterFunc(userContextAfter, func() { s.readUserContext(who) })
}

// userContextNow is who's context from the global state, when it is of this
// hour — read here, or set there by anyone since (a kind set by hand, with no
// time beside it, is now's).
func userContextNow(who string) (userContext, bool) {
	who = strings.TrimSpace(who)
	if who == "" {
		return userContext{}, false
	}
	c := userContext{Fields: map[string]string{}, UpdatedAt: time.Now()}
	for _, k := range ws.ContextKinds {
		if v, ok := mywant.GetGlobalState(userContextKey(who, k.Name)); ok && v != nil {
			if sv := strings.TrimSpace(fmt.Sprint(v)); sv != "" {
				c.Fields[k.Name] = sv
			}
		}
	}
	if v, ok := mywant.GetGlobalState(userContextKey(who, userContextAt)); ok {
		if t, err := time.Parse(time.RFC3339, fmt.Sprint(v)); err == nil {
			c.UpdatedAt = t
		}
	}
	return c, len(c.Fields) > 0 && time.Since(c.UpdatedAt) <= userContextWindow
}

// storeUserContext puts who's context in the global state, a key a kind; a
// kind no longer read is taken away.
func storeUserContext(who string, c userContext) {
	m := map[string]any{userContextKey(who, userContextAt): c.UpdatedAt.Format(time.RFC3339)}
	for k, v := range c.Fields {
		m[userContextKey(who, k)] = v
	}
	for _, k := range ws.ContextKinds {
		if _, kept := c.Fields[k.Name]; !kept {
			if _, there := mywant.GetGlobalState(userContextKey(who, k.Name)); there {
				mywant.DeleteGlobalStateKey(userContextKey(who, k.Name))
			}
		}
	}
	mywant.MergeGlobalState(m)
}

// readUserContext has the model read who's last requests into the fields.
func (s *Server) readUserContext(who string) {
	userContexts.Lock()
	var list []userRequest
	for _, r := range userContexts.asked[who] {
		if time.Since(r.At) <= userContextWindow {
			list = append(list, r)
		}
	}
	userContexts.Unlock()
	if len(list) == 0 {
		return
	}
	kinds := s.userContextKinds()
	prompt := userContextPrompt(list, kinds, s.paramContextKind)
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), userContextWait)
	defer cancel()
	text, by, err := generateText(ctx, prompt, false, userContextWait)
	trace := map[string]any{"who": who, "by": string(by), "ms": time.Since(started).Milliseconds(), "requests": len(list)}
	if err != nil {
		trace["error"] = err.Error()
		s.robotChatTrace("fm.context", trace, []string{"who", "requests", "by", "ms", "error"})
		return
	}
	things := map[string]bool{}
	for _, v := range s.boardWords() {
		things[v] = true
	}
	fields := userContextRead(text, list, kinds, func(v string) bool { return things[v] }, personNow())
	for k, v := range madeContext(list, s.paramContextKind) {
		if fields[k] == "" {
			fields[k] = v
		}
	}
	c := userContext{Fields: fields, UpdatedAt: time.Now()}
	order := []string{"who", "requests"}
	for _, f := range kinds {
		if v := c.Fields[f]; v != "" {
			trace[f] = v
			order = append(order, f)
		}
	}
	s.robotChatTrace("fm.context", trace, append(order, "by", "ms"))
	trace["key"] = userContextName(who) + ".*"
	storeUserContext(who, c)
}

// userContextPrompt: the requests, newest first, with what was made of each —
// each parameter with the kind it is, where it has one — and the kinds asked.
// No clock times: the model took one for arrive_by (2026-11-25).
func userContextPrompt(list []userRequest, kinds []string, kindOf func(typ, param string) string) string {
	var b strings.Builder
	b.WriteString("These are a person's requests, the LATEST FIRST, with the want made for each:\n")
	for i := len(list) - 1; i >= 0; i-- {
		r := list[i]
		mark := ""
		if i == len(list)-1 {
			mark = " (latest)"
		}
		fmt.Fprintf(&b, "%d.%s 「%s」", len(list)-i, mark, r.Said)
		if r.Type != "" {
			var kv []string
			for k, v := range r.Params {
				item := k + "=" + v
				if kindOf != nil {
					if kind := kindOf(r.Type, k); kind != "" {
						item += " [" + kind + "]"
					}
				}
				kv = append(kv, item)
			}
			sort.Strings(kv)
			fmt.Fprintf(&b, " → %s %s", r.Type, strings.Join(kv, " "))
		}
		b.WriteString("\n")
	}
	b.WriteString("Say what the person is about now. When requests differ, the latest one (1) is right — they changed their mind:\n")
	for _, name := range kinds {
		if k, ok := ws.ContextKindByName(name); ok {
			fmt.Fprintf(&b, "%s: %s (e.g. %s)\n", k.Name, k.Description, k.Example)
		}
	}
	b.WriteString("A place is its name alone, never a sentence or a question. Leave a value \"\" when the requests do not say it. " +
		"Fill in this JSON and write nothing else:\n{")
	for i, k := range kinds {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q: \"\"", k)
	}
	b.WriteString("}\n")
	return b.String()
}

var (
	userContextTime     = regexp.MustCompile(`^\d{1,2}:\d{2}$`)
	userContextDate     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	userContextDuration = regexp.MustCompile(`^(\d+h)?(\d+m)?(\d+s)?$`)
	userContextNumber   = regexp.MustCompile(`^\d+$`)
)

// timeSaid: HH:MM is a time a request says — its hour said as 「H時」 or the
// time as it is.
func timeSaid(v string, list []userRequest) bool {
	hm := strings.SplitN(v, ":", 2)
	h := strings.TrimLeft(hm[0], "0")
	if h == "" {
		h = "0"
	}
	for _, r := range list {
		said := toHalfWidthDigits(r.Said)
		if strings.Contains(said, v) || strings.Contains(said, h+"時") || strings.Contains(said, h+":"+hm[1]) {
			return true
		}
	}
	return false
}

// dateSaid: YYYY-MM-DD is a day a request says — 今日, 明日, 明後日 from now,
// or its month and day (11月26日, 11/26).
func dateSaid(v string, list []userRequest, now time.Time) bool {
	d, err := time.ParseInLocation("2006-01-02", v, now.Location())
	if err != nil {
		return false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	words := map[int][]string{0: {"今日", "きょう"}, 1: {"明日", "あした", "あす"}, 2: {"明後日", "あさって"}}
	md := []string{fmt.Sprintf("%d月%d日", d.Month(), d.Day()), fmt.Sprintf("%d/%d", d.Month(), d.Day())}
	for _, r := range list {
		said := toHalfWidthDigits(r.Said)
		for days, said2 := range words {
			if !today.AddDate(0, 0, days).Equal(d) {
				continue
			}
			for _, w := range said2 {
				// 「明後日」 holds 「明日」 too: only 明日 when it is not 明後日.
				if strings.Contains(said, w) && !(days == 1 && strings.Contains(said, "明後日")) {
					return true
				}
			}
		}
		for _, w := range md {
			if strings.Contains(said, w) {
				return true
			}
		}
	}
	return false
}

// toHalfWidthDigits writes ０-９ as 0-9.
func toHalfWidthDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return r - '０' + '0'
		}
		return r
	}, s)
}

// userContextModes are preference.mode's values, with words that say each.
var userContextModes = map[string][]string{
	"walking":   {"歩", "徒歩"},
	"cycling":   {"自転車", "チャリ"},
	"driving":   {"車", "ドライブ"},
	"transit":   {"電車", "乗り換え", "乗換"},
}

// contextValueOK: v, read by the model for kind, is what the person said or a
// want was made with, in the kind's form.
func contextValueOK(kind, v string, list []userRequest, thing func(string) bool, now time.Time) bool {
	made := func(v string) bool {
		for _, r := range list {
			for _, pv := range r.Params {
				if pv == v {
					return true
				}
			}
		}
		return false
	}
	saidWord := func(v string) bool {
		for _, r := range list {
			if strings.Contains(toHalfWidthDigits(r.Said), v) {
				return true
			}
		}
		return false
	}
	switch {
	case strings.HasPrefix(kind, "place."):
		// A place only as a want was made with it, or a thing on the board
		// they said: the model wrote a whole request as the destination
		// (2026-11-25).
		return made(v) || (thing != nil && thing(v) && saidWord(v))
	case kind == "time.date":
		return userContextDate.MatchString(v) && (made(v) || dateSaid(v, list, now))
	case kind == "time.duration":
		if !userContextDuration.MatchString(v) || v == "" {
			return false
		}
		if made(v) {
			return true
		}
		for _, n := range regexp.MustCompile(`\d+`).FindAllString(v, -1) {
			if !saidWord(n) {
				return false
			}
		}
		return true
	case strings.HasPrefix(kind, "time."):
		// Only a time they said (「10時」, 「9:40」).
		return userContextTime.MatchString(v) && (made(v) || timeSaid(v, list))
	case kind == "people.party_size", kind == "preference.budget":
		return userContextNumber.MatchString(v) && (made(v) || saidWord(v))
	case kind == "preference.mode":
		words, ok := userContextModes[v]
		if !ok {
			return false
		}
		if made(v) {
			return true
		}
		for _, w := range words {
			if saidWord(w) {
				return true
			}
		}
		return false
	case kind == "interest.topic":
		return len([]rune(v)) <= 20
	default:
		// people.who, interest.subject, preference.cuisine: their words.
		return len([]rune(v)) <= 20 && (made(v) || saidWord(v))
	}
}

// madeContext is what the wants made say of each kind, the latest first: a
// kind the model left empty, or wrote what was not said for, is the last
// want's made with it — 「荻窪から吉祥寺」 then 「スマートゴルフの予約は？」 is
// still on the way to 吉祥寺, where the model wrote スマートゴルフ (2026-11-25).
func madeContext(list []userRequest, kindOf func(typ, param string) string) map[string]string {
	out := map[string]string{}
	for i := len(list) - 1; i >= 0; i-- {
		r := list[i]
		if r.Type == "" || kindOf == nil {
			continue
		}
		for p, v := range r.Params {
			if k := kindOf(r.Type, p); k != "" && out[k] == "" && v != "" {
				out[k] = v
			}
		}
	}
	return out
}

// userContextRead reads the model's lines: each kind asked whose value
// contextValueOK keeps (a station as said, 「国分寺駅」 is 国分寺).
func userContextRead(text string, list []userRequest, kinds []string, thing func(string) bool, now time.Time) map[string]string {
	asked := map[string]bool{}
	for _, k := range kinds {
		asked[k] = true
	}
	out := map[string]string{}
	keep := func(kind, v string) {
		v = strings.Trim(strings.TrimSpace(v), "\"'「」`")
		if !asked[kind] || v == "" || v == "-" || strings.EqualFold(v, "none") || strings.EqualFold(v, "unknown") {
			return
		}
		if strings.HasPrefix(kind, "place.") {
			if t := strings.TrimSuffix(v, "駅"); t != v && contextValueOK(kind, t, list, thing, now) {
				v = t
			}
		}
		if contextValueOK(kind, v, list, thing, now) {
			out[kind] = v
		}
	}
	// The JSON asked for (in a code fence or not), its values any type.
	if i, j := strings.Index(text, "{"), strings.LastIndex(text, "}"); i >= 0 && j > i {
		var obj map[string]any
		if json.Unmarshal([]byte(text[i:j+1]), &obj) == nil {
			for k, v := range obj {
				if v != nil {
					keep(strings.ToLower(strings.TrimSpace(k)), fmt.Sprint(v))
				}
			}
			return out
		}
	}
	// Else lines, kind=value.
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*・ "))
		i := strings.Index(line, "=")
		if i < 0 {
			i = strings.IndexAny(line, ":：")
		}
		if i < 0 {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(line[:i]))
		keep(kind, strings.TrimPrefix(strings.TrimSpace(line[i+1:]), "="))
	}
	return out
}

// robotContextParams fills what who did not say with their context, by each
// parameter's context field (want-spec ParameterDef.Context: a route's to is
// the destination), and says what it filled; "" when nothing. Never the
// here-param (where the person is now).
//
// A parameter with a list of values (validation.enum) takes only one of them:
// a route's mode is walking, cycling or driving, not transit.
func (s *Server) robotContextParams(contextOf map[string]string, here string, params map[string]any, who string, enums map[string][]any) string {
	if len(contextOf) == 0 {
		return ""
	}
	c, ok := userContextNow(who)
	if !ok {
		return ""
	}
	var names []string
	for p := range contextOf {
		names = append(names, p)
	}
	sort.Strings(names)
	filled := ""
	for _, p := range names {
		if p == here {
			continue
		}
		if v, said := params[p]; said && v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" {
			continue
		}
		field := contextOf[p]
		if v := c.Fields[field]; v != "" && inEnum(v, enums[p]) {
			params[p] = v
			filled += fmt.Sprintf(" (%s=%s: the person's %s now, from what they asked before, as none was said)", p, v, field)
		}
	}
	return filled
}

// inEnum: v is one of enum, or there is no enum.
func inEnum(v string, enum []any) bool {
	if len(enum) == 0 {
		return true
	}
	for _, e := range enum {
		if fmt.Sprint(e) == v {
			return true
		}
	}
	return false
}

// clearUserContexts forgets every character's context, in the global state
// and the requests kept for it: the robot's chat cleared is the conversation
// ending (clearWantChatSession). Says how many keys went.
func clearUserContexts() int {
	userContexts.Lock()
	for _, t := range userContexts.pending {
		t.Stop()
	}
	userContexts.asked = map[string][]userRequest{}
	userContexts.pending = map[string]*time.Timer{}
	userContexts.Unlock()
	n := 0
	all := mywant.GetAllGlobalState()
	for _, ch := range mywant.ListCharacters() {
		for _, name := range []string{ch.Name, ch.ID} {
			if name == "" {
				continue
			}
			for _, k := range append(contextKindNames(), userContextAt) {
				if _, ok := all[name+"."+k]; ok {
					mywant.DeleteGlobalStateKey(name + "." + k)
					n++
				}
			}
		}
	}
	return n
}

// contextKindNames are every kind's name.
func contextKindNames() []string {
	var out []string
	for _, k := range ws.ContextKinds {
		out = append(out, k.Name)
	}
	return out
}

// userContextText is who's context as a line for the robot's brief; "" none.
func userContextText(who string) string {
	c, ok := userContextNow(who)
	if !ok {
		return ""
	}
	var kv []string
	for _, k := range ws.ContextKinds {
		if v := c.Fields[k.Name]; v != "" {
			kv = append(kv, k.Name+"="+v)
		}
	}
	if len(kv) == 0 {
		return ""
	}
	return fmt.Sprintf("What the person is about now, from their last requests (use it only for what they leave unsaid): %s\n\n", strings.Join(kv, ", "))
}
