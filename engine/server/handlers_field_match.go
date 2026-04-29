package server

import (
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"sort"
	"strings"

	mywant "mywant/engine/core"
)

// FieldMatchRecommendation is a single suggestion to connect a source field to a target param.
// ID has been removed — recommendations are stateless; provenance is carried in the apply request.
type FieldMatchRecommendation struct {
	Score       float64     `json:"score"`
	Description string      `json:"description"`
	Source      FieldRef    `json:"source"`
	Target      ParamRef    `json:"target"`
	ParamChange ParamChange `json:"param_change"`
}

// FieldRef describes the source field.
type FieldRef struct {
	WantID    string `json:"want_id"`
	WantName  string `json:"want_name"`
	FieldName string `json:"field_name"`
	FieldType string `json:"field_type"` // "array", "string", "number", "bool", "object"
	IsFinal   bool   `json:"is_final"`   // true if this is the want type's finalResultField
}

// ParamRef describes the target parameter to be written.
type ParamRef struct {
	WantID    string `json:"want_id"`
	WantName  string `json:"want_name"`
	ParamName string `json:"param_name"`
}

// ParamChange is a single param write to apply on approval.
type ParamChange struct {
	WantID    string `json:"want_id"`
	ParamName string `json:"param_name"`
	Value     any    `json:"value"`
}

// GET /api/v1/wants/field-match-recommendations?source_id=xxx&target_id=yyy
func (s *Server) getFieldMatchRecommendations(w http.ResponseWriter, r *http.Request) {
	sourceID := r.URL.Query().Get("source_id")
	targetID := r.URL.Query().Get("target_id")
	if sourceID == "" || targetID == "" {
		s.JSONError(w, r, http.StatusBadRequest, "source_id and target_id are required", "")
		return
	}

	sourceWant, _, sourceFound := s.globalBuilder.FindWantByID(sourceID)
	targetWant, _, targetFound := s.globalBuilder.FindWantByID(targetID)
	if !sourceFound {
		s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("source want %s not found", sourceID), "")
		return
	}
	if !targetFound {
		s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("target want %s not found", targetID), "")
		return
	}

	recs := computeFieldMatchRecommendations(s, sourceWant, targetWant)

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"source_id":       sourceID,
		"target_id":       targetID,
		"recommendations": recs,
	})
}

