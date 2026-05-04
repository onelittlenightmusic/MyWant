package mywant

// ChildRole represents the role an agent or child want plays in relation to its parent.
type ChildRole string

const (
	RoleThinker ChildRole = "thinker"
	RoleMonitor ChildRole = "monitor"
	RoleDoer    ChildRole = "doer"
	RoleAdmin   ChildRole = "admin"   // Full access for system components
	RoleUnknown ChildRole = "unknown" // Default/unspecified role
)

// GovernanceEngine enforces state access policies based on roles and labels.
type GovernanceEngine struct{}

// CanWriteParentState determines if a child with a given role is authorized
// to write to a parent's state field with a specific label.
func (g *GovernanceEngine) CanWriteParentState(role ChildRole, label StateLabel) bool {
	if role == RoleAdmin {
		return true
	}

	switch label {
	case LabelGoal:
		// Only thinkers can update the target/goal of a parent.
		return role == RoleThinker
	case LabelPlan:
		// Thinkers create plans; Doers might update them (e.g., marking steps as started).
		return role == RoleThinker || role == RoleDoer
	case LabelCurrent:
		// Monitors observe the world; Doers report their execution results.
		return role == RoleMonitor || role == RoleDoer
	case LabelInternal:
		// Internal state is private to the parent want's own Progress loop.
		return false
	default:
		// No label (LabelNone) is restricted by default to encourage explicit labeling.
		return false
	}
}

// CanReadParentState determines if a child is authorized to read a parent's state field.
func (g *GovernanceEngine) CanReadParentState(role ChildRole, label StateLabel) bool {
	if role == RoleAdmin {
		return true
	}
	// Generally, most state (except internal) is readable for coordination.
	return label != LabelInternal
}
