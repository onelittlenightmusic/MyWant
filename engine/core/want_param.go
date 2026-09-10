package mywant

// want_param.go — typed parameter accessors (GetIntParam, GetStringParam, etc.)

// setResolvedParam records the runtime-resolved value of a spec.params entry
// that was declared as a {fromGlobalParam: key} reference — see the
// resolvedParams field doc comment on Want for why this is kept separate from
// Spec.Params.
func (n *Want) setResolvedParam(key string, value any) {
	n.metadataMutex.Lock()
	defer n.metadataMutex.Unlock()
	if n.resolvedParams == nil {
		n.resolvedParams = make(map[string]any)
	}
	n.resolvedParams[key] = value
}

// getRawParamLocked returns the effective raw value for key: the resolved
// value if this param was a {fromGlobalParam: key} reference, otherwise
// whatever is in Spec.Params. Caller must hold metadataMutex (read or write).
func (n *Want) getRawParamLocked(key string) (any, bool) {
	if n.resolvedParams != nil {
		if v, ok := n.resolvedParams[key]; ok {
			return n.convertParamLocked(key, v), true
		}
	}
	if n.Spec.Params == nil {
		return nil, false
	}
	value, exists := n.Spec.Params[key]
	if !exists {
		return nil, false
	}
	return n.convertParamLocked(key, value), true
}

// convertParamLocked rewrites a value into the kind this parameter is written
// in, when the wire feeding it carries a compatible but differently-written
// one. Applied on the way OUT rather than when the wire is made: the value
// behind a live reference changes, and a conversion done once would be a copy
// of the first answer.
func (n *Want) convertParamLocked(key string, v any) any {
	if len(n.paramConversions) == 0 {
		return v
	}
	c, ok := n.paramConversions[key]
	if !ok {
		return v
	}
	if converted, ok := ConvertSubTypeValue(v, c.From, c.To); ok {
		return converted
	}
	return v
}

// ResolveGlobalParamRef re-reads one {fromGlobalParam: key} parameter from the
// global store.
//
// Params are resolved once, when a want's type definition is set — which is
// before anything has had a chance to publish. A want that declares where its
// value comes from is exactly the case where the publisher may not have existed
// yet, so the wire that arranges it re-asks afterwards rather than leaving the
// reference to resolve on some later restart.
//
// Returns whether a value was found.
func (n *Want) ResolveGlobalParamRef(param string) bool {
	n.metadataMutex.RLock()
	raw, ok := n.Spec.Params[param]
	n.metadataMutex.RUnlock()
	if !ok {
		return false
	}
	ref, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	key, ok := ref["fromGlobalParam"].(string)
	if !ok || key == "" {
		return false
	}
	v, ok := GetGlobalParameter(key)
	if !ok {
		return false
	}
	n.setResolvedParam(param, v)
	return true
}

// SetParamConversion records that this parameter is fed a `from`-shaped value
// and reads a `to`-shaped one. Idempotent; an empty pair clears it.
func (n *Want) SetParamConversion(param, from, to string) {
	n.metadataMutex.Lock()
	defer n.metadataMutex.Unlock()
	if from == "" || to == "" || from == to {
		delete(n.paramConversions, param)
		return
	}
	if n.paramConversions == nil {
		n.paramConversions = make(map[string]paramConversion)
	}
	n.paramConversions[param] = paramConversion{From: from, To: to}
}

// GetParameter returns the effective parameter value (resolved, if the param
// is a {fromGlobalParam: key} reference) and an existence flag.
func (n *Want) GetParameter(paramName string) (any, bool) {
	n.metadataMutex.RLock()
	defer n.metadataMutex.RUnlock()
	return n.getRawParamLocked(paramName)
}

func (n *Want) GetIntParam(key string, defaultValue int) int {
	n.metadataMutex.RLock()
	value, ok := n.getRawParamLocked(key)
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
	value, ok := n.getRawParamLocked(key)
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
	value, ok := n.getRawParamLocked(key)
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
	value, ok := n.getRawParamLocked(key)
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
	value, ok := n.getRawParamLocked(key)
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
