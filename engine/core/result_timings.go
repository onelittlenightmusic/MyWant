package mywant

import (
	"encoding/json"
	"reflect"
	"strconv"
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

// Labels a want chose its timings in before spec.voice (WantSpec.Voice), still
// read when it has none.
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

// answerTimings is the type's timings this answer comes at, with what changed
// in the fields they compared (want_change.go), and remembers each watched
// field's value (onChange, onIncrease, onDecrease) for the next. The first
// time (a restart), the values before are those of prev, the answer kept
// before it.
func (n *Want) answerTimings(fields map[string]any, prev *ResultHistoryEntry) ([]string, []Change) {
	defs := n.typeTimings()
	if len(defs) == 0 {
		return nil, nil
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
				if field, sub, ok := allCondition(d.If); ok {
					v, has := prev.Fields[field]
					if !has && field == n.Spec.FinalResultField {
						v = prev.Result
					}
					n.timingSeen["if:"+d.If] = allHold(v, sub)
				}
				for _, f := range watchedFields(d) {
					src, sub, counted := countCondition(f)
					if !counted {
						src = f
					}
					v, ok := prev.Fields[src]
					if !ok && src == n.Spec.FinalResultField {
						// The answer itself (a checklist's items): kept as
						// the entry's result, not among its fields.
						v = prev.Result
					}
					if counted {
						v = countHold(v, sub)
					}
					n.timingSeen[f] = v
				}
			}
		}
	}
	now := map[string]any{}
	value := func(field string) any {
		v, ok := now[field]
		if !ok {
			if list, sub, counted := countCondition(field); counted {
				// "count:items.done": how many items are so.
				v = countHold(read(list), sub)
			} else {
				v = read(field)
			}
			now[field] = v
		}
		return v
	}
	var at []string
	var changes []Change
	changed := map[string]bool{}
	for _, d := range defs {
		if d.If == "" && len(watchedFields(d)) == 0 {
			continue // said only by the want itself (Timing)
		}
		if field, sub, ok := allCondition(d.If); ok {
			// "all:items.done": every item of a list so, and not so before —
			// told once, as the last is ticked off.
			now := allHold(value(field), sub)
			before, _ := n.timingSeen["if:"+d.If].(bool)
			n.timingSeen["if:"+d.If] = now
			if !now || before {
				continue
			}
		} else if d.If != "" && !truthyAnswerField(read(d.If)) {
			continue
		}
		if d.OnChange != "" && sameAnswerResult(n.timingSeen[d.OnChange], value(d.OnChange)) {
			continue
		}
		if d.OnIncrease != "" && !grew(n.timingSeen[d.OnIncrease], value(d.OnIncrease), 1) {
			continue
		}
		if d.OnDecrease != "" && !grew(n.timingSeen[d.OnDecrease], value(d.OnDecrease), -1) {
			continue
		}
		at = append(at, d.Name)
		for _, f := range watchedFields(d) {
			if changed[f] {
				continue
			}
			_, _, counted := countCondition(f)
			list := (f == d.OnIncrease || f == d.OnDecrease) && !counted
			if c := fieldChange(f, n.timingSeen[f], value(f), list); c != nil {
				changed[f] = true
				changes = append(changes, *c)
			}
		}
	}
	// Every watched field is read and kept, whether or not its timing held.
	for _, d := range defs {
		for _, f := range watchedFields(d) {
			n.timingSeen[f] = value(f)
		}
	}
	return at, changes
}

