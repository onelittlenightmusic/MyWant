package want_spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

func (s WantSpec) GetParam(key string) (any, bool) {
	if s.Params == nil {
		return nil, false
	}
	v, ok := s.Params[key]
	return v, ok
}

func (s *WantSpec) SetParam(key string, val any) {
	if s.Params == nil {
		s.Params = make(map[string]any)
	}
	s.Params[key] = val
}

func (s WantSpec) HasParam(key string) bool {
	if s.Params == nil {
		return false
	}
	_, ok := s.Params[key]
	return ok
}

func (s WantSpec) ParamsAsMap() map[string]any {
	return s.Params
}

func (s *WantSpec) SetParamsFromMap(m map[string]any) {
	s.Params = m
}

// knownWantSpecJSONFields lists all JSON field names declared in WantSpec.
// Used for unknown-field detection — callers can log or error on any key not in this set.
var knownWantSpecJSONFields = func() map[string]struct{} {
	fields := []string{
		"params",
		"exposes",
		"imports",
		"using",
		"recipe",
		"stateSubscriptions",
		"notificationFilters",
		"requires",
		"when",
		"finalResultField",
		"resetOnRestart",
		"autoExpose",
		"alerts",
	}
	m := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		m[f] = struct{}{}
	}
	return m
}()

