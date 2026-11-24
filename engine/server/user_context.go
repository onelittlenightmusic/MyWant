package server

// user_context.go — where the person is going, read from what they asked.
//
// 「何時に着く？」「天気は？」 said a minute after 「国分寺までの乗り換え」 are
// about 国分寺. Each person's last requests (userContextKeep, this hour) are
// read, after one is answered, by Apple's model into a few fixed fields:
//
//	{"destination": "国分寺", "origin": "中野坂上", "arrive_by": "10:00", "topic": "移動"}
//
// kept per character (~/.mywant/contexts/<character>.json). A value is kept
// only when it is words the person said (or a want was made with): the model
// is small and fills a field it was asked for. Used two ways:
//
//   - a parameter not said is filled from it, by the type's labels
//     (context/<param>: <field> — transit_search's context/to: destination),
//     before home's (home_params.go): what was said, then this, then home;
//   - the robot's brief (plan.go) carries it as a line, for the model that
//     answers.
//
// It is worked out after a request, not while one waits: the next one uses it.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	userContextKeep   = 10               // requests read
	userContextWindow = 60 * time.Minute // older than this, not now's
	userContextAfter  = 4 * time.Second  // quiet this long after a request, it is read
	userContextWait   = 30 * time.Second // the model, waited for
	userContextLabel  = "context/"       // context/<param>: <field>
)

// userContextFields are what the model is asked for, in order.
var userContextFields = []string{"destination", "origin", "arrive_by", "topic"}

// userRequest is one thing a person asked, and the want made for it.
type userRequest struct {
	At     time.Time         `json:"at"`
	Said   string            `json:"said"`
	Type   string            `json:"type,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

// userContext is what a person is about now.
type userContext struct {
	Fields    map[string]string `json:"fields"`
	UpdatedAt time.Time         `json:"updated_at"`
	From      int               `json:"from_requests"`
	Requests  []userRequest     `json:"requests,omitempty"`
}

var userContexts = struct {
	sync.Mutex
	asked   map[string][]userRequest
	now     map[string]userContext
	pending map[string]*time.Timer
}{asked: map[string][]userRequest{}, now: map[string]userContext{}, pending: map[string]*time.Timer{}}

func userContextFile(who string) string {
	return initiativeHomeFile(filepath.Join("contexts", who+".json"))
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
	if len(list) == 0 {
		list = loadUserContext(who).Requests
	}
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

func loadUserContext(who string) userContext {
	var c userContext
	if b, err := os.ReadFile(userContextFile(who)); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

// userContextNow is who's context, when it is of this hour.
func userContextNow(who string) (userContext, bool) {
	who = strings.TrimSpace(who)
	if who == "" {
		return userContext{}, false
	}
	userContexts.Lock()
	c, ok := userContexts.now[who]
	if !ok {
		c = loadUserContext(who)
		userContexts.now[who] = c
	}
	userContexts.Unlock()
	return c, len(c.Fields) > 0 && time.Since(c.UpdatedAt) <= userContextWindow
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
	prompt := userContextPrompt(list)
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
	c := userContext{Fields: userContextRead(text, list, func(v string) bool { return things[v] }), UpdatedAt: time.Now(), From: len(list), Requests: list}
	order := []string{"who", "requests"}
	for _, f := range userContextFields {
		if v := c.Fields[f]; v != "" {
			trace[f] = v
			order = append(order, f)
		}
	}
	s.robotChatTrace("fm.context", trace, append(order, "by", "ms"))
	userContexts.Lock()
	userContexts.now[who] = c
	userContexts.Unlock()
	if b, err := json.MarshalIndent(c, "", "  "); err == nil {
		_ = os.MkdirAll(filepath.Dir(userContextFile(who)), 0o755)
		_ = os.WriteFile(userContextFile(who), b, 0o644)
	}
}

// userContextPrompt: the requests, newest first, with what was made of each.
// No clock times: the model took one for arrive_by (2026-11-25).
func userContextPrompt(list []userRequest) string {
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
				kv = append(kv, k+"="+v)
			}
			sort.Strings(kv)
			fmt.Fprintf(&b, " → %s %s", r.Type, strings.Join(kv, " "))
		}
		b.WriteString("\n")
	}
	b.WriteString("Say what the person is about now. When requests differ, the latest one (1) is right — they changed their mind:\n" +
		"destination: the place they are going to\n" +
		"origin: the place they are leaving from\n" +
		"arrive_by: the time (HH:MM) they must be there by\n" +
		"topic: what they are doing, in a word or two\n" +
		"A place is its name alone (国分寺), never a sentence or a question. Leave a line empty when the requests do not say it. " +
		"Fill in these lines and write nothing else:\n")
	for _, f := range userContextFields {
		b.WriteString(f + "=\n")
	}
	return b.String()
}

var userContextTime = regexp.MustCompile(`^\d{1,2}:\d{2}$`)

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

// toHalfWidthDigits writes ０-９ as 0-9.
func toHalfWidthDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return r - '０' + '0'
		}
		return r
	}, s)
}

// userContextRead reads the model's lines, a place kept only when a want was
// made with it, or it is a thing named on the board that the person said —
// the model wrote a whole request as the destination (2026-11-25) — and a
// time only as HH:MM.
func userContextRead(text string, list []userRequest, thing func(string) bool) map[string]string {
	said := func(v string) bool {
		for _, r := range list {
			for _, pv := range r.Params {
				if pv == v {
					return true
				}
			}
			if thing != nil && thing(v) && strings.Contains(r.Said, v) {
				return true
			}
		}
		return false
	}
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*・ "))
		i := strings.IndexAny(line, "=:")
		if i < 0 {
			continue
		}
		f := strings.ToLower(strings.TrimSpace(line[:i]))
		v := strings.Trim(strings.TrimSpace(line[i+1:]), "\"'「」`")
		if v == "" || strings.EqualFold(v, "none") || strings.EqualFold(v, "unknown") {
			continue
		}
		switch f {
		case "destination", "origin":
			if t := strings.TrimSuffix(v, "駅"); t != v && said(t) {
				v = t
			}
			if said(v) {
				out[f] = v
			}
		case "arrive_by":
			// Only a time they said (「10時」, 「9:40」).
			if userContextTime.MatchString(v) && timeSaid(v, list) {
				out[f] = v
			}
		case "topic":
			if len([]rune(v)) <= 20 {
				out[f] = v
			}
		}
	}
	return out
}

