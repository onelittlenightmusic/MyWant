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
	Score        float64       `json:"score"`
	Description  string        `json:"description"`
	Source       FieldRef      `json:"source"`
	Target       ParamRef      `json:"target"`
	ParamChange  ParamChange   `json:"param_change"`
	ExposeAction *ExposeAction `json:"expose_action,omitempty"`
	// ImportAction is set when this recommendation should create an expose+import link
	// instead of (or in addition to) a param change.
	ImportAction *ImportAction `json:"import_action,omitempty"`
}

// ExposeAction describes the expose entry that will be added to the source want
// when this recommendation is applied (only set when the field is not yet exposed).
type ExposeAction struct {
	WantID    string `json:"want_id"`
	WantName  string `json:"want_name"`
	FieldName string `json:"field_name"`
	// GlobalKey is the expose "as" key (global state key). Defaults to FieldName if empty.
	GlobalKey string `json:"global_key,omitempty"`
}

// ImportAction describes the import entry that will be added to the target want's Spec.Imports.
type ImportAction struct {
	WantID    string `json:"want_id"`
	WantName  string `json:"want_name"`
	GlobalKey string `json:"global_key"` // key in Spec.Imports (must match a Spec.Exposes[*].As)
	LocalKey  string `json:"local_key"`  // internal state key in target want
}

// FieldRef describes the source field.
type FieldRef struct {
	WantID      string `json:"want_id"`
	WantName    string `json:"want_name"`
	FieldName   string `json:"field_name"`
	FieldType   string `json:"field_type"`   // "array", "string", "number", "bool", "object"
	Label       string `json:"label"`        // "current", "plan", "goal", or "" (unlabeled)
	IsFinal     bool   `json:"is_final"`     // true if this is the want type's finalResultField
	IsExposable bool   `json:"is_exposable"` // true if the want type declares exposable: true for this field
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

// GET /api/v1/wants/field-match-recommendations?source_id=xxx&target_id=yyy&exposed_labels=current
//
// exposed_labels (optional, default "current"): comma-separated list of state labels
// to expose from source. Valid values: "current", "plan", "goal". Unknown values are ignored.
// This corresponds to the GPC spatial model:
//   - horizontal drop (left/right) → exposed_labels=current
//   - vertical drop (above/below)  → exposed_labels=plan,goal
func (s *Server) getFieldMatchRecommendations(w http.ResponseWriter, r *http.Request) {
	sourceID := r.URL.Query().Get("source_id")
	targetID := r.URL.Query().Get("target_id")
	if sourceID == "" || targetID == "" {
		s.JSONError(w, r, http.StatusBadRequest, "source_id and target_id are required", "")
		return
	}

	exposedLabels := parseExposedLabels(r.URL.Query().Get("exposed_labels"))

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

	recs := computeExposeImportRecommendations(s, sourceWant, targetWant, exposedLabels)
	sort.Slice(recs, func(i, j int) bool { return recs[i].Score > recs[j].Score })

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"source_id":       sourceID,
		"target_id":       targetID,
		"exposed_labels":  exposedLabelsToStrings(exposedLabels),
		"recommendations": recs,
	})
}

// parseExposedLabels converts a comma-separated string like "plan,goal" into a set of StateLabel.
// Empty or missing input defaults to {LabelCurrent} for backward compatibility.
func parseExposedLabels(raw string) map[mywant.StateLabel]bool {
	out := map[mywant.StateLabel]bool{}
	if raw == "" {
		out[mywant.LabelCurrent] = true
		return out
	}
	for _, token := range strings.Split(raw, ",") {
		switch strings.TrimSpace(strings.ToLower(token)) {
		case "current":
			out[mywant.LabelCurrent] = true
		case "plan":
			out[mywant.LabelPlan] = true
		case "goal":
			out[mywant.LabelGoal] = true
		}
	}
	if len(out) == 0 {
		out[mywant.LabelCurrent] = true
	}
	return out
}

func exposedLabelsToStrings(set map[mywant.StateLabel]bool) []string {
	var out []string
	if set[mywant.LabelGoal] {
		out = append(out, "goal")
	}
	if set[mywant.LabelPlan] {
		out = append(out, "plan")
	}
	if set[mywant.LabelCurrent] {
		out = append(out, "current")
	}
	return out
}

