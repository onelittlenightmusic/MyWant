package mywant

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The memory file (state.yaml) in YAML or, when its name ends in .json, in
// JSON — the same wants, the same fields under the same names. YAML is for a
// person to open; JSON is ~9x faster to write (encoding/json against yaml.v3,
// measured on a 3.4MB state: 5.6ms against 50ms) and 22% smaller, which is
// what a file rewritten every few seconds pays for.

// memoryIsJSON: the memory file at path is JSON.
func memoryIsJSON(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".json")
}

// persistedWant is a want as the memory file keeps it in JSON: what
// Want.MarshalYAML writes, field for field. Want's own JSON is the API's —
// it carries History and leaves out State and ResultHistory.
type persistedWant struct {
	State           map[string]any        `json:"state,omitempty"`
	StateTimestamps map[string]time.Time  `json:"state_timestamps,omitempty"`
	StateLabels     map[string]StateLabel `json:"state_labels,omitempty"`
	Metadata        Metadata              `json:"metadata"`
	Spec            WantSpec              `json:"spec"`
	Status          WantStatus            `json:"status,omitempty"`
	HiddenState     map[string]any        `json:"hidden_state,omitempty"`
	ResultHistory   []ResultHistoryEntry  `json:"result_history,omitempty"`
	Hash            string                `json:"hash,omitempty"`
	CurrentAgent    string                `json:"current_agent,omitempty"`
	RunningAgents   []string              `json:"running_agents,omitempty"`
	PhaseRetryCount map[string]int        `json:"phase_retry_count,omitempty"`
	LastPhaseError  string                `json:"last_phase_error,omitempty"`
}

func (n *Want) persisted() persistedWant {
	state := map[string]any{}
	n.State.Range(func(key, value any) bool {
		// As MarshalYAML: an imported field is global state's, not this want's.
		if _, isImported := n.importedLocalKey(key.(string)); !isImported {
			state[key.(string)] = value
		}
		return true
	})
	ts := map[string]time.Time{}
	n.stateTimestamps.Range(func(key, value any) bool {
		ts[key.(string)] = value.(time.Time)
		return true
	})
	return persistedWant{
		State: state, StateTimestamps: ts, StateLabels: n.StateLabels.Map(),
		Metadata: n.Metadata, Spec: n.Spec, Status: n.Status, HiddenState: n.HiddenState,
		ResultHistory: n.ResultHistory, Hash: n.Hash,
		CurrentAgent: n.CurrentAgent, RunningAgents: n.RunningAgents,
		PhaseRetryCount: n.PhaseRetryCount, LastPhaseError: n.LastPhaseError,
	}
}

func (p *persistedWant) want() *Want {
	n := &Want{
		Metadata: p.Metadata, Spec: p.Spec, Status: p.Status,
		HiddenState: asYAMLNumbers(p.HiddenState).(map[string]any),
		Hash:        p.Hash, CurrentAgent: p.CurrentAgent, RunningAgents: p.RunningAgents,
		PhaseRetryCount: p.PhaseRetryCount, LastPhaseError: p.LastPhaseError,
	}
	for key, val := range p.State {
		n.State.Store(key, asYAMLNumbers(val))
	}
	for key, at := range p.StateTimestamps {
		n.stateTimestamps.Store(key, at)
	}
	if p.StateLabels != nil {
		n.StateLabels.Replace(p.StateLabels)
	}
	for i := range p.ResultHistory {
		e := &p.ResultHistory[i]
		e.Result = asYAMLNumbers(e.Result)
		if e.Fields != nil {
			e.Fields = asYAMLNumbers(e.Fields).(map[string]any)
		}
	}
	n.ResultHistory = p.ResultHistory
	return n
}

// asYAMLNumbers gives a value read from JSON the numbers it would have had
// read from YAML: a whole number an int, as yaml.v3 reads "60", not the
// float64 encoding/json reads everything as — a want asserting .(int) on its
// state must find after a restart what it found before.
func asYAMLNumbers(v any) any {
	switch t := v.(type) {
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1<<53 {
			return int(t)
		}
		return t
	case map[string]any:
		if t == nil {
			return t
		}
		for k, x := range t {
			t[k] = asYAMLNumbers(x)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = asYAMLNumbers(x)
		}
		return t
	}
	return v
}

// encodeMemoryWant is one want as written into the memory file: a one-item
// YAML sequence (pieces end to end make the list), or the want's JSON object.
func encodeMemoryWant(w *Want, asJSON bool) ([]byte, error) {
	if asJSON {
		return json.Marshal(w.persisted())
	}
	return yaml.Marshal([]*Want{w})
}

// marshalMemory is the whole memory file for these wants.
func marshalMemory(wants []*Want, asJSON bool) ([]byte, error) {
	if !asJSON {
		return yaml.Marshal(wants)
	}
	list := make([]persistedWant, len(wants))
	for i, w := range wants {
		list[i] = w.persisted()
	}
	return json.Marshal(list)
}

// unmarshalMemory reads the memory file's wants.
func unmarshalMemory(data []byte, asJSON bool) ([]*Want, error) {
	if !asJSON {
		var wants []*Want
		err := yaml.Unmarshal(data, &wants)
		return wants, err
	}
	var list []persistedWant
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	wants := make([]*Want, len(list))
	for i := range list {
		wants[i] = list[i].want()
	}
	return wants, nil
}
