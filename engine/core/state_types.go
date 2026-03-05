package mywant

// StateLabel is an enum for state classifications, indicating the semantic
// purpose of a state field.
type StateLabel int

const (
	LabelNone StateLabel = iota // Default/unspecified
	LabelGoal
	LabelCurrent
	LabelPlan
	LabelPredefined
	LabelInternal
)
