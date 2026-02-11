package mywant

import "fmt"

// YAML/JSON validation helper functions to reduce code duplication
// These functions provide safe field extraction and validation

// GetMapField safely extracts and validates a map field from a parent map
// Returns error if field is missing or not a map
func GetMapField(obj map[string]any, key, fieldName string) (map[string]any, error) {
	value, ok := obj[key]
	if !ok {
		return nil, fmt.Errorf("missing required '%s' field", fieldName)
	}

	mapVal, ok := AsMap(value)
	if !ok {
		return nil, fmt.Errorf("'%s' must be an object, got %T", fieldName, value)
	}

	return mapVal, nil
}

// GetArrayField safely extracts and validates an array field from a parent map
// Returns error if field is missing or not an array
func GetArrayField(obj map[string]any, key, fieldName string) ([]any, error) {
	value, ok := obj[key]
	if !ok {
		return nil, fmt.Errorf("missing required '%s' field", fieldName)
	}

	arrayVal, ok := AsArray(value)
	if !ok {
		return nil, fmt.Errorf("'%s' must be an array, got %T", fieldName, value)
	}

	return arrayVal, nil
}

// GetStringField safely extracts and validates a string field from a parent map
// Returns error if field is missing or not a string
func GetStringField(obj map[string]any, key, fieldName string) (string, error) {
	value, ok := obj[key]
	if !ok {
		return "", fmt.Errorf("missing required '%s' field", fieldName)
	}

	strVal, ok := AsString(value)
	if !ok {
		return "", fmt.Errorf("'%s' must be a string, got %T", fieldName, value)
	}

	if strVal == "" {
		return "", fmt.Errorf("'%s' cannot be empty", fieldName)
	}

	return strVal, nil
}

// GetStringFieldOptional safely extracts an optional string field from a parent map
// Returns the string value or empty string if field is missing
func GetStringFieldOptional(obj map[string]any, key string) string {
	value, ok := obj[key]
	if !ok {
		return ""
	}

	strVal, ok := AsString(value)
	if !ok {
		return ""
	}

	return strVal
}

// GetIntField safely extracts and validates an int field from a parent map
// Returns error if field is missing or cannot be converted to int
func GetIntField(obj map[string]any, key, fieldName string) (int, error) {
	value, ok := obj[key]
	if !ok {
		return 0, fmt.Errorf("missing required '%s' field", fieldName)
	}

	intVal, ok := AsInt(value)
	if !ok {
		return 0, fmt.Errorf("'%s' must be an integer, got %T", fieldName, value)
	}

	return intVal, nil
}

// GetIntFieldWithDefault safely extracts an int field from a parent map with a default value
// Returns the value or default if field is missing
func GetIntFieldWithDefault(obj map[string]any, key string, defaultValue int) int {
	value, ok := obj[key]
	if !ok {
		return defaultValue
	}

	if intVal, ok := AsInt(value); ok {
		return intVal
	}

	return defaultValue
}

// ValidateArrayElementAsMap validates that an array element is a map
// Returns error with index information if validation fails
func ValidateArrayElementAsMap(element any, index int, elementType string) (map[string]any, error) {
	mapVal, ok := AsMap(element)
	if !ok {
		return nil, fmt.Errorf("%s at index %d must be an object, got %T", elementType, index, element)
	}

	return mapVal, nil
}

// ValidateRequiredMapFields validates that all required fields exist in a map
// Returns error with field name if any required field is missing
func ValidateRequiredMapFields(obj map[string]any, index int, elementType string, requiredFields []string) error {
	for _, field := range requiredFields {
		if _, ok := obj[field]; !ok {
			return fmt.Errorf("%s at index %d missing required '%s' field", elementType, index, field)
		}
	}
	return nil
}

// ExtractMapString safely extracts a string value from a nested map
// Returns the string or empty string if not found/invalid
func ExtractMapString(obj map[string]any, key string) string {
	if value, ok := obj[key]; ok {
		if strVal, ok := AsString(value); ok {
			return strVal
		}
	}
	return ""
}

// ExtractMapInt safely extracts an int value from a nested map
// Returns the int or 0 if not found/invalid
func ExtractMapInt(obj map[string]any, key string) int {
	if value, ok := obj[key]; ok {
		if intVal, ok := AsInt(value); ok {
			return intVal
		}
	}
	return 0
}

// ExtractMapBool safely extracts a bool value from a nested map
// Returns the bool or false if not found/invalid
func ExtractMapBool(obj map[string]any, key string) bool {
	if value, ok := obj[key]; ok {
		if boolVal, ok := AsBool(value); ok {
			return boolVal
		}
	}
	return false
}

// ExtractMapMap safely extracts a map value from a nested map
// Returns the map or nil if not found/invalid
func ExtractMapMap(obj map[string]any, key string) map[string]any {
	if value, ok := obj[key]; ok {
		if mapVal, ok := AsMap(value); ok {
			return mapVal
		}
	}
	return nil
}

// ExtractMapArray safely extracts an array value from a nested map
// Returns the array or nil if not found/invalid
func ExtractMapArray(obj map[string]any, key string) []any {
	if value, ok := obj[key]; ok {
		if arrayVal, ok := AsArray(value); ok {
			return arrayVal
		}
	}
	return nil
}
