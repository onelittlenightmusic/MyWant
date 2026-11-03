package want_spec

// Constellation is a named group of things and wants drawn together on the
// board, as stars are. It is a facade over labels: each member carries
// constellation/<name>, and its colour, art and capture ride on the members
// the same way.
//
// With no Capture it is an order, drawn as a chain through Members in order.
// With one, it is a capture: Capture is its hub's tile, and it is drawn from
// the hub out to each member.
//
// When the hub is a want, the members are joined to that want's answers — one
// constellation for the want, not one per answer — and which answer each
// member is for is a label on the member:
//
//	constellation-answer/<name> = <the answer's key>
//
// The answers themselves are not members; the want is, as the hub.
//
//	GET    /api/v1/constellations                    ConstellationList (?kind=thing|want)
//	POST   /api/v1/constellations                    ConstellationCreate → Constellation
//	DELETE /api/v1/constellations?names=a,b          ConstellationsDeleted
//	GET    /api/v1/constellations/{name}             Constellation
//	PATCH  /api/v1/constellations/{name}             ConstellationUpdate; PUT is the same
//	DELETE /api/v1/constellations/{name}
//	POST   /api/v1/constellations/export[?names=a,b] YAML []Constellation
//	POST   /api/v1/constellations/import             YAML []Constellation → ConstellationsImported
//
// Import makes each by Name (else ID), or joins one already there, and lays on
// its Members, Color, Art, Capture and CaptureAt; Kind and MemberKinds are
// derived again, not read.
//
// See spec/constellation-spec.yaml.
type Constellation struct {
	// ID is the same as Name.
	ID string `yaml:"id" json:"id"`
	// Name is what it is called and kept by: not empty, no '/'.
	Name string `yaml:"name" json:"name"`
	// Kind is what its members are, derived from them: thing, want or mixed.
	Kind string `yaml:"kind" json:"kind"`
	// Color is its colour, "#rrggbb"; empty draws it in the default starlight.
	Color string `yaml:"color,omitempty" json:"color,omitempty"`
	// Art is the URL of a picture laid on the floor under it, maybe with a
	// fragment saying how it is drawn (#scale=1.5).
	Art string `yaml:"art,omitempty" json:"art,omitempty"`
	// Capture is the id of its hub's tile (a want's or a thing's); empty, an order.
	Capture string `yaml:"capture,omitempty" json:"capture,omitempty"`
	// CaptureAt is the cell its hub was last seen in, "x,y" — its ring stays
	// there when the hub is gone.
	CaptureAt string `yaml:"captureAt,omitempty" json:"captureAt,omitempty"`
	// Members are thing ids or want ids, in order.
	Members []string `yaml:"members" json:"members"`
	// MemberKinds says which kind each member is, keyed by the ids in Members.
	MemberKinds map[string]string `yaml:"memberKinds,omitempty" json:"memberKinds,omitempty"`
}

// ConstellationList is GET /constellations.
type ConstellationList struct {
	Groups []Constellation `yaml:"groups" json:"groups"`
}

// ConstellationCreate is the body of POST /constellations. A name already
// there folds the new members in. Kind decides nothing (each member is written
// where it lives) but is still validated when given.
type ConstellationCreate struct {
	Name    string   `yaml:"name" json:"name"`
	Kind    string   `yaml:"kind,omitempty" json:"kind,omitempty"`
	Members []string `yaml:"members" json:"members"`
	Color   string   `yaml:"color,omitempty" json:"color,omitempty"`
	// Capture is the tile it is a capture into (one of its members); empty, an order.
	Capture string `yaml:"capture,omitempty" json:"capture,omitempty"`
}

// ConstellationUpdate is the body of PUT /constellations/{name}. A nil field
// is left as it is; Members reconciles membership to the set given; "" for
// Color, Capture or CaptureAt clears it.
type ConstellationUpdate struct {
	Name      *string   `yaml:"name,omitempty" json:"name,omitempty"`
	Members   *[]string `yaml:"members,omitempty" json:"members,omitempty"`
	Kind      string    `yaml:"kind,omitempty" json:"kind,omitempty"`
	Color     *string   `yaml:"color,omitempty" json:"color,omitempty"`
	Capture   *string   `yaml:"capture,omitempty" json:"capture,omitempty"`
	CaptureAt *string   `yaml:"captureAt,omitempty" json:"captureAt,omitempty"`
}

// ConstellationsDeleted is DELETE /constellations?names=a,b: the names
// deleted, and those asked for that were not there.
type ConstellationsDeleted struct {
	Deleted []string `yaml:"deleted" json:"deleted"`
	Missing []string `yaml:"missing" json:"missing"`
}

// ConstellationsImported is POST /constellations/import: the names imported,
// in the file's order.
type ConstellationsImported struct {
	Imported []string `yaml:"imported" json:"imported"`
}

// Constellation kinds.
const (
	ConstellationKindThing = "thing"
	ConstellationKindWant  = "want"
	ConstellationKindMixed = "mixed"
)

// Label keys a constellation rides on, each followed by its name.
const (
	ConstellationLabelPrefix       = "constellation/"
	ConstellationAnswerLabelPrefix = "constellation-answer/"
)
