package types

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[WorkLogWant, WorkLogLocals]("work_log")
	})
}

// workLogAgentName is the id of the internal PollingAgent (the "work log agent")
// that watches this want's imported fields and flushes changes to work.log.
const workLogAgentName = "work_log_agent"

// workLogDetectInterval is how often the agent checks imported fields for
// changes. This is independent of flush_interval, which only throttles how
// often accumulated changes are written to work.log — detection stays fast so
// each change's own timestamp/previous-value is captured accurately.
const workLogDetectInterval = 1 * time.Second

// defaultFlushInterval is used when the flush_interval param is empty or fails
// to parse.
const defaultFlushInterval = 5 * time.Minute

// recentEntriesLimit caps how many of the most-recently-flushed changes are
// kept in state.recent_entries for the GUI want card to show — a rolling
// window, not the full history (that lives in work.log itself).
const recentEntriesLimit = 5

// workLogRecentEntry is one flushed change as surfaced in state.recent_entries
// (newest first) — a trimmed-down view of workLogChange for GUI display.
type workLogRecentEntry struct {
	Ts            string `json:"ts"`
	Field         string `json:"field"`
	Event         string `json:"event"`
	PreviousValue any    `json:"previous_value"`
	NewValue      any    `json:"new_value"`
}

// workLogChange is one detected field change, buffered until the next flush.
type workLogChange struct {
	Ts        time.Time
	Field     string // local (imported) field name
	GlobalKey string // global state key the field was imported from
	Previous  any    // nil for Initial entries
	New       any
	Initial   bool // true for a field's first observation (want just started tracking it)
}

// WorkLogLocals holds the in-memory watch state. Not persisted across restarts
// (a restart re-baselines from the current imported values, same as any other
// want type's Locals) — restarting a want because its config changed is
// expected behavior, not something work_log needs to survive.
type WorkLogLocals struct {
	mu            sync.Mutex
	lastValues    map[string]any
	pending       []workLogChange
	lastFlush     time.Time
	recentEntries []workLogRecentEntry // newest first, capped at recentEntriesLimit
}

// WorkLogWant watches state fields imported via spec.imports (e.g. a
// globally-exposed "location" field from a location want) and appends a
// work.log entry — timestamp + previous value — whenever one changes. Output
// is throttled to at most once per flush_interval so a fast-changing field
// doesn't flood work.log; the change's true detection time and previous value
// are preserved regardless of how long it waits to be flushed.
type WorkLogWant struct{ Want }

func (w *WorkLogWant) GetLocals() *WorkLogLocals {
	return CheckLocalsInitialized[WorkLogLocals](&w.Want)
}

func (w *WorkLogWant) Initialize() {
	locals := w.GetLocals()
	locals.mu.Lock()
	locals.lastValues = map[string]any{}
	locals.pending = nil
	locals.lastFlush = time.Now()
	locals.recentEntries = nil
	locals.mu.Unlock()

	flushInterval := w.GetStringParam("flush_interval", "5m")
	w.SetCurrent("flush_interval", flushInterval)
	w.SetCurrent("tracked_fields", trackedFieldsFrom(&w.Want))
	w.SetCurrent("pending_count", 0)
	w.SetCurrent("total_logged", 0)
	w.SetCurrent("last_flush_at", "")
	w.SetCurrent("recent_entries", []workLogRecentEntry{})

	// Register imported local keys as explicit state so their raw values show
	// up in the GUI's Current section instead of Hidden State (same fix as
	// GUI-authored derived fields — see derived_fields.go).
	for _, localKey := range w.Spec.Imports {
		if !Contains(w.ProvidedStateFields, localKey) {
			w.ProvidedStateFields = append(w.ProvidedStateFields, localKey)
		}
	}

	// Idempotent: AddBackgroundAgent errors if an agent with this ID already
	// exists (e.g. a prior Initialize() call), which we can safely ignore.
	_ = w.AddMonitoringAgent(workLogAgentName, workLogDetectInterval, workLogAgentPoll)
}

func (w *WorkLogWant) IsAchieved() bool { return false }

// Progress is intentionally empty — all watching/logging happens in the
// work_log_agent PollingAgent so detection keeps running on its own fast,
// fixed cadence independent of the main progression loop.
func (w *WorkLogWant) Progress() {}

// parseFlushInterval accepts Go duration syntax ("5m", "30s") as well as the
// "5min"-style shorthand from the feature request, by normalizing "min" to "m"
// before falling back to defaultFlushInterval. Zero ("0", "0s", ...) is valid
// and means "flush immediately on every detected change" — see workLogAgentPoll.
func parseFlushInterval(s string) time.Duration {
	if d, err := time.ParseDuration(s); err == nil && d >= 0 {
		return d
	}
	if d, err := time.ParseDuration(strings.ReplaceAll(s, "min", "m")); err == nil && d >= 0 {
		return d
	}
	return defaultFlushInterval
}