// POST /api/v1/wants/field-match-recommendations/apply
// Body: { "source_id": "...", "target_id": "...", "param_change": { "want_id": "...", "param_name": "...", "value": ... } }
// source_id and target_id identify the want pair that generated the recommendation (for audit/logging).
func (s *Server) applyFieldMatchRecommendation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceID    string      `json:"source_id"`
		TargetID    string      `json:"target_id"`
		ParamChange ParamChange `json:"param_change"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}
	pc := req.ParamChange
	if req.SourceID == "" || req.TargetID == "" {
		s.JSONError(w, r, http.StatusBadRequest, "source_id and target_id are required", "")
		return
	}
	if pc.WantID == "" || pc.ParamName == "" {
		s.JSONError(w, r, http.StatusBadRequest, "param_change.want_id and param_name are required", "")
		return
	}
	// Validate that source want exists (confirms the recommendation context is genuine).
	if _, _, ok := s.globalBuilder.FindWantByID(req.SourceID); !ok {
		s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("source want %s not found", req.SourceID), "")
		return
	}
	// param_change.want_id must match target_id.
	if pc.WantID != req.TargetID {
		s.JSONError(w, r, http.StatusBadRequest, "param_change.want_id must match target_id", "")
		return
	}

	// Find under read-lock to get a snapshot, then build a new Want with the param applied
	// rather than mutating the live pointer — avoids concurrent map write races.
	s.wantsMu.RLock()
	var foundWant *mywant.Want
	for _, exec := range s.wants {
		if exec.Builder != nil {
			if w, _, ok := exec.Builder.FindWantByID(pc.WantID); ok {
				foundWant = w
				break
			}
		}
	}
	s.wantsMu.RUnlock()

	if foundWant == nil {
		if w, _, ok := s.globalBuilder.FindWantByID(pc.WantID); ok {
			foundWant = w
		}
	}
	if foundWant == nil {
		s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("want %s not found", pc.WantID), "")
		return
	}

	// Copy params map to avoid mutating the live want under concurrent requests.
	newParams := make(map[string]any, len(foundWant.Spec.Params)+1)
	maps.Copy(newParams, foundWant.Spec.Params)
	newParams[pc.ParamName] = pc.Value

	// Build a minimal updated want that UpdateWant can safely apply.
	updated := &mywant.Want{
		Metadata: foundWant.Metadata,
		Spec:     foundWant.Spec,
	}
	updated.Metadata.OwnerReferences = foundWant.Metadata.OwnerReferences
	updated.Spec.Params = newParams
	s.globalBuilder.UpdateWant(updated)

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/field-match-recommendations/apply", pc.WantID, "success", http.StatusOK, "",
		fmt.Sprintf("Applied param %s=%v (source=%s → target=%s)", pc.ParamName, pc.Value, req.SourceID, req.TargetID))
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"success":    true,
		"source_id":  req.SourceID,
		"target_id":  req.TargetID,
		"param_name": pc.ParamName,
		"value":      pc.Value,
	})
}

// computeFieldMatchRecommendations scores all combinations of source current fields
// against target *_import_field / *_source_* parameters.
func computeFieldMatchRecommendations(s *Server, source, target *mywant.Want) []FieldMatchRecommendation {
	// Collect source current fields
	sourceFields := collectSourceFields(s, source)
	// Collect target import-style parameters
	targetParams := collectTargetImportParams(s, target)

	if len(sourceFields) == 0 || len(targetParams) == 0 {
		return []FieldMatchRecommendation{}
	}

	var recs []FieldMatchRecommendation
	for _, sf := range sourceFields {
		for _, tp := range targetParams {
			score := scoreMatch(sf, tp)
			if score <= 0 {
				continue
			}
			recs = append(recs, FieldMatchRecommendation{
				Score:       score,
				Description: fmt.Sprintf("%s.%s → %s.%s", source.Metadata.Name, sf.FieldName, target.Metadata.Name, tp),
				Source:      sf,
				Target: ParamRef{
					WantID:    target.Metadata.ID,
					WantName:  target.Metadata.Name,
					ParamName: tp,
				},
				ParamChange: ParamChange{
					WantID:    target.Metadata.ID,
					ParamName: tp,
					Value:     sf.FieldName,
				},
			})
		}
	}

	sort.Slice(recs, func(i, j int) bool { return recs[i].Score > recs[j].Score })
	return recs
}

// collectSourceFields enumerates fields from want.state.current,
// annotating each with its runtime type and whether it is the finalResultField.
func collectSourceFields(s *Server, want *mywant.Want) []FieldRef {
	typeDef := s.globalBuilder.GetWantTypeDefinition(want.Metadata.Type)
	finalField := ""
	if typeDef != nil {
		finalField = typeDef.FinalResultField
	}

	state := want.GetExplicitState()
	var fields []FieldRef
	for k, v := range state {
		if strings.HasPrefix(k, "_") {
			continue // skip internal fields
		}
		label, hasLabel := want.StateLabels[k]
		if hasLabel && label != mywant.LabelCurrent {
			continue // only current-labelled fields are readable outputs
		}
		fields = append(fields, FieldRef{
			WantID:    want.Metadata.ID,
			WantName:  want.Metadata.Name,
			FieldName: k,
			FieldType: runtimeTypeName(v),
			IsFinal:   k == finalField,
		})
	}
	return fields
}

// collectTargetImportParams returns parameter names from the target want type definition
// that look like "import field" selectors (e.g. choice_import_field, source_field, *_import_*).
func collectTargetImportParams(s *Server, want *mywant.Want) []string {
	typeDef := s.globalBuilder.GetWantTypeDefinition(want.Metadata.Type)
	if typeDef == nil {
		return nil
	}

	var params []string
	for _, p := range typeDef.Parameters {
		if isImportParam(p.Name) {
			params = append(params, p.Name)
		}
	}
	return params
}

// isImportParam returns true if the parameter name looks like a field-import selector.
func isImportParam(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "_import_field") ||
		strings.Contains(lower, "_source_field") ||
		strings.HasSuffix(lower, "_field") ||
		strings.Contains(lower, "_from_field")
}

// scoreMatch returns a score [0,1] for how well a source field matches a target param name.
// 0 means not a match at all.
func scoreMatch(sf FieldRef, targetParam string) float64 {
	score := 0.0

	// finalResultField is the canonical output → highest boost
	if sf.IsFinal {
		score += 0.5
	}

	// Array/slice fields match "choices"-style params especially well
	targetLower := strings.ToLower(targetParam)
	if sf.FieldType == "array" && (strings.Contains(targetLower, "choice") || strings.Contains(targetLower, "list") || strings.Contains(targetLower, "import")) {
		score += 0.4
	}

	// Name similarity: words in common between field name and param name
	fieldWords := splitWords(sf.FieldName)
	paramWords := splitWords(targetParam)
	common := wordIntersection(fieldWords, paramWords)
	if len(common) > 0 {
		score += 0.1 * float64(len(common))
	}

	// Any non-zero score qualifies (minimum baseline so every candidate appears)
	if score == 0 {
		score = 0.1
	}
	return score
}

func runtimeTypeName(v any) string {
	if v == nil {
		return "null"
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map:
		return "object"
	case reflect.Bool:
		return "bool"
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	default:
		return "unknown"
	}
}

func splitWords(s string) []string {
	// Split on _ and camelCase boundaries
	s = strings.ReplaceAll(s, "-", "_")
	parts := strings.Split(s, "_")
	var words []string
	for _, p := range parts {
		if p != "" {
			words = append(words, strings.ToLower(p))
		}
	}
	return words
}

func wordIntersection(a, b []string) []string {
	set := make(map[string]bool, len(a))
	for _, w := range a {
		set[w] = true
	}
	var common []string
	for _, w := range b {
		if set[w] {
			common = append(common, w)
		}
	}
	return common
}
