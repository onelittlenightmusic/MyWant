package server

// story_digest.go — a day of a character's story, kept for good
// (want-spec's StoryDigest) after the day's events are let go (story.go:
// gzipped after storyRawDays, gone after storyGzipDays).
//
// How the day went: the wants visited and the moves the person made between
// them, the day's events by kind and its answers by outcome. Each judged
// answer itself — the question, the plan, how it went — is the ledger's
// (story_ledger.go), kept in one place for good to be searched; a digest
// leaves its turns empty.
//
//	~/.mywant/stories/<character>/digest/<YYYY-MM-DD>.yaml
//
// A day is digested once it is over and its last turns have been judged: from
// storyDigestAfter past the next midnight (a turn answered at 23:59 is judged
// after it). Days with events and no digest yet are digested then, so the
// days kept from before this was written are digested too.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	ws "github.com/onelittlenightmusic/want-spec"
	"gopkg.in/yaml.v3"
)

// storyDigestAfter: how long past midnight a day waits to be digested — its
// last answers' time to be judged (storyJudgeWithin), with room.
const storyDigestAfter = 15 * time.Minute

const storyDigestDir = "digest"

// digestDue is the newest day that can be digested now: the day before the
// one storyDigestAfter ago.
func (r *storyRecorder) digestDue() string {
	return r.now().In(time.Local).Add(-storyDigestAfter).AddDate(0, 0, -1).Format("2006-01-02")
}

// digestDays writes the digest of each character's days that are due and
// have none yet.
func (r *storyRecorder) digestDays() error {
	root := r.root()
	characters, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	due := r.digestDue()
	var firstErr error
	for _, c := range characters {
		if !c.IsDir() || !storyCharacterOK.MatchString(c.Name()) {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, c.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			day := strings.TrimSuffix(strings.TrimSuffix(f.Name(), ".gz"), ".jsonl")
			if day == f.Name() || day > due {
				continue
			}
			path := r.digestPath(c.Name(), day)
			if _, err := os.Stat(path); err == nil {
				continue
			}
			d, err := r.digest(c.Name(), day)
			if err == nil {
				err = writeStoryDigest(path, d)
			}
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("digest %s %s: %w", c.Name(), day, err)
			}
		}
	}
	return firstErr
}

func (r *storyRecorder) digestPath(character, day string) string {
	return filepath.Join(r.root(), character, storyDigestDir, day+".yaml")
}

func writeStoryDigest(path string, d *ws.StoryDigest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(d)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// digests is a character's digests for the days from since to until
// (YYYY-MM-DD, both kept): those written, and a day not yet digested (today)
// worked out from its events as they are, not written.
func (r *storyRecorder) digests(character, since, until string) ([]ws.StoryDigest, error) {
	if !storyCharacterOK.MatchString(character) {
		return nil, fmt.Errorf("bad character %q", character)
	}
	from, err := time.ParseInLocation("2006-01-02", since, time.Local)
	if err != nil {
		return nil, fmt.Errorf("since: %w", err)
	}
	to, err := time.ParseInLocation("2006-01-02", until, time.Local)
	if err != nil {
		return nil, fmt.Errorf("until: %w", err)
	}
	var out []ws.StoryDigest
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		day := d.Format("2006-01-02")
		if data, err := os.ReadFile(r.digestPath(character, day)); err == nil {
			var dg ws.StoryDigest
			if err := yaml.Unmarshal(data, &dg); err != nil {
				return nil, fmt.Errorf("%s: %w", day, err)
			}
			out = append(out, dg)
			continue
		}
		dg, err := r.digest(character, day)
		if err != nil {
			return nil, err
		}
		if len(dg.Counts) > 0 {
			out = append(out, *dg)
		}
	}
	return out, nil
}

// digest works a day out from its events — on disk, and held when the day
// is today — with the outcomes decided early the next day for its turns.
func (r *storyRecorder) digest(character, day string) (*ws.StoryDigest, error) {
	start, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return nil, err
	}
	events, err := r.read(character, storyQuery{since: start, until: start.AddDate(0, 0, 1)})
	if err != nil {
		return nil, err
	}
	// The outcomes of the day's turns decided after midnight.
	next, err := r.read(character, storyQuery{since: start.AddDate(0, 0, 1),
		until: start.AddDate(0, 0, 1).Add(storyDigestAfter), kinds: map[string]bool{ws.StoryKindTurnOutcome: true}})
	if err != nil {
		return nil, err
	}
	return buildStoryDigest(character, day, events, next), nil
}

