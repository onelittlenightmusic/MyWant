package mywant

import (
	"reflect"
	"testing"
	"time"
)

// A want written to a .json memory file reads back as the same want written
// to state.yaml reads back — the same values of the same types (a whole number
// an int, as yaml.v3 gives it), so nothing reading its state after a restart
// can tell which file it came from.
func TestMemoryJSONReadsBackAsYAMLDoes(t *testing.T) {
	at := time.Date(2026, 11, 9, 22, 0, 0, 0, time.UTC)
	w := &Want{Metadata: Metadata{ID: "w1", Name: "伊東→中野坂上", Type: "transit_search",
		Labels: map[string]string{"mywant.io/canvas-x": "6"}}}
	w.Spec.Params = map[string]any{"from": "伊東", "to": "中野坂上"}
	w.Status = "achieved"
	w.State.Store("count", 20)
	w.State.Store("ratio", 2.5)
	w.State.Store("done", true)
	w.State.Store("routes", []any{map[string]any{"minutes": 95, "fare": 1980, "legs": []any{"JR", "丸ノ内線"}}})
	w.State.Store("final_result", map[string]any{"summary": "1時間35分", "transfers": 2})
	w.stateTimestamps.Store("count", at)
	w.StateLabels.Set("count", LabelCurrent)
	w.StateLabels.Set("budget", LabelGoal)
	w.ResultHistory = []ResultHistoryEntry{{ID: "r1", Timestamp: at, Result: map[string]any{"minutes": 95},
		Fields: map[string]any{"fare": 1980}, Current: true}}
	w.PhaseRetryCount = map[string]int{"execute": 1}

	read := func(asJSON bool) *Want {
		data, err := marshalMemory([]*Want{w}, asJSON)
		if err != nil {
			t.Fatal(err)
		}
		got, err := unmarshalMemory(data, asJSON)
		if err != nil || len(got) != 1 {
			t.Fatalf("json=%v: %v %d", asJSON, err, len(got))
		}
		return got[0]
	}
	y, j := read(false), read(true)

	for _, key := range []string{"count", "ratio", "done", "routes", "final_result"} {
		yv, _ := y.State.Load(key)
		jv, _ := j.State.Load(key)
		if !reflect.DeepEqual(yv, jv) {
			t.Errorf("%s: yaml %#v, json %#v", key, yv, jv)
		}
	}
	if !reflect.DeepEqual(y.Metadata, j.Metadata) || !reflect.DeepEqual(y.Spec.Params, j.Spec.Params) || y.Status != j.Status {
		t.Errorf("metadata/spec/status differ:\n%#v\n%#v", y.Metadata, j.Metadata)
	}
	if !reflect.DeepEqual(y.StateLabels.Map(), j.StateLabels.Map()) {
		t.Errorf("labels: %v / %v", y.StateLabels.Map(), j.StateLabels.Map())
	}
	yt, _ := y.stateTimestamps.Load("count")
	jt, _ := j.stateTimestamps.Load("count")
	if !yt.(time.Time).Equal(jt.(time.Time)) {
		t.Errorf("timestamps: %v / %v", yt, jt)
	}
	if !reflect.DeepEqual(y.ResultHistory[0].Result, j.ResultHistory[0].Result) ||
		!reflect.DeepEqual(y.ResultHistory[0].Fields, j.ResultHistory[0].Fields) {
		t.Errorf("history: %#v / %#v", y.ResultHistory[0], j.ResultHistory[0])
	}
	if !reflect.DeepEqual(y.PhaseRetryCount, j.PhaseRetryCount) {
		t.Errorf("retries: %v / %v", y.PhaseRetryCount, j.PhaseRetryCount)
	}
}