// KnownWantSpecJSONFields returns a snapshot of the JSON field names that WantSpec
// currently recognises. mywant (or any other consumer) can call this to detect
// unknown fields arriving from an API client that was built against a newer spec.
func KnownWantSpecJSONFields() []string {
	out := make([]string, 0, len(knownWantSpecJSONFields))
	for k := range knownWantSpecJSONFields {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// UnmarshalJSON decodes a WantSpec from JSON.
//
// params is handled specially: it may be either a JSON object (map[string]any)
// or a JSON array of {key, value} entries ([]ParamEntry).  All other fields are
// decoded normally.
//
// If the JSON contains a key that is not listed in knownWantSpecJSONFields the
// field is silently ignored here; callers that want to detect schema drift should
// call KnownWantSpecJSONFields() and compare against the raw JSON themselves.
func (s *WantSpec) UnmarshalJSON(data []byte) error {
	// ── Step 1: grab every raw key so we can pass unknown fields back ──────
	var allRaw map[string]json.RawMessage
	if err := json.Unmarshal(data, &allRaw); err != nil {
		return err
	}

	// ── Step 2: decode all known fields except params ───────────────────────
	// Use a shadow type (no Params) to avoid infinite recursion and to keep
	// the params special-casing below.
	type wantSpecKnown struct {
		Exposes             []ExposeEntry        `json:"exposes,omitempty"`
		Imports             map[string]string    `json:"imports,omitempty"`
		Using               []UsingEntry         `json:"using,omitempty"`
		Recipe              string               `json:"recipe,omitempty"`
		StateSubscriptions  []StateSubscription  `json:"stateSubscriptions,omitempty"`
		NotificationFilters []NotificationFilter `json:"notificationFilters,omitempty"`
		Requires            []string             `json:"requires,omitempty"`
		When                []WhenSpec           `json:"when,omitempty"`
		FinalResultField    string               `json:"finalResultField,omitempty"`
		ResetOnRestart      *bool                `json:"resetOnRestart,omitempty"`
		Alerts              []AlertRule          `json:"alerts,omitempty"`
	}
	var raw struct {
		wantSpecKnown
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	s.Exposes = raw.Exposes
	s.Imports = raw.Imports
	s.Using = raw.Using
	s.Recipe = raw.Recipe
	s.StateSubscriptions = raw.StateSubscriptions
	s.NotificationFilters = raw.NotificationFilters
	s.Requires = raw.Requires
	s.When = raw.When
	s.FinalResultField = raw.FinalResultField
	s.ResetOnRestart = raw.ResetOnRestart
	s.Alerts = raw.Alerts

	// ── Step 3: detect unknown fields ───────────────────────────────────────
	// Populate UnknownFields so callers can log schema-drift warnings.
	s.UnknownFields = nil
	for k := range allRaw {
		if _, known := knownWantSpecJSONFields[k]; !known {
			s.UnknownFields = append(s.UnknownFields, k)
		}
	}
	sort.Strings(s.UnknownFields)

	// ── Step 4: decode params (object or array format) ──────────────────────
	if raw.Params == nil {
		return nil
	}
	trimmed := bytes.TrimSpace(raw.Params)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var entries []ParamEntry
		if err := json.Unmarshal(raw.Params, &entries); err != nil {
			return err
		}
		s.Params = make(map[string]any, len(entries))
		for _, e := range entries {
			s.Params[e.Key] = e.Value
		}
	} else {
		var m map[string]any
		if err := json.Unmarshal(raw.Params, &m); err != nil {
			return err
		}
		s.Params = m
	}
	return nil
}

// knownWantSpecYAMLFields lists all YAML field names declared in WantSpec.
var knownWantSpecYAMLFields = func() map[string]struct{} {
	fields := []string{
		"params",
		"exposes",
		"imports",
		"using",
		"recipe",
		"stateSubscriptions",
		"notificationFilters",
		"requires",
		"when",
		"finalResultField",
		"resetOnRestart",
		"autoExpose",
		"alerts",
	}
	m := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		m[f] = struct{}{}
	}
	return m
}()

// UnmarshalYAML decodes a WantSpec from a YAML mapping node.
//
// params is handled specially (object or sequence format).  Unknown keys are
// collected in UnknownFields.
func (s *WantSpec) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("WantSpec must be a YAML mapping")
	}

	// Shadow struct for all non-params fields.
	type restSpec struct {
		Exposes             []ExposeEntry        `yaml:"exposes,omitempty"`
		Imports             map[string]string    `yaml:"imports,omitempty"`
		Using               []UsingEntry         `yaml:"using,omitempty"`
		Recipe              string               `yaml:"recipe,omitempty"`
		StateSubscriptions  []StateSubscription  `yaml:"stateSubscriptions,omitempty"`
		NotificationFilters []NotificationFilter `yaml:"notificationFilters,omitempty"`
		Requires            []string             `yaml:"requires,omitempty"`
		When                []WhenSpec           `yaml:"when,omitempty"`
		FinalResultField    string               `yaml:"finalResultField,omitempty"`
		ResetOnRestart      *bool                `yaml:"resetOnRestart,omitempty"`
		AutoExpose          bool                 `yaml:"autoExpose,omitempty"`
		Alerts              []AlertRule          `yaml:"alerts,omitempty"`
	}

	var paramsNode *yaml.Node
	var rest restSpec
	s.UnknownFields = nil

	for i := 0; i+1 < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]
		key := keyNode.Value

		if key == "params" {
			paramsNode = valNode
			continue
		}

		// Detect unknown fields.
		if _, known := knownWantSpecYAMLFields[key]; !known {
			s.UnknownFields = append(s.UnknownFields, key)
			continue
		}

		// Decode into restSpec by building a temporary single-key mapping.
		tempNode := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: []*yaml.Node{keyNode, valNode},
		}
		if err := tempNode.Decode(&rest); err != nil {
			return fmt.Errorf("WantSpec field %q: %w", key, err)
		}
	}
	sort.Strings(s.UnknownFields)

	s.Exposes = rest.Exposes
	s.Imports = rest.Imports
	s.Using = rest.Using
	s.Recipe = rest.Recipe
	s.StateSubscriptions = rest.StateSubscriptions
	s.NotificationFilters = rest.NotificationFilters
	s.Requires = rest.Requires
	s.When = rest.When
	s.FinalResultField = rest.FinalResultField
	s.ResetOnRestart = rest.ResetOnRestart
	s.AutoExpose = rest.AutoExpose
	s.Alerts = rest.Alerts

	// ── params: object or sequence ───────────────────────────────────────────
	if paramsNode == nil {
		return nil
	}
	if paramsNode.Kind == yaml.SequenceNode {
		var entries []ParamEntry
		if err := paramsNode.Decode(&entries); err != nil {
			return err
		}
		s.Params = make(map[string]any, len(entries))
		for _, e := range entries {
			s.Params[e.Key] = e.Value
		}
	} else {
		var m map[string]any
		if err := paramsNode.Decode(&m); err != nil {
			return err
		}
		s.Params = m
	}
	return nil
}