// watchedFields are the fields a timing compares with their value at the
// last answer.
func watchedFields(d TimingDef) []string {
	var out []string
	for _, f := range []string{d.OnChange, d.OnIncrease, d.OnDecrease} {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// grew says whether a field went the way asked (sign 1: up, -1: down) from
// its value at the last answer — a number, or a list's length. With nothing
// before to compare with (the first answer), or a value that is neither, it
// did not: a want started again is not news that its list grew.
func grew(before, now any, sign int) bool {
	b, ok1 := answerMeasure(before)
	a, ok2 := answerMeasure(now)
	if !ok1 || !ok2 {
		return false
	}
	return (a-b)*float64(sign) > 0
}

// answerMeasure is a field's size: a number as it is (or written as one), a
// list as its length.
func answerMeasure(v any) (float64, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		return float64(rv.Len()), true
	}
	return 0, false
}

// TimingMoments says the moments an answer came at in its type's words (each
// timing's description, else its name), the standard "answer" left out — it
// is every answer, so it says nothing of which moment this one is. "" when
// there is no other.
func (n *Want) TimingMoments(names []string) string {
	defs := n.typeTimings()
	var out []string
	for _, name := range names {
		if name == AnswerTiming {
			continue
		}
		said := name
		for _, d := range defs {
			if d.Name == name && strings.TrimSpace(d.Description) != "" {
				said = strings.TrimSpace(d.Description)
			}
		}
		out = append(out, said)
	}
	return strings.Join(out, "、")
}

// AnswerTiming is the standard timing every type with timings has, declared
// or not: a new answer — what every answer told of before there were
// timings. A type's own timings come on top of it, so declaring "soon" does
// not silence the answers that told of themselves until then; a type that
// tells of only some of its answers (a player's album, not each song)
// declares it with murmur and notify off.
const AnswerTiming = "answer"

// withStandardTimings gives every type that has answers — a result field, or
// timings of its own — the standard one (AnswerTiming) first, telling of
// every new answer, unless it declares it itself: so every want with answers
// can choose whether it murmurs and notifies at them (Want Settings' Timing),
// and a type adds only the moments of its own. A type with no answers has
// nothing to tell of and is left as it is.
func withStandardTimings(def *WantTypeDefinition) {
	if def == nil || (len(def.Timings) == 0 && def.FinalResultField == "") {
		return
	}
	for i, d := range def.Timings {
		if d.Name == AnswerTiming {
			// Declared by the type: its words, the standard glyph if none.
			if d.Icon == "" {
				def.Timings[i].Icon = AnswerTimingIcon
			}
			return
		}
	}
	def.Timings = append([]TimingDef{{
		Name: AnswerTiming, Description: "新しい結果が出たとき", Icon: AnswerTimingIcon, Murmur: true, Notify: true,
	}}, def.Timings...)
}

// AnswerTimingIcon is the standard timing's glyph: a new answer.
const AnswerTimingIcon = "refresh-cw"

// HasTimings: its type names moments of its own to tell of its answers at —
// not only the standard one every type with answers has.
func (n *Want) HasTimings() bool {
	for _, d := range n.typeTimings() {
		if d.Name != AnswerTiming {
			return true
		}
	}
	return false
}

func (n *Want) typeTimings() []TimingDef {
	if n.WantTypeDefinition == nil {
		return nil
	}
	return n.WantTypeDefinition.Timings
}

// TimingDef is one moment of a type's answer worth telling of.
type TimingDef = want_spec.TimingDef

// VoiceSpec is when a want speaks up about its answers (spec.voice).
type VoiceSpec = want_spec.VoiceSpec

// Telling is how a want tells of one answer.
type Telling struct {
	// The type's timings the answer came at.
	Timings []string
	Murmur  bool
	Notify  bool
	// NotifyChosen: the want named its notify timings itself — its own
	// choice, not a default for Laya to weigh.
	NotifyChosen bool
	// Irregular: the timings the answer came at that are out of the ordinary
	// (TimingDef.Irregular: a train late, a reservation changed) — what the
	// robot looks at, never held back as routine.
	Irregular []string
}

// TellingOf says whether the want murmurs and notifies of this answer.
func (n *Want) TellingOf(entry ResultHistoryEntry) Telling {
	defs := n.typeTimings()
	if len(defs) == 0 {
		return Telling{Murmur: true, Notify: true}
	}
	labels := n.GetLabels()
	var murmur, notify *[]string
	if v := n.Spec.Voice; v != nil {
		murmur, notify = v.Murmur, v.Notify
	}
	t := Telling{Timings: entry.Timings}
	for _, name := range entry.Timings {
		for _, d := range defs {
			if d.Name == name && d.Irregular {
				t.Irregular = append(t.Irregular, name)
			}
		}
	}
	murmurAt, _ := voiceTimings(murmur, labels[MurmurLabel], defs, func(d TimingDef) bool { return d.Murmur })
	notifyAt, chosen := voiceTimings(notify, labels[NotifyLabel], defs, func(d TimingDef) bool { return d.Notify })
	t.NotifyChosen = chosen
	for _, name := range entry.Timings {
		t.Murmur = t.Murmur || murmurAt[name]
		t.Notify = t.Notify || notifyAt[name]
	}
	// A notice is a murmur that also goes to the devices: one id, one read
	// state (read on the board, gone from the devices; read there, gone here).
	if t.Notify {
		t.Murmur = true
	}
	return t
}

// voiceTimings is a want's choice of the timings it murmurs or notifies at:
// its spec.voice list (an empty one, none), else — a want written before
// spec.voice — the label of the same name, else the type's defaults. chosen
// says the want made one.
func voiceTimings(list *[]string, label string, defs []TimingDef, byDefault func(TimingDef) bool) (map[string]bool, bool) {
	if list == nil {
		return chosenTimings(label, defs, byDefault)
	}
	out := map[string]bool{}
	for _, name := range *list {
		if name = strings.TrimSpace(name); name != "" {
			out[name] = true
		}
	}
	return out, true
}

// chosenTimings reads a want's choice as a label held it (before spec.voice):
// names, "all" or "none"; with none written, the type's defaults. chosen says
// the want wrote one.
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

// NotifyTimings is the timings the want notifies at now, in its type's order
// (spec.voice.notify, else the older label, else the type's defaults).
func (n *Want) NotifyTimings() []string {
	defs := n.typeTimings()
	var notify *[]string
	if v := n.Spec.Voice; v != nil {
		notify = v.Notify
	}
	at, _ := voiceTimings(notify, n.GetLabels()[NotifyLabel], defs, func(d TimingDef) bool { return d.Notify })
	var out []string
	for _, d := range defs {
		if at[d.Name] {
			out = append(out, d.Name)
		}
	}
	return out
}

// MutedNotify is the want's spec.voice with timings taken out of what it
// notifies at — written as its own list from then on (an empty one is none).
func (n *Want) MutedNotify(timings []string) *VoiceSpec {
	off := map[string]bool{}
	for _, t := range timings {
		off[t] = true
	}
	keep := []string{}
	for _, t := range n.NotifyTimings() {
		if !off[t] {
			keep = append(keep, t)
		}
	}
	v := VoiceSpec{}
	if n.Spec.Voice != nil {
		v = *n.Spec.Voice
	}
	v.Notify = &keep
	return &v
}

// TimingConditions says what made each timing come, in its type's words:
// "onIncrease:reservations", "if:…", "onChange:…" — empty for one that comes
// with every answer (the standard "answer").
func (n *Want) TimingConditions(names []string) []string {
	defs := map[string]TimingDef{}
	for _, d := range n.typeTimings() {
		defs[d.Name] = d
	}
	var out []string
	for _, name := range names {
		d, ok := defs[name]
		if !ok {
			continue
		}
		for _, c := range [][2]string{{"if", d.If}, {"onChange", d.OnChange}, {"onIncrease", d.OnIncrease}, {"onDecrease", d.OnDecrease}} {
			if c[1] != "" {
				out = append(out, c[0]+":"+c[1])
			}
		}
	}
	return out
}

// allCondition reads a timing's if of the form "all:<list>.<field>" — every
// item of the list with the field true.
func allCondition(cond string) (list, field string, ok bool) {
	rest, found := strings.CutPrefix(cond, "all:")
	if !found {
		return "", "", false
	}
	list, field, ok = strings.Cut(rest, ".")
	return list, field, ok && list != "" && field != ""
}

// countCondition reads a watched field of the form "count:<list>.<field>" —
// the number of items of the list with the field true, so onIncrease of it is
// "one more of them is so" (a checklist item ticked: count:items.done).
func countCondition(f string) (list, field string, ok bool) {
	rest, found := strings.CutPrefix(f, "count:")
	if !found {
		return "", "", false
	}
	list, field, ok = strings.Cut(rest, ".")
	return list, field, ok && list != "" && field != ""
}

// countHold is how many items of a list have field true; nil when it is no
// list (nothing to compare with yet).
func countHold(v any, field string) any {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	n := 0
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && truthyAnswerField(m[field]) {
			n++
		}
	}
	return n
}

// allHold says whether a list has items and every one has field true.
func allHold(v any, field string) bool {
	items, ok := v.([]any)
	if !ok || len(items) == 0 {
		return false
	}
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok || !truthyAnswerField(m[field]) {
			return false
		}
	}
	return true
}

// MurmurTimings are the type's timings this want murmurs at — what another
// want can hear of it (spec.hear). A notice is a murmur too.
func (n *Want) MurmurTimings() []TimingDef {
	defs := n.typeTimings()
	labels := n.GetLabels()
	var murmur, notify *[]string
	if v := n.Spec.Voice; v != nil {
		murmur, notify = v.Murmur, v.Notify
	}
	murmurAt, _ := voiceTimings(murmur, labels[MurmurLabel], defs, func(d TimingDef) bool { return d.Murmur })
	notifyAt, _ := voiceTimings(notify, labels[NotifyLabel], defs, func(d TimingDef) bool { return d.Notify })
	var out []TimingDef
	for _, d := range defs {
		if murmurAt[d.Name] || notifyAt[d.Name] {
			out = append(out, d)
		}
	}
	return out
}
