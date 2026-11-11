package mywant

import (
	"strings"

	want_spec "github.com/onelittlenightmusic/want-spec"
)

// When a want tells of its answer: murmuring (its tile's "!", said when
// someone comes to it) and notifying (a push to the phone), each at the
// moments its type names (timings) and the want chooses.
//
// A type names the moments of its answer worth telling of — a player's track
// changing, its album changing, its starting to play; a position coming near
// a saved place. The want says one has come from its Progress with the
// standard function Timing("album") (a plugin script: {"_timing": "album"});
// a timing that names a field that must hold (if) or must have moved since the
// last answer (onChange) is also read off every answer kept (answerTimings),
// with no code. Each answer carries the timings it came at.
//
// A want chooses, in its labels, which it murmurs at (murmur) and which it
// notifies at (notify): a comma list of timing names, "all", or "none". With
// no label, the type's own say (a timing's murmur / notify). A type with no
// timings tells of every answer, as before.
//
// The history is not this: every answer the type keeps is kept, told of or
// not — a player's every song, murmured only when the album changes.

// Labels a want chooses its timings in.
const (
	MurmurLabel = "murmur"
	NotifyLabel = "notify"
)

// Timing is the standard way a want says, from its Progress, that its answer
// has come at one of its type's timings — "album": the album changed — for it
// to be murmured and notified of as the want chose. Taken with the next answer
// kept; with no new answer by the end of the cycle, told of with the answer it
// has. A plugin script says the same with a line {"_timing": "album"}
// (mywant.current().timing("album")).
func (n *Want) Timing(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	known := false
	for _, d := range n.typeTimings() {
		known = known || d.Name == name
	}
	if !known {
		n.StoreLog("[TIMING] %q is not one of its type's timings; told of all the same", name)
	}
	n.timingMu.Lock()
	n.pendingTimings = append(n.pendingTimings, name)
	n.timingMu.Unlock()
}

// takeTimings is the timings said since they were last taken.
func (n *Want) takeTimings() []string {
	n.timingMu.Lock()
	defer n.timingMu.Unlock()
	out := n.pendingTimings
	n.pendingTimings = nil
	return out
}

func (n *Want) hasPendingTimings() bool {
	n.timingMu.Lock()
	defer n.timingMu.Unlock()
	return len(n.pendingTimings) > 0
}

// withTimings: a and b, each once, in order.
func withTimings(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range append(append([]string{}, a...), b...) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// answerTimings is the type's timings this answer comes at, and remembers
// each onChange field's value for the next. The first time (a restart), the
// values before are those of prev, the answer kept before it.
func (n *Want) answerTimings(fields map[string]any, prev *ResultHistoryEntry) []string {
	defs := n.typeTimings()
	if len(defs) == 0 {
		return nil
	}
	read := func(field string) any {
		if v, ok := fields[field]; ok {
			return v
		}
		if v, ok := resolveNestedStateField(n, field); ok && !isEmptyAnswer(v) {
			return v
		}
		return nil
	}
	if n.timingSeen == nil {
		n.timingSeen = map[string]any{}
		if prev != nil {
			for _, d := range defs {
				if d.OnChange != "" {
					n.timingSeen[d.OnChange] = prev.Fields[d.OnChange]
				}
			}
		}
	}
	now := map[string]any{}
	var at []string
	for _, d := range defs {
		if d.If == "" && d.OnChange == "" {
			continue // said only by the want itself (Timing)
		}
		if d.If != "" && !truthyAnswerField(read(d.If)) {
			continue
		}
		if d.OnChange != "" {
			v, ok := now[d.OnChange]
			if !ok {
				v = read(d.OnChange)
				now[d.OnChange] = v
			}
			if sameAnswerResult(n.timingSeen[d.OnChange], v) {
				continue
			}
		}
		at = append(at, d.Name)
	}
	// Every onChange field is read and kept, whether or not its timing held.
	for _, d := range defs {
		if d.OnChange == "" {
			continue
		}
		if _, ok := now[d.OnChange]; !ok {
			now[d.OnChange] = read(d.OnChange)
		}
		n.timingSeen[d.OnChange] = now[d.OnChange]
	}
	return at
}

// HasTimings: its type names timings to tell of its answers at.
func (n *Want) HasTimings() bool { return len(n.typeTimings()) > 0 }

func (n *Want) typeTimings() []TimingDef {
	if n.WantTypeDefinition == nil {
		return nil
	}
	return n.WantTypeDefinition.Timings
}

// TimingDef is one moment of a type's answer worth telling of.
type TimingDef = want_spec.TimingDef

// Telling is how a want tells of one answer.
type Telling struct {
	// The type's timings the answer came at.
	Timings []string
	Murmur  bool
	Notify  bool
	// NotifyChosen: the want named its notify timings itself — its own
	// choice, not a default for Laya to weigh.
	NotifyChosen bool
}

// TellingOf says whether the want murmurs and notifies of this answer.
func (n *Want) TellingOf(entry ResultHistoryEntry) Telling {
	defs := n.typeTimings()
	if len(defs) == 0 {
		return Telling{Murmur: true, Notify: true}
	}
	labels := n.GetLabels()
	t := Telling{Timings: entry.Timings}
	murmurAt, _ := chosenTimings(labels[MurmurLabel], defs, func(d TimingDef) bool { return d.Murmur })
	notifyAt, chosen := chosenTimings(labels[NotifyLabel], defs, func(d TimingDef) bool { return d.Notify })
	t.NotifyChosen = chosen
	for _, name := range entry.Timings {
		t.Murmur = t.Murmur || murmurAt[name]
		t.Notify = t.Notify || notifyAt[name]
	}
	return t
}

// chosenTimings reads a want's choice: names, "all" or "none"; with none
// written, the type's defaults. chosen says the want wrote one.
func chosenTimings(label string, defs []TimingDef, byDefault func(TimingDef) bool) (map[string]bool, bool) {
	out := map[string]bool{}
	label = strings.TrimSpace(label)
	if label == "" {
		for _, d := range defs {
			if byDefault(d) {
				out[d.Name] = true
			}
		}
		return out, false
	}
	for _, name := range strings.Split(label, ",") {
		name = strings.TrimSpace(name)
		switch name {
		case "", "none":
		case "all":
			for _, d := range defs {
				out[d.Name] = true
			}
		default:
			out[name] = true
		}
	}
	return out, true
}
