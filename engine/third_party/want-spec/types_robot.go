package want_spec

// Resources of the board's robot — the Plan and the Act — and what they are
// made of (RobotStep, RobotCard). Written the Kubernetes way: apiVersion,
// kind, metadata, spec, and for what runs, status.

// ResourceAPIVersion is the apiVersion of the robot's resources.
const ResourceAPIVersion = "mywant.io/v1"

// ResourceMeta names a robot resource. The server gives it.
type ResourceMeta struct {
	// Name is what it is kept by: plan-<unix nanoseconds>, act-<…>.
	Name string `yaml:"name" json:"name"`
	// CreatedAt is when it was made, RFC 3339.
	CreatedAt string `yaml:"createdAt" json:"createdAt"`
}

// RobotCard is a want (or thing) a step brought the robot to, shown under
// its words. Type, Status and Summary are filled when the answer is settled,
// the same for every client that shows it.
type RobotCard struct {
	Kind    string `yaml:"kind" json:"kind"`
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Type    string `yaml:"type,omitempty" json:"type,omitempty"`
	Status  string `yaml:"status,omitempty" json:"status,omitempty"`
	Summary string `yaml:"summary,omitempty" json:"summary,omitempty"`
}

// RobotStep is one run of a tool — the same run POST /api/v1/tools/call
// makes.
type RobotStep struct {
	Tool      string            `yaml:"tool" json:"tool"`
	Arguments map[string]string `yaml:"arguments" json:"arguments"`
	Output    string            `yaml:"output,omitempty" json:"output,omitempty"`
	// Card is the want this step brought the robot to.
	Card *RobotCard `yaml:"card,omitempty" json:"card,omitempty"`
	// Done: run against the server already. A step posted without it (in a
	// turn) is run when the turn arrives.
	Done bool   `yaml:"done" json:"done"`
	At   string `yaml:"at,omitempty" json:"at,omitempty"`
}