// buildStoryDigest is the digest of a day's events (oldest first); late, the
// outcomes decided for its turns after it.
func buildStoryDigest(character, day string, events, late []ws.StoryEvent) *ws.StoryDigest {
	d := &ws.StoryDigest{Character: character, Date: day, Turns: []ws.StoryTurn{},
		Transitions: []ws.StoryTransition{}, Visits: []ws.StoryVisit{},
		Counts: map[string]int{}, Outcomes: map[string]int{}}

	turns := map[string]*ws.StoryTurn{}
	var order []string
	var open string // the turn asked and not yet answered, for an act that does not name its turn
	visits := map[string]*ws.StoryVisit{}
	var visitOrder []string
	onSince := map[string]time.Time{}
	transitions := map[[2]string]*ws.StoryTransition{}
	var transitionOrder [][2]string

	visit := func(p *ws.StoryPlace) *ws.StoryVisit {
		v := visits[p.ID]
		if v == nil {
			v = &ws.StoryVisit{Place: storyPlaceOnly(p)}
			visits[p.ID] = v
			visitOrder = append(visitOrder, p.ID)
		}
		return v
	}
	at := func(ev ws.StoryEvent) time.Time {
		t, _ := time.Parse(time.RFC3339, ev.At)
		return t
	}
	outcome := func(ev ws.StoryEvent) {
		t := turns[ev.Ref]
		if t == nil {
			return
		}
		t.Outcome = ev.Detail["outcome"]
		if t.Outcome == ws.TurnOutcomeCorrected && ev.To != nil {
			p := storyPlaceOnly(ev.To)
			t.CorrectedTo = &p
		}
		if s, err := strconv.Atoi(ev.Detail["seconds"]); err == nil {
			t.SecondsToAct = s
		}
		if t.Plan == nil && ev.Detail["plan.action"] != "" {
			t.Plan = &ws.PlanSpec{Question: t.Question, Action: ev.Detail["plan.action"], Target: ev.Detail["plan.target"],
				Capability: ev.Detail["plan.capability"], Type: ev.Detail["plan.type"]}
		}
	}

	for _, ev := range events {
		d.Counts[ev.Kind]++
		switch ev.Kind {
		case ws.StoryKindChatAsk:
			turns[ev.Ref] = &ws.StoryTurn{Turn: ev.Ref, At: ev.At, Question: ev.Detail["question"]}
			order = append(order, ev.Ref)
			open = ev.Ref
		case ws.StoryKindActDone:
			ref := ev.Detail["turn"]
			if ref == "" {
				ref = open
			}
			if t := turns[ref]; t != nil {
				t.Plan = storyPlanOf(ev.Detail, t.Question)
			}
		case ws.StoryKindChatAnswer:
			if t := turns[ev.Ref]; t != nil {
				if cards := ev.Detail["cards"]; cards != "" {
					t.Shown = strings.Split(cards, ",")
				}
			}
			if open == ev.Ref {
				open = ""
			}
		case ws.StoryKindTurnOutcome:
			outcome(ev)
		case ws.StoryKindWantArrive:
			if ev.To == nil || ev.To.ID == "" {
				break
			}
			visit(ev.To).Arrived++
			onSince[ev.To.ID] = at(ev)
			// A move the person made: one an answer brought about (via=answer)
			// is the answer's, not where they go from where.
			if ev.Detail["via"] != "answer" && ev.From != nil && ev.From.ID != "" && ev.From.ID != ev.To.ID &&
				(ev.From.Kind == "want" || ev.From.Kind == "thing") {
				k := [2]string{ev.From.ID, ev.To.ID}
				tr := transitions[k]
				if tr == nil {
					tr = &ws.StoryTransition{From: storyPlaceOnly(ev.From), To: storyPlaceOnly(ev.To)}
					transitions[k] = tr
					transitionOrder = append(transitionOrder, k)
				}
				tr.Count++
			}
		case ws.StoryKindWantLeave:
			if ev.From == nil || ev.From.ID == "" {
				break
			}
			if since, ok := onSince[ev.From.ID]; ok {
				visit(ev.From).Seconds += int(at(ev).Sub(since).Round(time.Second) / time.Second)
				delete(onSince, ev.From.ID)
			}
		case ws.StoryKindWantOpen, ws.StoryKindWantExpand:
			if ev.To != nil && ev.To.ID != "" {
				visit(ev.To).Opened++
			}
		}
	}
	for _, ev := range late {
		outcome(ev)
	}
	for _, ref := range order {
		if t := turns[ref]; t.Outcome != "" {
			d.Outcomes[t.Outcome]++
		}
	}
	for _, id := range visitOrder {
		d.Visits = append(d.Visits, *visits[id])
	}
	for _, k := range transitionOrder {
		d.Transitions = append(d.Transitions, *transitions[k])
	}
	sort.SliceStable(d.Transitions, func(i, j int) bool { return d.Transitions[i].Count > d.Transitions[j].Count })
	return d
}

// storyPlaceOnly is a place without its cell: what it is, not where it stood.
func storyPlaceOnly(p *ws.StoryPlace) ws.StoryPlace {
	return ws.StoryPlace{Kind: p.Kind, ID: p.ID, Name: p.Name, Type: p.Type}
}

// storyPlanOf is an act.done's detail as the plan it carried out.
func storyPlanOf(detail map[string]string, question string) *ws.PlanSpec {
	p := &ws.PlanSpec{Question: question, Action: detail["action"], Target: detail["target"],
		Capability: detail["capability"], Type: detail["type"]}
	for k, v := range detail {
		if name, ok := strings.CutPrefix(k, "param."); ok {
			if p.Params == nil {
				p.Params = map[string]string{}
			}
			p.Params[name] = v
		}
	}
	return p
}