// POST /api/v1/wants/field-match-recommendations/apply
// Supports two modes:
//   1. Param-change (legacy): set a param on target, auto-expose source field.
//      Body: { "source_id": "...", "target_id": "...", "param_change": { ... } }
//   2. Expose+Import (new): explicitly add expose entry to source and import entry to target.
//      Body: { "source_id": "...", "target_id": "...", "expose_action": { ... }, "import_action": { ... } }
// Both modes can be combined in one request.
func (s *Server) applyFieldMatchRecommendation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceID     string        `json:"source_id"`
		TargetID     string        `json:"target_id"`
		ParamChange  *ParamChange  `json:"param_change,omitempty"`
		ExposeAction *ExposeAction `json:"expose_action,omitempty"`
		ImportAction *ImportAction `json:"import_action,omitempty"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}
	if req.SourceID == "" || req.TargetID == "" {
		s.JSONError(w, r, http.StatusBadRequest, "source_id and target_id are required", "")
		return
	}
	if req.ParamChange == nil && req.ExposeAction == nil && req.ImportAction == nil {
		s.JSONError(w, r, http.StatusBadRequest, "at least one of param_change, expose_action, or import_action is required", "")
		return
	}
	// Validate that source want exists.
	if _, _, ok := s.globalBuilder.FindWantByID(req.SourceID); !ok {
		s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("source want %s not found", req.SourceID), "")
		return
	}

	result := map[string]any{
		"success":   true,
		"source_id": req.SourceID,
		"target_id": req.TargetID,
	}

	// ── Mode 1: Param-change (legacy) ──────────────────────────────────────
	if pc := req.ParamChange; pc != nil && pc.WantID != "" && pc.ParamName != "" {
		if pc.WantID != req.TargetID {
			s.JSONError(w, r, http.StatusBadRequest, "param_change.want_id must match target_id", "")
			return
		}
		var foundWant *mywant.Want
		if fw, _, ok := s.globalBuilder.FindWantByID(pc.WantID); ok {
			foundWant = fw
		}
		if foundWant == nil {
			s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("want %s not found", pc.WantID), "")
			return
		}
		newParams := make(map[string]any, len(foundWant.Spec.Params)+1)
		maps.Copy(newParams, foundWant.Spec.Params)
		newParams[pc.ParamName] = pc.Value
		updated := &mywant.Want{Metadata: foundWant.Metadata, Spec: foundWant.Spec}
		updated.Metadata.OwnerReferences = foundWant.Metadata.OwnerReferences
		updated.Spec.Params = newParams
		s.globalBuilder.UpdateWant(updated)
		result["param_name"] = pc.ParamName
		result["value"] = pc.Value

		// Auto-expose source field if param value looks like a field name.
		if fieldName, isStr := pc.Value.(string); isStr && fieldName != "" {
			if sourceWant, _, ok := s.globalBuilder.FindWantByID(req.SourceID); ok {
				if !hasExposeAs(sourceWant, fieldName) {
					srcUpdated := &mywant.Want{Metadata: sourceWant.Metadata, Spec: sourceWant.Spec}
					srcUpdated.Spec.Exposes = append(append([]mywant.ExposeEntry{}, sourceWant.Spec.Exposes...), mywant.ExposeEntry{
						As: fieldName, CurrentState: fieldName,
					})
					s.globalBuilder.UpdateWant(srcUpdated)
					result["exposed_field"] = fieldName
				}
			}
		}
	}

	// ── Mode 2a: Explicit expose action ───────────────────────────────────
	if ea := req.ExposeAction; ea != nil && ea.WantID != "" && ea.FieldName != "" {
		globalKey := ea.GlobalKey
		if globalKey == "" {
			globalKey = ea.FieldName
		}
		if sourceWant, _, ok := s.globalBuilder.FindWantByID(ea.WantID); ok {
			if !hasExposeAs(sourceWant, globalKey) {
				srcUpdated := &mywant.Want{Metadata: sourceWant.Metadata, Spec: sourceWant.Spec}
				srcUpdated.Spec.Exposes = append(append([]mywant.ExposeEntry{}, sourceWant.Spec.Exposes...), mywant.ExposeEntry{
					As: globalKey, CurrentState: ea.FieldName,
				})
				s.globalBuilder.UpdateWant(srcUpdated)
				result["exposed_global_key"] = globalKey
			}
		}
	}

	// ── Mode 2b: Explicit import action ───────────────────────────────────
	if ia := req.ImportAction; ia != nil && ia.WantID != "" && ia.GlobalKey != "" && ia.LocalKey != "" {
		if targetWant, _, ok := s.globalBuilder.FindWantByID(ia.WantID); ok {
			if _, exists := targetWant.Spec.Imports[ia.GlobalKey]; !exists {
				newImports := make(map[string]string, len(targetWant.Spec.Imports)+1)
				for k, v := range targetWant.Spec.Imports {
					newImports[k] = v
				}
				newImports[ia.GlobalKey] = ia.LocalKey
				tgtUpdated := &mywant.Want{Metadata: targetWant.Metadata, Spec: targetWant.Spec}
				tgtUpdated.Spec.Imports = newImports
				s.globalBuilder.UpdateWant(tgtUpdated)
				result["imported_key"] = ia.GlobalKey + "→" + ia.LocalKey
			}
		}
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/field-match-recommendations/apply", req.TargetID, "success", http.StatusOK, "",
		fmt.Sprintf("apply recommendation (source=%s → target=%s): %v", req.SourceID, req.TargetID, result))
	s.JSONResponse(w, http.StatusOK, result)
}

// hasExposeAs returns true if the want already has an expose entry with the given As key.
func hasExposeAs(w *mywant.Want, asKey string) bool {
	for _, e := range w.Spec.Exposes {
		if e.As == asKey {
			return true
		}
	}
	return false
}


// collectSourceFields enumerates state fields whose label is in allowedLabels,
// annotating each with its runtime type, label, and whether it is the finalResultField.
//
// Unlabeled fields (LabelNone) are treated as LabelCurrent for backward compatibility
// — they show up only when allowedLabels includes LabelCurrent.
func collectSourceFields(s *Server, want *mywant.Want, allowedLabels map[mywant.StateLabel]bool) []FieldRef {
	typeDef := s.globalBuilder.GetWantTypeDefinition(want.Metadata.Type)
	finalField := ""
	exposableFields := make(map[string]bool)
	if typeDef != nil {
		finalField = typeDef.FinalResultField
		for _, sd := range typeDef.State {
			if sd.Exposable {
				exposableFields[sd.Name] = true
			}
		}
	}

	// Build a set of framework-reserved field names to exclude from recommendations.
	reservedFields := make(map[string]bool)
	for _, f := range mywant.SystemReservedStateFields() {
		reservedFields[f] = true
	}

	state := want.GetExplicitState()
	var fields []FieldRef
	for k, v := range state {
		if strings.HasPrefix(k, "_") || reservedFields[k] {
			continue // skip internal fields (underscore-prefixed) and framework-reserved fields
		}
		label, hasLabel := want.StateLabels[k]
		// Treat unlabeled fields as "current" so legacy types still expose outputs.
		effective := label
		if !hasLabel || label == mywant.LabelNone {
			effective = mywant.LabelCurrent
		}
		if !allowedLabels[effective] {
			continue
		}
		fields = append(fields, FieldRef{
			WantID:      want.Metadata.ID,
			WantName:    want.Metadata.Name,
			FieldName:   k,
			FieldType:   runtimeTypeName(v),
			Label:       stateLabelString(effective),
			IsFinal:     k == finalField,
			IsExposable: exposableFields[k],
		})
	}
	return fields
}

func stateLabelString(label mywant.StateLabel) string {
	switch label {
	case mywant.LabelCurrent:
		return "current"
	case mywant.LabelPlan:
		return "plan"
	case mywant.LabelGoal:
		return "goal"
	default:
		return ""
	}
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

// computeExposeImportRecommendations suggests connecting source → target via the
// expose/import mechanism (Spec.Exposes + Spec.Imports) rather than a param change.
// This is the preferred approach for the generic data-flow linkage between any two wants.
func computeExposeImportRecommendations(s *Server, source, target *mywant.Want, allowedLabels map[mywant.StateLabel]bool) []FieldMatchRecommendation {
	sourceFields := collectSourceFields(s, source, allowedLabels)
	if len(sourceFields) == 0 {
		return nil
	}

	// Build map: globalKey already exposed by source (As value).
	alreadyExposedAs := make(map[string]string) // as → currentState
	for _, e := range source.Spec.Exposes {
		if e.As != "" {
			alreadyExposedAs[e.As] = e.CurrentState
		}
	}

	// Build map: globalKey already imported by target.
	alreadyImported := make(map[string]bool)
	for gk := range target.Spec.Imports {
		alreadyImported[gk] = true
	}

	// Build reverse map: currentState → existing expose As key.
	// Used to reuse an already-exposed key rather than generating a new one.
	fieldToExistingAs := make(map[string]string) // currentState → as
	for as, cs := range alreadyExposedAs {
		fieldToExistingAs[cs] = as
	}

	var recs []FieldMatchRecommendation
	for _, sf := range sourceFields {
		// Bug fix 1: Prefer the existing expose key for this field if the source
		// already exposes it. Only fall back to auto-generating a new key when the
		// field has not yet been exposed at all.
		globalKey := fieldToExistingAs[sf.FieldName]
		if globalKey == "" {
			globalKey = slugifyWantKey(source.Metadata.Name) + "_" + sf.FieldName
		}

		// Bug fix 2: Find the best local key in the TARGET want's state definition.
		// Using the source field name as-is caused mismatch (e.g. source "smartgolf_all_available_times"
		// mapped to target local key "smartgolf_all_available_times", but choice reads "choices").
		localKey := bestLocalKey(s, target, sf)

		// Skip if target already imports this global key.
		if alreadyImported[globalKey] {
			continue
		}

		// Score: finalResultField >> exposable >> current >> plan/goal
		score := 0.3
		if sf.IsFinal {
			score = 0.85
		} else if sf.IsExposable {
			score = 0.75
		} else if sf.Label == "current" {
			score = 0.55
		}

		// ExposeAction only when the source has not yet exposed this global key.
		var exposeAction *ExposeAction
		if _, exposed := alreadyExposedAs[globalKey]; !exposed {
			exposeAction = &ExposeAction{
				WantID:    source.Metadata.ID,
				WantName:  source.Metadata.Name,
				FieldName: sf.FieldName,
				GlobalKey: globalKey,
			}
		}

		recs = append(recs, FieldMatchRecommendation{
			Score:       score,
			Description: fmt.Sprintf("expose %s.%s → import as %s in %s", source.Metadata.Name, sf.FieldName, localKey, target.Metadata.Name),
			Source:      sf,
			Target: ParamRef{
				WantID:    target.Metadata.ID,
				WantName:  target.Metadata.Name,
				ParamName: localKey,
			},
			// ParamChange is left empty — this is an expose/import recommendation.
			ParamChange:  ParamChange{},
			ExposeAction: exposeAction,
			ImportAction: &ImportAction{
				WantID:    target.Metadata.ID,
				WantName:  target.Metadata.Name,
				GlobalKey: globalKey,
				LocalKey:  localKey,
			},
		})
	}
	return recs
}

// bestLocalKey finds the most appropriate local state key in the target want for receiving
// an imported value from the given source field.
//
// Priority:
//  1. Target state field whose name exactly matches the source field name.
//  2. Target state field with the same runtime type and semantic similarity (e.g. both arrays).
//  3. Fall back to the source field name (may not match any target state field).
func bestLocalKey(s *Server, target *mywant.Want, sf FieldRef) string {
	typeDef := s.globalBuilder.GetWantTypeDefinition(target.Metadata.Type)
	if typeDef == nil {
		return sf.FieldName
	}

	// 1. Exact name match.
	for _, st := range typeDef.State {
		if st.Name == sf.FieldName {
			return st.Name
		}
	}

	// 2. Type-based match: prefer state fields whose declared type matches the source
	//    runtime type. Among those, rank by name similarity to the source field.
	var sameType []string
	srcWords := splitWords(sf.FieldName)
	for _, st := range typeDef.State {
		if strings.EqualFold(st.Type, sf.FieldType) ||
			(sf.FieldType == "array" && (strings.EqualFold(st.Type, "array") || strings.Contains(strings.ToLower(st.Type), "[]"))) {
			sameType = append(sameType, st.Name)
		}
	}
	if len(sameType) == 1 {
		return sameType[0] // Only one candidate — use it.
	}
	if len(sameType) > 1 {
		// Pick the one with the most word overlap with the source field name.
		best, bestScore := sameType[0], -1
		for _, name := range sameType {
			overlap := len(wordIntersection(srcWords, splitWords(name)))
			if overlap > bestScore {
				bestScore, best = overlap, name
			}
		}
		return best
	}

	// 3. Fall back: source field name (may need manual adjustment).
	return sf.FieldName
}

// slugifyWantKey converts a want name to a safe global-key prefix.
// e.g. "My Want" → "my_want", "smartgolf" → "smartgolf".
func slugifyWantKey(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}
