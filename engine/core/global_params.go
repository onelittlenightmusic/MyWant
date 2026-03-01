package mywant

import (
	"os"
	"sync"

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
