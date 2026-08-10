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
	// AsGlobalParam publishes the field as a global PARAMETER rather than global
	// state. Parameters are wired through the parameter side of the system
	// (asGlobalParam out, fromGlobalParam in); imports carry state and would
	// leave a parameter holding a key it cannot resolve — and would silently
	// make the target's own writes to that state key no-ops.
	AsGlobalParam string `json:"as_global_param,omitempty"`
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
	WantID   string `json:"want_id"`
	WantName string `json:"want_name"`
	// WantType lets a caller draw the provider's icon: with several wants
	// offering the same field name, the name alone does not say which is which.
	WantType string `json:"want_type,omitempty"`
	// CurrentValue is what the field holds right now. A recommendation carries
	// the field NAME as its param value (that is how the expose wiring reads),
	// which tells a person nothing — this is what to show them.
	CurrentValue any    `json:"current_value,omitempty"`
	FieldName    string `json:"field_name"`
	FieldType    string `json:"field_type"`          // runtime type: "array", "string", "number", "bool", "object"
	DataType     string `json:"data_type,omitempty"` // semantic type from want type def: "weather", "date", "url", etc.
	Label        string `json:"label"`               // "current", "plan", "goal", or "" (unlabeled)
	IsFinal      bool   `json:"is_final"`            // true if this is the want type's finalResultField
	IsExposable  bool   `json:"is_exposable"`        // true if the want type declares exposable: true for this field
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
	targetType := r.URL.Query().Get("target_type")
	if targetID == "" && targetType == "" {
		s.JSONError(w, r, http.StatusBadRequest, "target_id or target_type is required", "")
		return
	}

	exposedLabels := parseExposedLabels(r.URL.Query().Get("exposed_labels"))

	// The target may not exist yet — the Add Want form asks what a want of this
	// type could be filled with before anything is deployed. Scoring only reads
	// the target's type definition and its imports, so a want carrying just the
	// type answers the same question a deployed one would.
	var targetWant *mywant.Want
	if targetID != "" {
		tw, _, found := s.globalBuilder.FindWantByID(targetID)
		if !found {
			s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("target want %s not found", targetID), "")
			return
		}
		targetWant = tw
	} else {
		if s.globalBuilder.GetWantTypeDefinition(targetType) == nil {
			s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("want type %s not found", targetType), "")
			return
		}
		targetWant = &mywant.Want{Metadata: mywant.Metadata{Type: targetType, Name: targetType}}
	}

	// A named remembered value asks a different question from a want: not "what
	// could flow from there to here", but "does this want have somewhere to put
	// this". A thing is a literal — it exposes nothing and imports nothing — so
	// none of the provider machinery below applies, and the answer is complete
	// on its own.
	if thingID := r.URL.Query().Get("source_thing"); thingID != "" {
		catalog, value, ok := strings.Cut(thingID, "::")
		if !ok || catalog == "" || value == "" {
			s.JSONError(w, r, http.StatusBadRequest, "source_thing must be catalog::value", thingID)
			return
		}
		recs := computeThingValueRecommendations(s, targetWant, catalog, value)
		sort.Slice(recs, func(i, j int) bool { return recs[i].Score > recs[j].Score })
		s.JSONResponse(w, http.StatusOK, map[string]any{
			"source_thing":    thingID,
			"target_id":       targetID,
			"target_type":     targetType,
			"recommendations": recs,
		})
		return
	}

	// With no source named, every deployed want is a candidate provider — that is
	// the whole question the form is asking ("what out there could fill this?").
	var sources []*mywant.Want
	if sourceID != "" {
		sw, _, found := s.globalBuilder.FindWantByID(sourceID)
		if !found {
			s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("source want %s not found", sourceID), "")
			return
		}
		sources = []*mywant.Want{sw}
	} else if sourceType := r.URL.Query().Get("source_type"); sourceType != "" {
		sw := s.resolveSourceByType(sourceType)
		if sw == nil {
			s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("no deployed want of type %s", sourceType), "")
			return
		}
		sources = []*mywant.Want{sw}
	} else {
		for _, cw := range s.globalBuilder.GetWants() {
			if targetID != "" && cw.Metadata.ID == targetID {
				continue // a want cannot feed itself
			}
			sources = append(sources, cw)
		}
	}

	var recs []FieldMatchRecommendation
	for _, src := range sources {
		recs = append(recs, computeExposeImportRecommendations(s, src, targetWant, exposedLabels)...)
		recs = append(recs, computeStateParamRecommendations(s, src, targetWant, exposedLabels)...)
	}
	// Remembered values stand alongside live want fields as candidates.
	recs = append(recs, computeMemoRecommendations(s, targetWant)...)
	sort.Slice(recs, func(i, j int) bool { return recs[i].Score > recs[j].Score })

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"source_id":       sourceID,
		"target_id":       targetID,
		"target_type":     targetType,
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

