package mywant

// want_param.go — typed parameter accessors (GetIntParam, GetStringParam, etc.)

// GetParameter returns the raw parameter value and an existence flag.
func (n *Want) GetParameter(paramName string) (any, bool) {
	n.metadataMutex.RLock()
	defer n.metadataMutex.RUnlock()
	if n.Spec.Params == nil {
		return nil, false
	}
	value, exists := n.Spec.Params[paramName]
	return value, exists
}

func (n *Want) GetIntParam(key string, defaultValue int) int {
	n.metadataMutex.RLock()
	value, ok := n.Spec.Params[key]
	n.metadataMutex.RUnlock()
	if ok {
		if intVal, ok := value.(int); ok {
			return intVal
		} else if floatVal, ok := value.(float64); ok {
			return int(floatVal)
		}
	}
	return defaultValue
}

func (n *Want) GetFloatParam(key string, defaultValue float64) float64 {
	n.metadataMutex.RLock()
	value, ok := n.Spec.Params[key]
	n.metadataMutex.RUnlock()
	if ok {
		if floatVal, ok := value.(float64); ok {
			return floatVal
		} else if intVal, ok := value.(int); ok {
			return float64(intVal)
		}
	}
	return defaultValue
}

func (n *Want) GetStringParam(key string, defaultValue string) string {
	n.metadataMutex.RLock()
	value, ok := n.Spec.Params[key]
	n.metadataMutex.RUnlock()
	if ok {
		if strVal, ok := value.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

// GetStringSliceParam returns a []string parameter, accepting both a native
// []string (set programmatically) and the []any shape produced by YAML/JSON
// unmarshaling. Returns nil if the key is absent or not a recognizable slice.
func (n *Want) GetStringSliceParam(key string) []string {
	n.metadataMutex.RLock()
	value, ok := n.Spec.Params[key]
	n.metadataMutex.RUnlock()
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func (n *Want) GetBoolParam(key string, defaultValue bool) bool {
	n.metadataMutex.RLock()
	value, ok := n.Spec.Params[key]
	n.metadataMutex.RUnlock()
	if ok {
		if boolVal, ok := value.(bool); ok {
			return boolVal
		} else if strVal, ok := value.(string); ok {
			return strVal == "true" || strVal == "True" || strVal == "TRUE" || strVal == "1"
		}
	}
	return defaultValue
}

// GetGlobalParameter returns the value from parameters.yaml for the given key,
// or defaultValue if the key is absent.
func (n *Want) GetGlobalParameter(key string, defaultValue any) any {
	if v, ok := GetGlobalParameter(key); ok {
		return v
	}
	return defaultValue
}
