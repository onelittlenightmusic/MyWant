package mywant

import (
	"context"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	globalParamsMu   sync.RWMutex
	globalParameters map[string]any
	globalParamsPath string
)

// LoadGlobalParameters reads <configDir>/parameters.yaml into memory.
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
	globalParamsMu.Unlock()
	return nil
}

// SetAllGlobalParameters replaces all parameters in memory and persists to disk.
func SetAllGlobalParameters(params map[string]any) error {
	globalParamsMu.Lock()
	globalParameters = params
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

// SetGlobalParameter sets a single global parameter, persists it, and emits a ParameterChangeEvent
// so that any registered wants with matching exposeAs subscriptions receive the update.
func SetGlobalParameter(key string, value any) error {
	globalParamsMu.Lock()
	if globalParameters == nil {
		globalParameters = make(map[string]any)
	}
	globalParameters[key] = value
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
