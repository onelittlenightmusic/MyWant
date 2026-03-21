package mywant

import "maps"

// DataObject is a typed runtime packet exchanged via Use/Provide.
// Schema is loaded from yaml/data/*.yaml (JSON Schema format).
type DataObject struct {
	typeName string
	data     map[string]any
}

// NewDataObject creates an empty DataObject with the given type name.
func NewDataObject(typeName string) *DataObject {
	return &DataObject{
		typeName: typeName,
		data:     make(map[string]any),
	}
}

// NewDataObjectFrom wraps an existing map in a DataObject with the given type name.
func NewDataObjectFrom(typeName string, data map[string]any) *DataObject {
	if data == nil {
		data = make(map[string]any)
	}
	return &DataObject{
		typeName: typeName,
		data:     data,
	}
}

// Get returns the value for key, or defaultVal if absent.
func (d *DataObject) Get(key string, defaultVal any) any {
	if d == nil || d.data == nil {
		return defaultVal
	}
	val, ok := d.data[key]
	if !ok {
		return defaultVal
	}
	return val
}

// Set sets key to value.
func (d *DataObject) Set(key string, value any) {
	if d == nil {
		return
	}
	if d.data == nil {
		d.data = make(map[string]any)
	}
	d.data[key] = value
}

// TypeName returns the data type name.
func (d *DataObject) TypeName() string {
	if d == nil {
		return ""
	}
	return d.typeName
}

// GetTyped retrieves a typed value from a DataObject, eliminating manual type assertions.
// T is inferred from defaultVal. Returns defaultVal if key is absent or type doesn't match.
//
// Example:
//
//	reactionType := mywant.GetTyped(obj, "reaction_type", "internal")  // string inferred
//	num := mywant.GetTyped(obj, "num", 0)                              // int inferred
func GetTyped[T any](d *DataObject, key string, defaultVal T) T {
	if d == nil || d.data == nil {
		return defaultVal
	}
	val, ok := d.data[key]
	if !ok {
		return defaultVal
	}
	if v, ok := val.(T); ok {
		return v
	}
	return defaultVal
}

// ToMap returns a copy of the internal data map.
func (d *DataObject) ToMap() map[string]any {
	if d == nil || d.data == nil {
		return make(map[string]any)
	}
	result := make(map[string]any, len(d.data))
	maps.Copy(result, d.data)
	return result
}