// workLogAgentPoll is the work log agent: on every tick it (1) detects changes
// in every imported field since the last tick, buffering each with its own
// timestamp and previous value, then (2) flushes the buffer to work.log —
// immediately if any entry is a field's first observation (so a newly added
// want/import shows up in work.log right away instead of waiting up to
// flush_interval), otherwise once flush_interval has elapsed since the last flush.
func workLogAgentPoll(_ context.Context, want *Want) (bool, error) {
	locals := CheckLocalsInitialized[WorkLogLocals](want)

	now := time.Now()
	allState := want.GetAllState() // resolves imports to live values under their local key

	locals.mu.Lock()
	hasInitial := false
	for globalKey, localKey := range want.Spec.Imports {
		newVal, present := allState[localKey]
		if !present {
			continue
		}
		oldVal, hadOld := locals.lastValues[localKey]
		locals.lastValues[localKey] = newVal
		if !hadOld {
			// First observation of this field (want just started, or the import
			// was just added) — report it immediately rather than silently
			// establishing a baseline.
			locals.pending = append(locals.pending, workLogChange{
				Ts:        now,
				Field:     localKey,
				GlobalKey: globalKey,
				Previous:  nil,
				New:       newVal,
				Initial:   true,
			})
			hasInitial = true
			continue
		}
		if valuesEqualForWorkLog(oldVal, newVal) {
			continue
		}
		locals.pending = append(locals.pending, workLogChange{
			Ts:        now,
			Field:     localKey,
			GlobalKey: globalKey,
			Previous:  oldVal,
			New:       newVal,
		})
	}

	flushInterval := parseFlushInterval(want.GetStringParam("flush_interval", "5m"))
	shouldFlush := len(locals.pending) > 0 && (hasInitial || flushInterval <= 0 || now.Sub(locals.lastFlush) >= flushInterval)
	var toFlush []workLogChange
	if shouldFlush {
		toFlush = locals.pending
		locals.pending = nil
		locals.lastFlush = now
	}
	pendingCount := len(locals.pending)
	locals.mu.Unlock()

	want.SetCurrent("pending_count", pendingCount)
	want.SetCurrent("tracked_fields", trackedFieldsFrom(want))

	if len(toFlush) == 0 {
		return false, nil
	}

	for _, change := range toFlush {
		event := "change"
		if change.Initial {
			event = "initial"
		}
		AppendWorkLog(WorkLogEntry{
			Ts:        change.Ts.UTC().Format(time.RFC3339Nano),
			Type:      "field_change",
			Important: true,
			Data: map[string]any{
				"want_id":        want.Metadata.ID,
				"want_name":      want.Metadata.Name,
				"field":          change.Field,
				"global_key":     change.GlobalKey,
				"event":          event,
				"previous_value": change.Previous,
				"new_value":      change.New,
			},
		})
	}

	total, _ := want.GetStateInt("total_logged", 0)
	want.SetCurrent("total_logged", total+len(toFlush))
	want.SetCurrent("last_flush_at", now.UTC().Format(time.RFC3339Nano))

	// Prepend this flush's changes (newest first, latest change last within a
	// single flush goes first) and trim to the rolling display window — the
	// full history stays in work.log itself, this is just for the want card.
	locals.mu.Lock()
	for i := len(toFlush) - 1; i >= 0; i-- {
		change := toFlush[i]
		event := "change"
		if change.Initial {
			event = "initial"
		}
		locals.recentEntries = append([]workLogRecentEntry{{
			Ts:            change.Ts.UTC().Format(time.RFC3339Nano),
			Field:         change.Field,
			Event:         event,
			PreviousValue: change.Previous,
			NewValue:      change.New,
		}}, locals.recentEntries...)
	}
	if len(locals.recentEntries) > recentEntriesLimit {
		locals.recentEntries = locals.recentEntries[:recentEntriesLimit]
	}
	recentEntries := append([]workLogRecentEntry(nil), locals.recentEntries...)
	locals.mu.Unlock()

	want.SetCurrent("recent_entries", recentEntries)

	return false, nil
}

func trackedFieldsFrom(want *Want) []string {
	fields := make([]string, 0, len(want.Spec.Imports))
	for _, localKey := range want.Spec.Imports {
		fields = append(fields, localKey)
	}
	sort.Strings(fields)
	return fields
}

// valuesEqualForWorkLog compares two state values for change detection.
// Uses reflect.DeepEqual so map/slice-typed imports (e.g. a JSON object field)
// compare correctly instead of panicking on == (uncomparable dynamic types).
func valuesEqualForWorkLog(a, b any) bool {
	return reflect.DeepEqual(a, b)
}
