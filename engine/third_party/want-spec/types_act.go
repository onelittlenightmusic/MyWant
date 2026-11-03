package want_spec

import "fmt"

// Act is a Plan carried out, kept like a Kubernetes Job. Posting one that
// names a Plan is what makes something happen on the board; it is answered at
// once, Pending, and carried out behind it, its status written as it goes.
//
//	apiVersion: mywant.io/v1
//	kind: Act
//	metadata:
//	  name: act-1730…
//	  createdAt: …
//	spec:
//	  plan: plan-1730…           # the Plan carried out
//	  turn: turn-…               # the robot's turn it belongs to, if any
//	  locale: ja_JP
//	status:
//	  phase: Running             # Pending → Running → Succeeded | Failed
//	  startTime: …
//	  completionTime: …
//	  steps: [RobotStep…]        # each tool run, as it is done
//	  cards: [RobotCard…]        # what the steps showed
//	  prompt: …                  # when done: what to word the answer from
//	  message: …                 # when failed: why
//
//	POST   /api/v1/acts                Act (YAML or JSON) → Act, Pending
//	GET    /api/v1/acts                ActList, newest first (?limit=)
//	GET    /api/v1/acts/{name}         Act as far as it has got;
//	                                   ?wait=<seconds> holds it until done
//	DELETE /api/v1/acts/{name}         its record let go; what it did stays
//
// See spec/act-spec.yaml.
type Act struct {
	APIVersion string       `yaml:"apiVersion" json:"apiVersion"`
	Kind       string       `yaml:"kind" json:"kind"`
	Metadata   ResourceMeta `yaml:"metadata" json:"metadata"`
	Spec       ActSpec      `yaml:"spec" json:"spec"`
	Status     ActStatus    `yaml:"status" json:"status"`
}

// ActKind is an Act's kind.
const ActKind = "Act"

// An act's phases.
const (
	ActPending   = "Pending"
	ActRunning   = "Running"
	ActSucceeded = "Succeeded"
	ActFailed    = "Failed"
)

// ActSpec is which plan is carried out, and for whom.
type ActSpec struct {
	// Plan is the name of the Plan carried out.
	Plan string `yaml:"plan" json:"plan"`
	// Turn is the robot's turn it belongs to, if any.
	Turn   string `yaml:"turn,omitempty" json:"turn,omitempty"`
	Locale string `yaml:"locale,omitempty" json:"locale,omitempty"`
}

// ActStatus is how far it has got.
type ActStatus struct {
	Phase          string       `yaml:"phase" json:"phase"`
	StartTime      string       `yaml:"startTime,omitempty" json:"startTime,omitempty"`
	CompletionTime string       `yaml:"completionTime,omitempty" json:"completionTime,omitempty"`
	Steps          []RobotStep  `yaml:"steps" json:"steps"`
	Cards          []*RobotCard `yaml:"cards" json:"cards"`
	// Prompt, when done: what to word the answer from.
	Prompt string `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	// Message, when failed: why.
	Message string `yaml:"message,omitempty" json:"message,omitempty"`
}

// ActList is GET /acts.
type ActList struct {
	Acts []Act `yaml:"acts" json:"acts"`
}

// Finished: Succeeded or Failed.
func (a *Act) Finished() bool {
	return a.Status.Phase == ActSucceeded || a.Status.Phase == ActFailed
}

// Validate says what is wrong with an act as posted.
func (a *Act) Validate() error {
	if a.Kind != "" && a.Kind != ActKind {
		return fmt.Errorf("kind must be %s", ActKind)
	}
	if a.Spec.Plan == "" {
		return fmt.Errorf("spec.plan is required")
	}
	return nil
}
