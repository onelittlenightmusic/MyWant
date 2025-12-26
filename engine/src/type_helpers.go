package mywant

// Type conversion helper functions to reduce code duplication
// These functions provide safe type assertions with fallback logic

// AsInt safely converts any to int
// Handles int and float64 types, returns (value, ok)
func AsInt(value any) (int, bool) {
	if intVal, ok := value.(int); ok {
		return intVal, true
	}
	if floatVal, ok := value.(float64); ok {
		return int(floatVal), true
	}
	return 0, false
}

// AsFloat safely converts any to float64
// Handles float64 and int types, returns (value, ok)
func AsFloat(value any) (float64, bool) {
	if floatVal, ok := value.(float64); ok {
		return floatVal, true
	}
	if intVal, ok := value.(int); ok {
		return float64(intVal), true
	}
	return 0, false
}

// AsString safely converts any to string
// Handles string type only, returns (value, ok)
func AsString(value any) (string, bool) {
	if strVal, ok := value.(string); ok {
		return strVal, true
	}
	return "", false
}

// AsBool safely converts any to bool
// Handles bool type only, returns (value, ok)
func AsBool(value any) (bool, bool) {
	if boolVal, ok := value.(bool); ok {
		return boolVal, true
	}
	return false, false
}

// AsMap safely converts any to map[string]any
// Returns (value, ok)
func AsMap(value any) (map[string]any, bool) {
	if mapVal, ok := value.(map[string]any); ok {
		return mapVal, true
	}
	return nil, false
}

// AsArray safely converts any to []any
// Returns (value, ok)
func AsArray(value any) ([]any, bool) {
	if arrayVal, ok := value.([]any); ok {
		return arrayVal, true
	}
	return nil, false
}

// AsIntWithDefault safely converts any to int with a default value
// Returns the converted value or the default if conversion fails
func AsIntWithDefault(value any, defaultValue int) int {
	if intVal, ok := AsInt(value); ok {
		return intVal
	}
	return defaultValue
}

// AsFloatWithDefault safely converts any to float64 with a default value
// Returns the converted value or the default if conversion fails
func AsFloatWithDefault(value any, defaultValue float64) float64 {
	if floatVal, ok := AsFloat(value); ok {
		return floatVal
	}
	return defaultValue
}

// AsStringWithDefault safely converts any to string with a default value
// Returns the converted value or the default if conversion fails
func AsStringWithDefault(value any, defaultValue string) string {
	if strVal, ok := AsString(value); ok {
		return strVal
	}
	return defaultValue
}

// AsBoolWithDefault safely converts any to bool with a default value
// Returns the converted value or the default if conversion fails
func AsBoolWithDefault(value any, defaultValue bool) bool {
	if boolVal, ok := AsBool(value); ok {
		return boolVal
	}
	return defaultValue
}
