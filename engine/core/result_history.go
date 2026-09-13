package mywant

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// A want's answers, kept as answers.
//
// State history already records every write this want ever made, and that is
// the wrong shape for the question a person actually brings to a want like
// smartgolf_check_reserved: "what did it find, and what did it find before
// that?". One run of a check writes a dozen entries — a script path, a
// percentage, a summary, a raw payload, a reset — and the answer is one of
// them, buried among the machinery that produced it. Reading the answer back
// out of those deltas is guesswork, and guesswork belongs nowhere near a
// history: a reader who reconstructs it has to know which fields a run wrote
// itself and which it merely inherited, and gets it wrong in the case that
// matters most (a want whose last run found nothing still carries the fields
// its last successful run set, so folding deltas forward hands every card the
// same stale reservation).
//
// So the engine keeps the answers itself, shaped once, here. One entry per
// distinct answer, newest last, and nothing in an entry that the run which
// produced it did not itself produce.
//
// It is the one history that outlives the process. The four ring buffers in
// HistoryManager are per-run scratch — state.yaml excludes them (WantHistory is
// `yaml:"-"`) because two hundred snapshots per want would dwarf the state they
// are snapshots of. Answers are different in kind and in size: a handful of
// them, and the only thing a person would be sorry to lose across a
// `mywant restart`. Want.ResultHistory carries them through the file, and
// RestoreResultHistory puts them back in the ring on the way in.

