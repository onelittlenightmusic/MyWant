package mywant

// Type conversion helper functions to reduce code duplication
// These functions provide safe type assertions with fallback logic

// AsInt safely converts interface{} to int
// Handles int and float64 types, returns (value, ok)
func AsInt(value interface{}) (int, bool) {
	if intVal, ok := value.(int); ok {
		return intVal, true
	}
	if floatVal, ok := value.(float64); ok {
		return int(floatVal), true
	}
	return 0, false
}

// AsFloat safely converts interface{} to float64
// Handles float64 and int types, returns (value, ok)
func AsFloat(value interface{}) (float64, bool) {
	if floatVal, ok := value.(float64); ok {
		return floatVal, true
	}
	if intVal, ok := value.(int); ok {
		return float64(intVal), true
	}
	return 0, false
}

// AsString safely converts interface{} to string
// Handles string type only, returns (value, ok)
func AsString(value interface{}) (string, bool) {
	if strVal, ok := value.(string); ok {
		return strVal, true
	}
	return "", false
}

// AsBool safely converts interface{} to bool
// Handles bool type only, returns (value, ok)
func AsBool(value interface{}) (bool, bool) {
	if boolVal, ok := value.(bool); ok {
		return boolVal, true
	}
	return false, false
}

// AsMap safely converts interface{} to map[string]interface{}
// Returns (value, ok)
func AsMap(value interface{}) (map[string]interface{}, bool) {
	if mapVal, ok := value.(map[string]interface{}); ok {
		return mapVal, true
	}
	return nil, false
}

// AsArray safely converts interface{} to []interface{}
// Returns (value, ok)
func AsArray(value interface{}) ([]interface{}, bool) {
	if arrayVal, ok := value.([]interface{}); ok {
		return arrayVal, true
	}
	return nil, false
}

// AsIntWithDefault safely converts interface{} to int with a default value
// Returns the converted value or the default if conversion fails
func AsIntWithDefault(value interface{}, defaultValue int) int {
	if intVal, ok := AsInt(value); ok {
		return intVal
	}
	return defaultValue
}

// AsFloatWithDefault safely converts interface{} to float64 with a default value
// Returns the converted value or the default if conversion fails
func AsFloatWithDefault(value interface{}, defaultValue float64) float64 {
	if floatVal, ok := AsFloat(value); ok {
		return floatVal
	}
	return defaultValue
}

// AsStringWithDefault safely converts interface{} to string with a default value
// Returns the converted value or the default if conversion fails
func AsStringWithDefault(value interface{}, defaultValue string) string {
	if strVal, ok := AsString(value); ok {
		return strVal
	}
	return defaultValue
}

// AsBoolWithDefault safely converts interface{} to bool with a default value
// Returns the converted value or the default if conversion fails
func AsBoolWithDefault(value interface{}, defaultValue bool) bool {
	if boolVal, ok := AsBool(value); ok {
		return boolVal
	}
	return defaultValue
}
