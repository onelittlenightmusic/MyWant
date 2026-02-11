package mywant

import (
	"fmt"
	"strconv"
)

// ToInt converts an interface{} value to int with a default fallback
// Handles: int, float64, string (numeric), and other types
func ToInt(value interface{}, defaultVal int) int {
	if value == nil {
		return defaultVal
	}

	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
		return defaultVal
	default:
		return defaultVal
	}
}

// ToString converts an interface{} value to string with a default fallback
func ToString(value interface{}, defaultVal string) string {
	if value == nil {
		return defaultVal
	}

	switch v := value.(type) {
	case string:
		return v
	case int, int64, float64, float32, bool:
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ToBool converts an interface{} value to bool with a default fallback
func ToBool(value interface{}, defaultVal bool) bool {
	if value == nil {
		return defaultVal
	}

	switch v := value.(type) {
	case bool:
		return v
	case int, int64:
		return v != 0
	case float64, float32:
		return v != 0.0
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		return defaultVal
	default:
		return defaultVal
	}
}

// ToFloat64 converts an interface{} value to float64 with a default fallback
func ToFloat64(value interface{}, defaultVal float64) float64 {
	if value == nil {
		return defaultVal
	}

	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		return defaultVal
	default:
		return defaultVal
	}
}

// ToStringSlice converts an interface{} value to []string with a default fallback
func ToStringSlice(value interface{}, defaultVal []string) []string {
	if value == nil {
		return defaultVal
	}

	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			result = append(result, ToString(item, ""))
		}
		return result
	default:
		return defaultVal
	}
}

// ExtractParamsWithDefaults extracts multiple parameters from a map with type conversion
type ParamExtractor struct {
	params map[string]interface{}
}

// NewParamExtractor creates a new parameter extractor
func NewParamExtractor(params map[string]interface{}) *ParamExtractor {
	if params == nil {
		params = make(map[string]interface{})
	}
	return &ParamExtractor{params: params}
}

// String extracts a string parameter
func (pe *ParamExtractor) String(key string, defaultVal string) string {
	return ToString(pe.params[key], defaultVal)
}

// Int extracts an int parameter
func (pe *ParamExtractor) Int(key string, defaultVal int) int {
	return ToInt(pe.params[key], defaultVal)
}

// Bool extracts a bool parameter
func (pe *ParamExtractor) Bool(key string, defaultVal bool) bool {
	return ToBool(pe.params[key], defaultVal)
}

// Float64 extracts a float64 parameter
func (pe *ParamExtractor) Float64(key string, defaultVal float64) float64 {
	return ToFloat64(pe.params[key], defaultVal)
}

// StringSlice extracts a []string parameter
func (pe *ParamExtractor) StringSlice(key string, defaultVal []string) []string {
	return ToStringSlice(pe.params[key], defaultVal)
}
