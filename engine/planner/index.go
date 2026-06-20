// Package planner provides backward-chaining plan derivation from WantTypePlan.
// It reads the exposable index (all want types with exposable:true state fields)
// and generates Recipe wiring automatically.
//
// The planner package has no dependency on mywant/engine/core — it operates
// purely on want-spec types and stdlib, making it safe to import from core.
package planner

import (
	"strings"

	ws "github.com/onelittlenightmusic/want-spec"
)

// ExposableField is an entry in the Planner's capability index.
// Built by scanning all registered WantTypeDefinitions for Exposable:true state fields.
type ExposableField struct {
	WantType    string `json:"wantType"`
	Field       string `json:"field"`       // StateDef.Name
	Description string `json:"description"` // StateDef.Description — used for semantic matching
	DataType    string `json:"dataType"`    // StateDef.Type (e.g. "string", "array", "boolean")
}

// ExposableIndex is a searchable collection of exposable fields across all want types.
type ExposableIndex struct {
	fields []ExposableField
}

// BuildExposableIndexFromDefs constructs an ExposableIndex from a raw definition map.
func BuildExposableIndexFromDefs(defs map[string]*ws.WantTypeDefinition) *ExposableIndex {
	idx := &ExposableIndex{}
	for typeName, def := range defs {
		for _, s := range def.State {
			if s.Exposable {
				idx.fields = append(idx.fields, ExposableField{
					WantType:    typeName,
					Field:       s.Name,
					Description: s.Description,
					DataType:    s.Type,
				})
			}
		}
	}
	return idx
}

// All returns all indexed exposable fields.
func (idx *ExposableIndex) All() []ExposableField {
	return idx.fields
}

// FindByExactName returns all exposable fields whose field name exactly matches name.
func (idx *ExposableIndex) FindByExactName(name string) []ExposableField {
	var out []ExposableField
	for _, f := range idx.fields {
		if f.Field == name {
			out = append(out, f)
		}
	}
	return out
}

// FindBySuffix returns exposable fields whose last underscore-separated token
// matches suffix (case-insensitive).
// For example: "first_room" → last token "room" → matches param "room".
// This avoids false positives like "next_datetime" matching "time".
func (idx *ExposableIndex) FindBySuffix(suffix string) []ExposableField {
	lower := strings.ToLower(suffix)
	var out []ExposableField
	for _, f := range idx.fields {
		tokens := strings.Split(strings.ToLower(f.Field), "_")
		lastToken := tokens[len(tokens)-1]
		if lastToken == lower {
			out = append(out, f)
		}
	}
	return out
}

// FindByDataType returns all exposable fields of the given data type.
func (idx *ExposableIndex) FindByDataType(dataType string) []ExposableField {
	var out []ExposableField
	for _, f := range idx.fields {
		if f.DataType == dataType {
			out = append(out, f)
		}
	}
	return out
}

// FindByWantType returns all exposable fields for a specific want type.
func (idx *ExposableIndex) FindByWantType(wantType string) []ExposableField {
	var out []ExposableField
	for _, f := range idx.fields {
		if f.WantType == wantType {
			out = append(out, f)
		}
	}
	return out
}
