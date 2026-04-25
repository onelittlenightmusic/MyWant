package mywant

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	globalParamsMu   sync.RWMutex
	globalParameters map[string]any
	globalParamTypes map[string][]string // cached classification, updated on every write
	globalParamsPath string
)

// recomputeTypes classifies the given parameter snapshot by type schema.
// Must be called with globalParamsMu held for writing, or on an isolated snapshot.
// Currently recognised types:
//   - "timer": value has a valid WhenSpec structure (requires "every" field)
func recomputeTypes(params map[string]any) map[string][]string {
	var timerKeys []string
	for key, raw := range params {
		data, err := yaml.Marshal(raw)
		if err != nil {
			continue
		}
		var spec WhenSpec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			continue
		}
		if spec.Every != "" {
			timerKeys = append(timerKeys, key)
		}
	}
	result := map[string][]string{}
	if len(timerKeys) > 0 {
		result["timer"] = timerKeys
	}
	return result
}

// LoadGlobalParameters reads <configDir>/parameters.yaml into memory and
// recomputes the type classification cache.
// Absent file is silently ignored (not an error).
func LoadGlobalParameters(path string) error {
	globalParamsPath = path
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var params map[string]any
	if err := yaml.Unmarshal(data, &params); err != nil {
		return err
	}
	globalParamsMu.Lock()
	globalParameters = params
	globalParamTypes = recomputeTypes(params)
	globalParamsMu.Unlock()
	return nil
}

// SetAllGlobalParameters replaces all parameters in memory, recomputes the type
// classification cache, and persists to disk.
func SetAllGlobalParameters(params map[string]any) error {
	globalParamsMu.Lock()
	globalParameters = params
	globalParamTypes = recomputeTypes(params)
	path := globalParamsPath
	globalParamsMu.Unlock()
	if path == "" {
		return nil
	}
	data, err := yaml.Marshal(params)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetGlobalParameter returns (value, true) if key exists, or (nil, false).
func GetGlobalParameter(key string) (any, bool) {
	globalParamsMu.RLock()
	defer globalParamsMu.RUnlock()
	if globalParameters == nil {
		return nil, false
	}
	v, ok := globalParameters[key]
	return v, ok
}

// SetGlobalParameter sets a single global parameter, recomputes the type
// classification cache, persists to disk, and emits a ParameterChangeEvent.
func SetGlobalParameter(key string, value any) error {
	globalParamsMu.Lock()
	if globalParameters == nil {
		globalParameters = make(map[string]any)
	}
	globalParameters[key] = value
	globalParamTypes = recomputeTypes(globalParameters)
	path := globalParamsPath
	globalParamsMu.Unlock()

	if path != "" {
		all := GetAllGlobalParameters()
		data, err := yaml.Marshal(all)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}

	// Emit ParameterChangeEvent so subscribed wants receive the update asynchronously
	event := &ParameterChangeEvent{
		BaseEvent: BaseEvent{
			EventType:  EventTypeParameterChange,
			SourceName: "__global__",
			Timestamp:  time.Now(),
		},
		ParamName:  key,
		ParamValue: value,
	}
	GetGlobalSubscriptionSystem().Emit(context.Background(), event)
	return nil
}

// GetGlobalParamTypes returns a snapshot copy of the cached type classification.
// Keys are type names (e.g. "timer") and values are lists of parameter keys
// whose values conform to that type's schema.
func GetGlobalParamTypes() map[string][]string {
	globalParamsMu.RLock()
	defer globalParamsMu.RUnlock()
	result := make(map[string][]string, len(globalParamTypes))
	for typeName, keys := range globalParamTypes {
		cp := make([]string, len(keys))
		copy(cp, keys)
		result[typeName] = cp
	}
	return result
}

// ResolveFromGlobalParamSpec looks up a named entry from global parameters and converts it to a WhenSpec.
// The value in parameters.yaml must be a map with at least an "every" field (and optionally "at").
// The returned WhenSpec retains FromGlobalParam so callers can see where the values originated.
func ResolveFromGlobalParamSpec(name string) (WhenSpec, error) {
	raw, ok := GetGlobalParameter(name)
	if !ok {
		return WhenSpec{}, fmt.Errorf("fromGlobalParam %q not found in parameters", name)
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return WhenSpec{}, fmt.Errorf("fromGlobalParam %q: marshal error: %w", name, err)
	}
	var spec WhenSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return WhenSpec{}, fmt.Errorf("fromGlobalParam %q: not a valid WhenSpec: %w", name, err)
	}
	if spec.Every == "" {
		return WhenSpec{}, fmt.Errorf("fromGlobalParam %q: missing required field 'every'", name)
	}
	spec.FromGlobalParam = name // retain reference so API responses show the origin
	return spec, nil
}

// ResolveFromGlobalParams resolves fromGlobalParam references in each want's spec.when.
// The resolved at/every fields are populated from parameters.yaml, and fromGlobalParam is
// preserved so API responses can show both the expanded values and their origin.
func ResolveFromGlobalParams(wants []*Want) error {
	for _, w := range wants {
		for i, ws := range w.Spec.When {
			if ws.FromGlobalParam == "" {
				continue
			}
			resolved, err := ResolveFromGlobalParamSpec(ws.FromGlobalParam)
			if err != nil {
				return err
			}
			w.Spec.When[i] = resolved
		}
	}
	return nil
}

// GetAllGlobalParameters returns a snapshot copy of all parameters.
func GetAllGlobalParameters() map[string]any {
	globalParamsMu.RLock()
	defer globalParamsMu.RUnlock()
	result := make(map[string]any, len(globalParameters))
	for k, v := range globalParameters {
		result[k] = v
	}
	return result
}