// robotContextParams fills what who did not say with their context, by the
// type's context/<param> labels, and says what it filled; "" when nothing.
// Never the here-param (where the person is now).
func (s *Server) robotContextParams(labels map[string]string, params map[string]any, who string) string {
	c, ok := userContextNow(who)
	if !ok {
		return ""
	}
	filled := ""
	var names []string
	for k := range labels {
		if strings.HasPrefix(k, userContextLabel) {
			names = append(names, strings.TrimPrefix(k, userContextLabel))
		}
	}
	sort.Strings(names)
	for _, p := range names {
		if p == labels[robotHereLabel] {
			continue
		}
		if v, said := params[p]; said && v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" {
			continue
		}
		field := strings.TrimSpace(labels[userContextLabel+p])
		if v := c.Fields[field]; v != "" {
			params[p] = v
			filled += fmt.Sprintf(" (%s=%s: the person's %s now, from what they asked before, as none was said)", p, v, field)
		}
	}
	return filled
}

// clearUserContexts forgets every person's context: the robot's chat cleared
// is the conversation ending (clearWantChatSession).
func clearUserContexts() int {
	userContexts.Lock()
	defer userContexts.Unlock()
	for _, t := range userContexts.pending {
		t.Stop()
	}
	userContexts.asked = map[string][]userRequest{}
	userContexts.now = map[string]userContext{}
	userContexts.pending = map[string]*time.Timer{}
	files, _ := filepath.Glob(initiativeHomeFile(filepath.Join("contexts", "*.json")))
	for _, f := range files {
		_ = os.Remove(f)
	}
	return len(files)
}

// userContextText is who's context as a line for the robot's brief; "" none.
func userContextText(who string) string {
	c, ok := userContextNow(who)
	if !ok {
		return ""
	}
	var kv []string
	for _, f := range userContextFields {
		if v := c.Fields[f]; v != "" {
			kv = append(kv, f+"="+v)
		}
	}
	if len(kv) == 0 {
		return ""
	}
	return fmt.Sprintf("What the person is about now, from their last requests (use it only for what they leave unsaid): %s\n\n", strings.Join(kv, ", "))
}