// ResultHistoryEntry is one answer a want arrived at.
type ResultHistoryEntry struct {
	// When this answer first appeared.
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`
	// When it was last confirmed still current. A monitor that re-checks every
	// hour and keeps finding the same thing extends this rather than filling
	// the ring with copies — "found again" is not a new answer.
	LastSeen time.Time `json:"lastSeen,omitempty" yaml:"lastSeen,omitempty"`
	// What the answer is ABOUT, if it says so — an MRS skill that checks
	// something writes its own `checked_at`, and that is the time a reader
	// means. It comes apart from Timestamp by days: a reservation found on the
	// 5th is recorded again by the state restore on the 13th, and leading with
	// the recording time makes that answer look eight days younger than the
	// thing it reports.
	About string `json:"about,omitempty" yaml:"about,omitempty"`
	// The want's own answer: whatever finalResultField names.
	Result any `json:"result,omitempty" yaml:"result,omitempty"`
	// The rest of the answer — the fields the type publishes or keeps, as they
	// stood when this answer arrived. See answerFields.
	Fields map[string]any `json:"fields,omitempty" yaml:"fields,omitempty"`
}

// How many answers a want keeps.
//
// Small on purpose: these are persisted per want in state.yaml, and twenty is
// already more history than any of these wants has ever been asked for.
const resultHistoryDepth = 20

// recordResultHistory notes this want's answer, if it has one and it is new.
//
// Called at the end of EndProgressCycle, after the final result has been
// resolved and after fetchFrom has filled in the fields derived from it, so
// that the entry is the whole answer and not the half of it that happened to
// land first.
func (n *Want) recordResultHistory() {
	field := n.Spec.FinalResultField
	if field == "" {
		// A want with no declared answer has no answers to keep. Everything it
		// did is in the state and log histories, which is the right place for
		// a want whose point is the doing.
		return
	}

	// Same revision guard the fetchFrom expansion above uses: nothing derived
	// from this want's state can have moved unless the state moved, and this
	// runs on every reconcile of every want.
	rev := n.stateRevision.Load()
	if n.resultRecordedOnce && rev == n.resultRecordedAtRevision {
		return
	}
	n.resultRecordedAtRevision = rev
	n.resultRecordedOnce = true

	val, ok := resolveNestedStateField(n, field)
	if !ok || isEmptyAnswer(val) {
		// No answer right now — a check that found nothing, or a want reset by
		// a restart. Deliberately not recorded: "nothing" is not an answer
		// worth a card, and the point of this history is that the answers from
		// before are still there to read when the present one is empty.
		return
	}

	fields := n.answerFields(field)
	now := time.Now()
	n.getHistoryManager().AddResultEntry(ResultHistoryEntry{
		Timestamp: now,
		LastSeen:  now,
		About:     answerAbout(fields),
		Result:    val,
		Fields:    fields,
	})
}

// answerFields is the rest of the answer, beside the result itself.
//
// A field belongs to the answer if its type either publishes it to other wants
// (`exposable`) or keeps it across a restart (`persistent`). Both are the type
// author saying "this is what this want found", and the fields that fail both
// tests are exactly the machinery: where the script is, how long it may take,
// the raw payload the derived fields were cut out of, the percentage and the
// summary that narrate a run while it happens, the reason the last attempt
// failed.
//
// `label: goal` is excluded even when persistent: that is the request, not the
// answer — it is on the card already, and it does not change from one run to
// the next.
//
// The engine's own bookkeeping is excluded by name, because the type
// definition is not only the type author's: a plugin agent injects its state
// defs into typeDef.State (SetWantTypeDefinition), and monitor_mrs_agent's
// include `achieved`, `completed`, `achieving_percentage` and `action_by_agent`
// marked persistent. Those failed no test above and came through — and
// `achieving_percentage` in particular is the one field GUARANTEED to differ
// between a run's cycles, so it made a second card for the same answer with 5%
// where the first had 100%. `final_result` goes too: it is the entry's Result,
// arriving under the engine's name for it rather than the type's.
func (n *Want) answerFields(resultField string) map[string]any {
	if n.WantTypeDefinition == nil {
		return nil
	}
	engineOwned := map[string]bool{"final_result": true}
	for _, k := range SystemReservedStateFields() {
		engineOwned[k] = true
	}
	for _, k := range mrsScaffoldingFields {
		engineOwned[k] = true
	}
	// The result's own field is skipped: it is the entry's Result, and printed
	// twice it reads as two findings.
	resultRoot := resultField
	if i := strings.Index(resultRoot, "."); i >= 0 {
		resultRoot = resultRoot[:i]
	}
	out := map[string]any{}
	for _, sd := range n.WantTypeDefinition.State {
		if sd.Name == resultRoot || sd.Label == "goal" || engineOwned[sd.Name] {
			continue
		}
		if !sd.Exposable && !sd.Persistent {
			continue
		}
		val, ok := n.getState(sd.Name)
		if !ok || isEmptyAnswer(val) {
			continue
		}
		out[sd.Name] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// The MRS agent's plumbing, by the names its contract fixes.
//
// These are not the want type author's fields even though they appear in the
// type's YAML — every MRS type declares the same six because the agent reads
// them, and a plugin is free to mark them persistent (mywant-claude-info does,
// which put `skill_path` on its answer cards). Where the script lives, how long
// it may take and what it printed are how the answer was fetched, not what the
// answer is. `mrs_raw_output` in particular is the same answer again, unparsed:
// this list only applies to a want that declares a finalResultField, so there is
// always a shaped form of it already.
var mrsScaffoldingFields = []string{
	"skill_path", "skill_timeout_seconds", "skill_json_arg",
	"mrs_raw_output", "summary", "error",
}

// answerAbout picks the answer's own timestamp out of its fields.
//
// `checked_at` by name first, since that is what the MRS skills write, then any
// other field whose name ends in `_at` and whose value is a string — so a skill
// that calls it something else is not silently ignored. Keys are sorted so the
// choice does not depend on map order.
func answerAbout(fields map[string]any) string {
	if s, ok := fields["checked_at"].(string); ok && s != "" {
		return s
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !strings.HasSuffix(k, "_at") {
			continue
		}
		if s, ok := fields[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// isEmptyAnswer reports whether a value says nothing — the same emptiness the
// final_result override already skips, extended to the containers a skill's
// answer actually arrives in.
func isEmptyAnswer(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case int:
		return t == 0
	case float64:
		return t == 0
	case bool:
		return false
	case map[string]any:
		return len(t) == 0
	case []any:
		return len(t) == 0
	}
	return false
}

// resultFingerprint is what makes two answers the same answer.
//
// JSON rather than reflect.DeepEqual: the values come back from a YAML restore
// as different Go types than the ones that went in (a float64 becomes an int,
// a map[string]any a map[any]any), and a history that duplicates every entry
// on the first cycle after a restart would be worse than none.
//
// WHEN the answer was found is excluded — both About itself and the field it
// was read out of. A monitor re-checks on its own schedule, and a check that
// finds the same reservation ten minutes later writes a new `checked_at` and
// nothing else; fingerprinting that as a new answer filled the ring with the
// one reservation over and over, which is the shape this history exists to
// avoid. Same finding twice is one answer seen twice: Timestamp and About stay
// at the first sighting and LastSeen moves, so the entry reads "found this,
// and it was still true as of then".
//
// Only the key that actually produced About is dropped, not every field that
// looks like a time — a want whose answer IS a moment (a reminder's event
// time, a departure) must still be able to answer differently tomorrow.
func resultFingerprint(e ResultHistoryEntry) string {
	fields := e.Fields
	if e.About != "" {
		for k, v := range e.Fields {
			if s, ok := v.(string); ok && s == e.About {
				if fields == nil {
					break
				}
				trimmed := make(map[string]any, len(e.Fields))
				for k2, v2 := range e.Fields {
					if k2 != k {
						trimmed[k2] = v2
					}
				}
				fields = trimmed
				break
			}
		}
	}
	b, err := json.Marshal(struct {
		Result any            `json:"r"`
		Fields map[string]any `json:"f"`
	}{e.Result, fields})
	if err != nil {
		return ""
	}
	return string(b)
}

// RestoreResultHistory seeds the ring from what state.yaml kept, on the way in.
//
// Only when the ring is empty: this runs on every reconcile of a want that came
// from the file, and a want that has answered since startup owns its own
// history — re-seeding it would put the restored copies back in front of the
// answers that came after them.
func (n *Want) RestoreResultHistory(entries []ResultHistoryEntry) {
	if len(entries) == 0 {
		return
	}
	h := n.getHistoryManager()
	if len(h.ResultHistoryRing.Snapshot(0)) > 0 {
		return
	}
	for _, e := range entries {
		h.ResultHistoryRing.Append(e)
	}
}