// resolveSourceByType picks the deployed want that should provide a field when
// the caller names a provider by type rather than by id. Only a deployed want
// holds a live value, so a type resolves to an instance here; the most recently
// updated one wins, which is the one whose value is current.
func (s *Server) resolveSourceByType(wantType string) *mywant.Want {
	var best *mywant.Want
	for _, cw := range s.globalBuilder.GetWants() {
		if cw.Metadata.Type != wantType {
			continue
		}
		if best == nil || cw.Metadata.UpdatedAt > best.Metadata.UpdatedAt {
			best = cw
		}
	}
	return best
}

// POST /api/v1/wants/field-match-recommendations/apply
// Supports two modes:
//  1. Param-change (legacy): set a param on target, auto-expose source field.
//     Body: { "source_id": "...", "target_id": "...", "param_change": { ... } }
//  2. Expose+Import (new): explicitly add expose entry to source and import entry to target.
//     Body: { "source_id": "...", "target_id": "...", "expose_action": { ... }, "import_action": { ... } }
//
// Both modes can be combined in one request.
func (s *Server) applyFieldMatchRecommendation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceID   string `json:"source_id"`
		SourceType string `json:"source_type"`
		// SourceThing names a remembered value (catalog::value) instead of a
		// want. A literal has nothing to expose, so it only ever writes a param.
		SourceThing string `json:"source_thing"`
		// SourceField names the field to connect from. Its presence is what
		// asks for a connection rather than a plain value write.
		SourceField  string        `json:"source_field"`
		TargetID     string        `json:"target_id"`
		ParamChange  *ParamChange  `json:"param_change,omitempty"`
		ExposeAction *ExposeAction `json:"expose_action,omitempty"`
		ImportAction *ImportAction `json:"import_action,omitempty"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}
	// target_id may be empty: the Add Want form applies a recommendation to a
	// want that does not exist yet. The source-side expose still has to happen
	// here (the provider is real and must publish the field), while the param
	// and the import ride back in the response for the form to put into the want
	// it is about to deploy.
	targetPending := req.TargetID == ""
	if req.SourceID == "" && req.SourceType != "" {
		if sw := s.resolveSourceByType(req.SourceType); sw != nil {
			req.SourceID = sw.Metadata.ID
		}
	}
	// A remembered value is a literal: there is no provider to publish a field,
	// so the write to the target is the whole of the action.
	fromThing := req.SourceThing != ""
	if req.SourceID == "" && !fromThing {
		s.JSONError(w, r, http.StatusBadRequest, "source_id, source_type or source_thing is required", "")
		return
	}
	if req.ParamChange == nil && req.ExposeAction == nil && req.ImportAction == nil {
		s.JSONError(w, r, http.StatusBadRequest, "at least one of param_change, expose_action, or import_action is required", "")
		return
	}
	// Validate that source want exists.
	if !fromThing {
		if _, _, ok := s.globalBuilder.FindWantByID(req.SourceID); !ok {
			s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("source want %s not found", req.SourceID), "")
			return
		}
	}

	result := map[string]any{
		"success":   true,
		"source_id": req.SourceID,
		"target_id": req.TargetID,
	}

	// ── Mode 0: Connect a source field to a target parameter ───────────────
	// The caller names the two ends; the wiring is chosen here. A parameter is
	// fed from the parameter side (asGlobalParam out, fromGlobalParam in) —
	// imports carry state, and would leave the parameter holding a key it never
	// resolves while silently voiding the target's own writes to that key.
	if req.SourceField != "" && req.ParamChange != nil && req.ParamChange.ParamName != "" {
		sourceWant, _, ok := s.globalBuilder.FindWantByID(req.SourceID)
		if !ok {
			s.JSONError(w, r, http.StatusNotFound, fmt.Sprintf("source want %s not found", req.SourceID), "")
			return
		}
		paramKey := globalKeyFor(sourceWant.Metadata.Name, req.SourceField)
		if !hasExposeAsGlobalParam(sourceWant, paramKey) {
			srcUpdated := &mywant.Want{Metadata: sourceWant.Metadata, Spec: sourceWant.Spec}
			srcUpdated.Spec.Exposes = append(append([]mywant.ExposeEntry{}, sourceWant.Spec.Exposes...), mywant.ExposeEntry{
				CurrentState: req.SourceField, AsGlobalParam: paramKey,
			})
			s.globalBuilder.UpdateWant(srcUpdated)
		}
		ref := map[string]any{"fromGlobalParam": paramKey}
		result["exposed_global_param"] = paramKey
		if targetPending {
			result["pending_param_change"] = map[string]any{"param_name": req.ParamChange.ParamName, "value": ref}
		} else if tw, _, ok := s.globalBuilder.FindWantByID(req.TargetID); ok {
			newParams := make(map[string]any, len(tw.Spec.Params)+1)
			maps.Copy(newParams, tw.Spec.Params)
			newParams[req.ParamChange.ParamName] = ref
			updated := &mywant.Want{Metadata: tw.Metadata, Spec: tw.Spec}
			updated.Metadata.OwnerReferences = tw.Metadata.OwnerReferences
			updated.Spec.Params = newParams
			s.globalBuilder.UpdateWant(updated)
			result["param_name"] = req.ParamChange.ParamName
			result["value"] = ref
		}
		s.JSONResponse(w, http.StatusOK, result)
		return
	}

	// ── Mode 1: Param-change (legacy) ──────────────────────────────────────
	if pc := req.ParamChange; pc != nil && targetPending && pc.ParamName != "" {
		// Nothing to write to yet — hand the change back for the form to apply.
		result["pending_param_change"] = map[string]any{
			"param_name": pc.ParamName,
			"value":      pc.Value,
		}
	} else if pc := req.ParamChange; pc != nil && pc.WantID != "" && pc.ParamName != "" {
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
			if ea.AsGlobalParam != "" {
				if !hasExposeAsGlobalParam(sourceWant, ea.AsGlobalParam) {
					srcUpdated := &mywant.Want{Metadata: sourceWant.Metadata, Spec: sourceWant.Spec}
					srcUpdated.Spec.Exposes = append(append([]mywant.ExposeEntry{}, sourceWant.Spec.Exposes...), mywant.ExposeEntry{
						CurrentState: ea.FieldName, AsGlobalParam: ea.AsGlobalParam,
					})
					s.globalBuilder.UpdateWant(srcUpdated)
				}
				result["exposed_global_param"] = ea.AsGlobalParam
			} else if !hasExposeAs(sourceWant, globalKey) {
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
	if ia := req.ImportAction; ia != nil && targetPending && ia.GlobalKey != "" && ia.LocalKey != "" {
		// Same as the param above: the want that would import this does not
		// exist yet, so the entry goes back for the form to carry into it.
		result["pending_import"] = map[string]any{
			"global_key": ia.GlobalKey,
			"local_key":  ia.LocalKey,
		}
	} else if ia := req.ImportAction; ia != nil && ia.WantID != "" && ia.GlobalKey != "" && ia.LocalKey != "" {
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
func hasExposeAsGlobalParam(w *mywant.Want, key string) bool {
	for _, e := range w.Spec.Exposes {
		if e.AsGlobalParam == key {
			return true
		}
	}
	return false
}

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
	fieldDataTypes := make(map[string]string) // fieldName → semantic subtype from StateDef.SubType
	if typeDef != nil {
		finalField = typeDef.FinalResultField
		for _, sd := range typeDef.State {
			if sd.Exposable {
				exposableFields[sd.Name] = true
			}
			if sd.SubType != "" {
				fieldDataTypes[sd.Name] = sd.SubType
			}
		}
	}

	// Build declared-type lookup for nil-value fallback.
	declaredTypes := make(map[string]string)
	if typeDef != nil {
		for _, sd := range typeDef.State {
			declaredTypes[sd.Name] = strings.ToLower(sd.Type)
		}
	}

	// Build a set of framework-reserved field names to exclude from recommendations.
	reservedFields := make(map[string]bool)
	for _, f := range mywant.SystemReservedStateFields() {
		reservedFields[f] = true
	}

	// normalizeDecl converts a StateDef declared type to a runtime type name.
	normalizeDecl := func(t string) string {
		switch strings.ToLower(t) {
		case "string", "text":
			return "string"
		case "int", "integer", "float", "float64", "double", "number":
			return "number"
		case "bool", "boolean":
			return "bool"
		case "array", "slice":
			return "array"
		case "object", "map":
			return "object"
		default:
			return "string" // any / unknown → default to string
		}
	}

	state := want.GetExplicitState()

	// Start with runtime state fields.
	seen := make(map[string]bool)
	var fields []FieldRef
	for k, v := range state {
		if strings.HasPrefix(k, "_") || reservedFields[k] {
			continue
		}
		label, hasLabel := want.StateLabels[k]
		effective := label
		if !hasLabel || label == mywant.LabelNone {
			effective = mywant.LabelCurrent
		}
		if !allowedLabels[effective] {
			continue
		}
		fieldType := runtimeTypeName(v)
		if fieldType == "null" {
			fieldType = normalizeDecl(declaredTypes[k])
		}
		seen[k] = true
		fields = append(fields, FieldRef{
			WantID:       want.Metadata.ID,
			WantName:     want.Metadata.Name,
			WantType:     want.Metadata.Type,
			CurrentValue: v,
			FieldName:    k,
			FieldType:    fieldType,
			DataType:     fieldDataTypes[k],
			Label:        stateLabelString(effective),
			IsFinal:      k == finalField,
			IsExposable:  exposableFields[k],
		})
	}

	// Also include typeDef-declared fields that are not yet in the runtime state
	// (e.g. choice.selected before any selection is made).
	if typeDef != nil {
		for _, sd := range typeDef.State {
			if seen[sd.Name] || strings.HasPrefix(sd.Name, "_") || reservedFields[sd.Name] {
				continue
			}
			var effective mywant.StateLabel
			switch string(sd.Label) {
			case "plan":
				effective = mywant.LabelPlan
			case "goal":
				effective = mywant.LabelGoal
			case "current":
				effective = mywant.LabelCurrent
			default:
				effective = mywant.LabelCurrent
			}
			if !allowedLabels[effective] {
				continue
			}
			fields = append(fields, FieldRef{
				WantID:      want.Metadata.ID,
				WantName:    want.Metadata.Name,
				WantType:    want.Metadata.Type,
				FieldName:   sd.Name,
				FieldType:   normalizeDecl(string(sd.Type)),
				DataType:    sd.SubType,
				Label:       stateLabelString(effective),
				IsFinal:     sd.Name == finalField,
				IsExposable: sd.Exposable,
			})
		}
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
// computeStateParamRecommendations offers a deployed want's state fields for a
// target's parameters, matched on subtype alone.
//
// This is what chains two searches: one transit search holds the station it
// arrived at and the time it arrives, the next one needs a station and a time.
// Nothing declares "the arrival feeds the departure" — the subtypes say a
// station fits a station parameter, and the user picks which. A declared chain
// would only describe this one pair of types and would not read backwards.
//
// Unlike memo values these are not literals: applying one exposes the source
// field and imports it, so the second leg follows the first when it moves.
func computeStateParamRecommendations(s *Server, source, target *mywant.Want, allowedLabels map[mywant.StateLabel]bool) []FieldMatchRecommendation {
	def := s.globalBuilder.GetWantTypeDefinition(target.Metadata.Type)
	if def == nil || source.Metadata.ID == target.Metadata.ID {
		return nil
	}
	fields := collectSourceFields(s, source, allowedLabels)
	if len(fields) == 0 {
		return nil
	}

	var recs []FieldMatchRecommendation
	for _, p := range def.Parameters {
		if p.SubType == "" {
			continue
		}
		if v, ok := target.Spec.Params[p.Name]; ok && v != nil && v != "" {
			continue
		}
		for _, sf := range fields {
			if sf.DataType != p.SubType {
				continue
			}
			// Says only what could fill what. HOW the two get connected — as a
			// global parameter here, since the target is a parameter — is decided
			// when one is applied, not when it is offered. The value carried is
			// the field's current one, which is also what apply falls back to if
			// the wiring cannot be made.
			recs = append(recs, FieldMatchRecommendation{
				// Above every memo value: a station a want is standing at now
				// beats one merely remembered.
				Score:       0.9,
				Description: fmt.Sprintf("Take %s's %s as %s", source.Metadata.Name, sf.FieldName, p.Name),
				Source:      sf,
				Target: ParamRef{
					WantID:    target.Metadata.ID,
					WantName:  target.Metadata.Name,
					ParamName: p.Name,
				},
				ParamChange: ParamChange{
					WantID:    target.Metadata.ID,
					ParamName: p.Name,
					Value:     sf.CurrentValue,
				},
			})
		}
	}
	return recs
}

// computeThingValueRecommendations answers for one remembered value what the
// parameters of one want could do with it.
//
// The board asks this when a thing is set down beside a want. Where
// computeMemoRecommendations offers a parameter its best few candidates, this
// starts from the value and looks for somewhere to put it — the same match
// (a parameter's subtype against the catalogue the value lives in), read from
// the other end.
//
// A parameter that already holds a value is still offered, scored below an
// empty one. Setting a thing down next to a want is a deliberate act, and
// "replace what is in `from` with this station" is exactly what it usually
// means; refusing to say so would make the gesture silently do nothing.
func computeThingValueRecommendations(s *Server, target *mywant.Want, catalog, value string) []FieldMatchRecommendation {
	if target == nil {
		return nil
	}
	def := s.globalBuilder.GetWantTypeDefinition(target.Metadata.Type)
	if def == nil {
		return nil
	}
	var recs []FieldMatchRecommendation
	for _, p := range def.Parameters {
		// The parameter names a subtype; the value lives in that subtype's
		// catalogue or in another one, and only the first is a match.
		if p.SubType == "" || subtypeToKey(p.SubType) != catalog {
			continue
		}
		current, has := target.Spec.Params[p.Name]
		if has && current == value {
			continue // already exactly this — nothing to offer
		}
		filled := has && current != nil && current != ""
		score := 0.95
		description := fmt.Sprintf("Use %s %q for %s", p.SubType, value, p.Name)
		if filled {
			score = 0.6
			description = fmt.Sprintf("Replace %s with %s %q", p.Name, p.SubType, value)
		}
		recs = append(recs, FieldMatchRecommendation{
			Score:       score,
			Description: description,
			Source: FieldRef{
				WantName:  "thing",
				FieldName: p.SubType,
				FieldType: "string",
				DataType:  p.SubType,
				Label:     "thing",
			},
			Target: ParamRef{
				WantID:    target.Metadata.ID,
				WantName:  target.Metadata.Name,
				ParamName: p.Name,
			},
			ParamChange: ParamChange{
				WantID:    target.Metadata.ID,
				ParamName: p.Name,
				Value:     value,
			},
		})
	}
	return recs
}

// thingRecommendationLimit caps how many remembered values one parameter offers.
const thingRecommendationLimit = 3

// computeMemoRecommendations offers values the user has already used for a
// parameter's subtype.
//
// This is the only source that can fill a parameter no deployed want provides:
// a transit's `from` wants a station, and nothing in the system holds a station
// in its state — but every station the user has ever typed is in the memo. A
// memo value is a literal, so unlike a want-to-want match it needs no expose or
// import; applying one is just a param write.
//
// Scored below a live semantic match: a value that some want is holding right
// now beats one merely remembered, and among remembered ones the most recent
// wins (ThingStore.Suggestions returns them newest first).
func computeMemoRecommendations(s *Server, target *mywant.Want) []FieldMatchRecommendation {
	if s.thingStore == nil {
		return nil
	}
	def := s.globalBuilder.GetWantTypeDefinition(target.Metadata.Type)
	if def == nil {
		return nil
	}
	var recs []FieldMatchRecommendation
	for _, p := range def.Parameters {
		if p.SubType == "" {
			continue
		}
		// Already filled by the caller — do not suggest over an existing value.
		if v, ok := target.Spec.Params[p.Name]; ok && v != nil && v != "" {
			continue
		}
		for i, v := range s.thingStore.Suggestions(p.SubType, thingRecommendationLimit) {
			recs = append(recs, FieldMatchRecommendation{
				Score:       0.7 - float64(i)*0.05,
				Description: fmt.Sprintf("Use remembered %s %q for %s", p.SubType, v, p.Name),
				Source: FieldRef{
					WantName:  "thing",
					FieldName: p.SubType,
					FieldType: "string",
					DataType:  p.SubType,
					Label:     "thing",
				},
				Target: ParamRef{
					WantID:    target.Metadata.ID,
					WantName:  target.Metadata.Name,
					ParamName: p.Name,
				},
				ParamChange: ParamChange{
					WantID:    target.Metadata.ID,
					ParamName: p.Name,
					Value:     v,
				},
			})
		}
	}
	return recs
}

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
			globalKey = globalKeyFor(source.Metadata.Name, sf.FieldName)
		}

		// Bug fix 2: Find the best local key in the TARGET want's state definition.
		// Using the source field name as-is caused mismatch (e.g. source "smartgolf_all_available_times"
		// mapped to target local key "smartgolf_all_available_times", but choice reads "choices").
		localKey := bestLocalKey(s, target, sf)

		// Skip if target already imports this global key.
		if alreadyImported[globalKey] {
			continue
		}

		// Check semantic type match: source DataType matches the target field's declared type.
		// e.g. source field DataType="weather" and target localKey field has type="weather".
		semanticMatch := false
		if sf.DataType != "" {
			if targetDef := s.globalBuilder.GetWantTypeDefinition(target.Metadata.Type); targetDef != nil {
				for _, st := range targetDef.State {
					if st.Name == localKey && st.SubType == sf.DataType {
						semanticMatch = true
						break
					}
				}
			}
		}

		// Score: semantic type match >> finalResultField >> exposable >> current >> plan/goal
		score := 0.3
		if semanticMatch {
			score = 0.95
		} else if sf.IsFinal {
			score = 0.85
		} else if sf.IsExposable {
			score = 0.75
		} else if sf.Label == "current" {
			score = 0.55
		}
		// Boost score when the matched target field is plan/goal-labeled (it's an input slot).
		if targetDef := s.globalBuilder.GetWantTypeDefinition(target.Metadata.Type); targetDef != nil {
			for _, st := range targetDef.State {
				if st.Name == localKey && (st.Label == "plan" || st.Label == "goal") {
					if score < 0.8 {
						score += 0.15
					}
					break
				}
			}
		}

		// The target said outright that it wants this filled. That is a stronger
		// statement than any of the guesses above, which read names, labels and
		// types and infer intent from them — here the want type declared it. So
		// an importable slot outranks everything, and an importable slot whose
		// subType also matches is the top of the list: the pairing both sides
		// asked for. Without this a location want offered its lat, its lng and
		// its coordinate with nothing to separate them, and the one the target
		// could actually use was buried among the ones it could not.
		if targetDef := s.globalBuilder.GetWantTypeDefinition(target.Metadata.Type); targetDef != nil {
			for _, st := range targetDef.State {
				if st.Name != localKey || !st.Importable {
					continue
				}
				if semanticMatch {
					score = 1.0
				} else if score < 0.9 {
					score = 0.9
				}
				break
			}
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

	// 1b. SubType match: prefer state field whose subType matches the source DataType.
	// This handles cases like source "percentage" matching target "value_pct" (subType=percentage).
	if sf.DataType != "" {
		var subTypeMatches []string
		for _, st := range typeDef.State {
			if st.SubType == sf.DataType {
				subTypeMatches = append(subTypeMatches, st.Name)
			}
		}
		if len(subTypeMatches) == 1 {
			return subTypeMatches[0]
		}
		if len(subTypeMatches) > 1 {
			srcWords := splitWords(sf.FieldName)
			best, bestScore := subTypeMatches[0], -1
			for _, name := range subTypeMatches {
				overlap := len(wordIntersection(srcWords, splitWords(name)))
				if overlap > bestScore {
					bestScore, best = overlap, name
				}
			}
			return best
		}
	}

	// 2. Type-based match: prefer state fields whose declared type matches the source
	//    runtime type. Among those, rank by name similarity to the source field.
	// normalizeFieldType maps yaml-declared types to the runtime type names returned by
	// runtimeTypeName so that "int"/"float" correctly match "number".
	normalizeFieldType := func(t string) string {
		switch strings.ToLower(t) {
		case "int", "integer", "float", "float64", "double", "number":
			return "number"
		case "bool", "boolean":
			return "bool"
		case "array", "slice":
			return "array"
		case "object", "map":
			return "object"
		default:
			return strings.ToLower(t)
		}
	}
	// Build a label lookup for quick plan/goal detection.
	planOrGoal := make(map[string]bool, len(typeDef.State))
	for _, st := range typeDef.State {
		if st.Label == "plan" || st.Label == "goal" {
			planOrGoal[st.Name] = true
		}
	}

	var sameType []string
	srcWords := splitWords(sf.FieldName)
	for _, st := range typeDef.State {
		if normalizeFieldType(st.Type) == sf.FieldType ||
			(sf.FieldType == "array" && (strings.EqualFold(st.Type, "array") || strings.Contains(strings.ToLower(st.Type), "[]"))) {
			sameType = append(sameType, st.Name)
		}
	}
	if len(sameType) == 1 {
		return sameType[0] // Only one candidate — use it.
	}
	if len(sameType) > 1 {
		// Prefer plan/goal fields (they are inputs; current fields are outputs).
		var planSameType []string
		for _, name := range sameType {
			if planOrGoal[name] {
				planSameType = append(planSameType, name)
			}
		}
		candidates := planSameType
		if len(candidates) == 0 {
			candidates = sameType // No plan fields — fall back to all matches.
		}
		if len(candidates) == 1 {
			return candidates[0]
		}
		// Among candidates, pick the one with the most word overlap with the source field name.
		best, bestScore := candidates[0], -1
		for _, name := range candidates {
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
// globalKeyFor names the global slot a want's field is published into.
//
// The two halves are joined with a dot, not an underscore. Both halves already
// contain underscores of their own — spotify_instance and album_art_url — so an
// underscore between them left nothing to read the boundary by, and the key
// came out as one long word whose middle was a guess. A dot says where the want
// ends and the field begins.
//
// Keys made before this are unaffected: an expose and the import that reads it
// match on the exact string, and neither is rewritten. Only new connections are
// named this way.
func globalKeyFor(wantName, field string) string {
	return slugifyWantKey(wantName) + "." + field
}

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
