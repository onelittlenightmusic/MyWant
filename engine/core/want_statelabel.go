package mywant

// want_statelabel.go — labeled state accessors (Goal/Current/Plan/Internal) and generic helpers

// CalculateAchievingPercentage computes the progress percentage toward completion.
// Default implementation returns 0 unless the want has reached completion status.
// Want types should override this method for type-specific logic.
func (n *Want) CalculateAchievingPercentage() int {
	switch n.Status {
	case "completed", "achieved":
		return 100
	default:
		return 0
	}
}

// SetStateLabels populates the static StateLabels map from a WantTypeDefinition.
func (n *Want) SetStateLabels(def *WantTypeDefinition) {
	if def == nil {
		return
	}
	n.StateLabels = make(map[string]StateLabel)
	for _, s := range def.State {
		var label StateLabel
		switch s.Label {
		case "goal":
			label = LabelGoal
		case "current":
			label = LabelCurrent
		case "plan":
			label = LabelPlan
		case "internal":
			label = LabelInternal
		default:
			label = LabelNone
		}
		n.StateLabels[s.Name] = label
	}
}

func (n *Want) SetGoal(key string, value any) {
	if label, ok := n.StateLabels[key]; !ok || label != LabelGoal {
		WarnLog("[WARN] SetGoal(%q) on want %q (type=%s): key not declared with label 'goal' in state definition", key, n.Metadata.Name, n.Metadata.Type)
		n.governanceViolationCount++
	}
	n.storeState(key, value)
}

func (n *Want) GetGoal(key string) (any, bool) {
	if label, ok := n.StateLabels[key]; ok && label == LabelGoal {
		return n.getState(key)
	}
	return nil, false
}

func (n *Want) SetCurrent(key string, value any) {
	if label, ok := n.StateLabels[key]; !ok || label != LabelCurrent {
		WarnLog("[WARN] SetCurrent(%q) on want %q (type=%s): key not declared with label 'current' in state definition", key, n.Metadata.Name, n.Metadata.Type)
		n.governanceViolationCount++
	}
	n.storeState(key, value)
}

func (n *Want) GetCurrent(key string) (any, bool) {
	if label, ok := n.StateLabels[key]; ok && label == LabelCurrent {
		return n.getState(key)
	}
	return nil, false
}

// GetAllCurrent returns all state entries whose label is LabelCurrent.
func (n *Want) GetAllCurrent() map[string]any {
	return n.getAllByLabel(LabelCurrent)
}

// GetAllGoal returns all state entries whose label is LabelGoal.
func (n *Want) GetAllGoal() map[string]any {
	return n.getAllByLabel(LabelGoal)
}

// GetAllPlan returns all state entries whose label is LabelPlan.
func (n *Want) GetAllPlan() map[string]any {
	return n.getAllByLabel(LabelPlan)
}

// GetParentAllCurrent returns all current-labeled state entries from the parent want,
// or nil if there is no parent.
func (n *Want) GetParentAllCurrent() map[string]any {
	parent := n.getParentWant()
	if parent == nil {
		return nil
	}
	return parent.GetAllCurrent()
}

// GetParentAllGoal returns all goal-labeled state entries from the parent want,
// or nil if there is no parent.
func (n *Want) GetParentAllGoal() map[string]any {
	parent := n.getParentWant()
	if parent == nil {
		return nil
	}
	return parent.GetAllGoal()
}

// getAllByLabel collects all state key-value pairs registered under the given label.
func (n *Want) getAllByLabel(label StateLabel) map[string]any {
	result := make(map[string]any)
	for key, l := range n.StateLabels {
		if l != label {
			continue
		}
		if val, ok := n.getState(key); ok {
			result[key] = val
		}
	}
	return result
}

func (n *Want) SetPlan(key string, value any) {
	if label, ok := n.StateLabels[key]; !ok || label != LabelPlan {
		WarnLog("[WARN] SetPlan(%q) on want %q (type=%s): key not declared with label 'plan' in state definition", key, n.Metadata.Name, n.Metadata.Type)
		n.governanceViolationCount++
	}
	n.storeState(key, value)
}

func (n *Want) GetPlan(key string) (any, bool) {
	if label, ok := n.StateLabels[key]; ok && label == LabelPlan {
		return n.getState(key)
	}
	return nil, false
}

func (n *Want) ClearPlan(key string) {
	if label, ok := n.StateLabels[key]; ok && label == LabelPlan {
		n.storeState(key, nil)
	}
}

func (n *Want) SetInternal(key string, value any) {
	if label, ok := n.StateLabels[key]; !ok || label != LabelInternal {
		WarnLog("[WARN] SetInternal(%q) on want %q (type=%s): key not declared with label 'internal' in state definition", key, n.Metadata.Name, n.Metadata.Type)
		n.governanceViolationCount++
	}
	n.storeState(key, value)
}

func (n *Want) GetInternal(key string) (any, bool) {
	if label, ok := n.StateLabels[key]; ok && label == LabelInternal {
		return n.getState(key)
	}
	return nil, false
}

// WantPointer is an interface for types that can provide a pointer to their underlying Want.
// This allows our generic helpers to work with custom types that embed Want.
type WantPointer interface {
	GetWant() *Want
}

// GetWant implements the WantPointer interface for the base Want type.
func (n *Want) GetWant() *Want {
	return n
}

// GetState retrieves a value from the want's state with automatic type conversion.
func GetState[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.getState(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// GetGoal retrieves a goal-labeled value with automatic type conversion.
func GetGoal[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.GetGoal(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// GetCurrent retrieves a current-labeled value with automatic type conversion.
func GetCurrent[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.GetCurrent(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// GetPlan retrieves a plan-labeled value with automatic type conversion.
func GetPlan[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.GetPlan(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// GetInternal retrieves an internal-labeled value with automatic type conversion.
func GetInternal[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.GetInternal(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// GetParentState retrieves a value from the parent want's state with automatic type conversion.
func GetParentState[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.GetParentState(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// convertToType is an internal helper that bridges generics to our conversion utilities.
func convertToType[T any](val any, defaultVal T) T {
	if tVal, ok := val.(T); ok {
		return tVal
	}

	var result any
	switch d := any(defaultVal).(type) {
	case string:
		result = ToString(val, d)
	case int:
		result = ToInt(val, d)
	case bool:
		result = ToBool(val, d)
	case float64:
		result = ToFloat64(val, d)
	case []string:
		result = ToStringSlice(val, d)
	case []int:
		result = ToIntSlice(val, d)
	case []float64:
		result = ToFloat64Slice(val, d)
	default:
		return defaultVal
	}
	return result.(T)
}
