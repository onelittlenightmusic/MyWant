package server

import (
	"net/http"
	"strings"

	ws "github.com/onelittlenightmusic/want-spec"
	planner "mywant/engine/planner"
)

// GET /api/v1/datatypes
// Returns the full data type catalog (primitives + subtypes) with color/icon,
// plus a fieldTypeMap derived from live exposable state fields (for relation road coloring).
func (s *Server) getDataTypes(w http.ResponseWriter, r *http.Request) {
	// Build fieldTypeMap from exposable index.
	defs := s.globalBuilder.AllWantTypeDefinitions()
	wsDefs := make(map[string]*ws.WantTypeDefinition, len(defs))
	for k, v := range defs {
		wsDefs[k] = v
	}
	idx := planner.BuildExposableIndexFromDefs(wsDefs)

	fieldTypeMap := make(map[string]string)
	for _, f := range idx.All() {
		if _, seen := fieldTypeMap[f.Field]; seen {
			continue
		}
		key := fieldTypeFromDataTypes(f.Field)
		if key == "" {
			key = f.DataType // primitive fallback (string, number, bool, …)
		}
		if key == "" {
			key = "string"
		}
		fieldTypeMap[f.Field] = key
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"types":        DataTypeDefinitions(),
		"fieldTypeMap": fieldTypeMap,
	})
}

// fieldTypeFromDataTypes maps a state field name to a datatype catalog key
// using semantic name patterns. Most-specific patterns listed first.
func fieldTypeFromDataTypes(fieldName string) string {
	lower := strings.ToLower(fieldName)
	switch {
	case strings.Contains(lower, "album_art") ||
		strings.Contains(lower, "_image") ||
		strings.Contains(lower, "image_") ||
		strings.Contains(lower, "_artwork") ||
		strings.Contains(lower, "_photo"):
		return "image"
	case strings.HasSuffix(lower, "_url") ||
		strings.HasSuffix(lower, "_uri"):
		return "url"
	case strings.Contains(lower, "_station"):
		return "station"
	case strings.Contains(lower, "_city") ||
		strings.Contains(lower, "_location"):
		return "city"
	case strings.HasSuffix(lower, "_date") ||
		strings.HasPrefix(lower, "date_"):
		return "date"
	case strings.HasSuffix(lower, "_time") ||
		strings.HasPrefix(lower, "time_"):
		return "time"
	case strings.Contains(lower, "_condition") ||
		strings.HasSuffix(lower, "_status"):
		return "string"
	}
	return ""
}
