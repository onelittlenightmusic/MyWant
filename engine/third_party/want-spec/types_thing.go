package want_spec

// ThingEntry is one thing as the backend's thing store keeps it — one value
// the city remembers (a station, a place, a date, …): what a parameter with a
// SubType records when a person types into it (RecordThing), and what a
// character picks up and throws on the board.
//
// Its ID is its identity: it survives the thing being refiled or renamed, and
// everything pointing at it (where it sits on the board, which Constellation
// it joined) is keyed by it.
//
//	POST   /api/v1/things                    ThingWrite → ThingEntry
//	DELETE /api/v1/things?ids=a,b            ThingsDeleted
//	PATCH  /api/v1/things/{id}               ThingWrite (either field); PUT is the same
//	DELETE /api/v1/things/{id}
//	POST   /api/v1/things/{id}/labels        ThingLabelWrite
//	PATCH  /api/v1/things/{id}/labels        ThingLabelsPatch → ThingLabels
//	DELETE /api/v1/things/{id}/labels/{key}
//	POST   /api/v1/things/export[?ids=a,b]   YAML []ThingExport
//	POST   /api/v1/things/import             YAML []ThingExport → ThingsImported
//
// See spec/thing-spec.yaml.
type ThingEntry struct {
	// ID is "thg-<uuid>", made once when it is added.
	ID string `yaml:"id" json:"id"`
	// Catalog is what it is filed under: its subtype's catalog key ("stations").
	Catalog string `yaml:"catalog" json:"catalog"`
	// Value is what it is called — what is shown for it.
	Value string `yaml:"value" json:"value"`
	// CreatedAt is when it was added, RFC 3339. Things added before it was
	// kept have none; ThingFull.AddedOrder still orders them.
	CreatedAt string `yaml:"createdAt,omitempty" json:"createdAt,omitempty"`
}

// ThingWrite is the body of POST /things (both fields required) and PATCH
// /things/{id} (either). Catalog may be the catalog key or the subtype.
type ThingWrite struct {
	Catalog string `yaml:"catalog,omitempty" json:"catalog,omitempty"`
	Value   string `yaml:"value,omitempty" json:"value,omitempty"`
}

// Thing is one thing as it is read — the entry and what is derived about it.
//
//	GET /api/v1/things        ThingList
//	GET /api/v1/things/{id}   Thing
type Thing struct {
	ID      string `yaml:"id" json:"id"`
	Catalog string `yaml:"catalog" json:"catalog"`
	// Subtype is the SubType its Catalog is the catalog of ("station").
	Subtype string `yaml:"subtype" json:"subtype"`
	Value   string `yaml:"value" json:"value"`

	// AddedOrder is its place among all things in the order they were added,
	// 0 the first. With CreatedAt, sorts newest first.
	AddedOrder int    `yaml:"addedOrder" json:"addedOrder"`
	CreatedAt  string `yaml:"createdAt,omitempty" json:"createdAt,omitempty"`

	// Icon and Color are its subtype's.
	Icon  string `yaml:"icon" json:"icon"`
	Color string `yaml:"color" json:"color"`
	// Background is its subtype's picture name, for a subtype that has one.
	Background string `yaml:"background,omitempty" json:"background,omitempty"`

	// Definitions are the names given to this value, by every character.
	Definitions []ThingDefinition `yaml:"definitions,omitempty" json:"definitions,omitempty"`
	Stats       *ThingStats       `yaml:"stats,omitempty" json:"stats,omitempty"`
	// WantIDs are the live wants naming this value right now.
	WantIDs []string `yaml:"wantIDs,omitempty" json:"wantIDs,omitempty"`
	// ListWantIDs are those of them that name it as one of several — a stop
	// on a route rather than either end. The board draws those dashed.
	ListWantIDs []string `yaml:"listWantIDs,omitempty" json:"listWantIDs,omitempty"`
	// Labels are its labels; constellations ride on constellation/<name> keys.
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// ThingList is GET /things.
type ThingList struct {
	Things []Thing `yaml:"things" json:"things"`
}

// ThingDefinition is one name given to a value, by one character, from one want.
type ThingDefinition struct {
	Catalog string `yaml:"catalog" json:"catalog"`
	Subtype string `yaml:"subtype" json:"subtype"`
	Name    string `yaml:"name" json:"name"`
	Value   any    `yaml:"value,omitempty" json:"value,omitempty"`
	At      string `yaml:"at" json:"at"`

	CharacterID   string `yaml:"characterId,omitempty" json:"characterId,omitempty"`
	CharacterName string `yaml:"characterName,omitempty" json:"characterName,omitempty"`

	WantID   string `yaml:"wantId,omitempty" json:"wantId,omitempty"`
	WantType string `yaml:"wantType,omitempty" json:"wantType,omitempty"`
}

// ThingStats is how a value has been used (the backend's MemoStat).
type ThingStats struct {
	// Count is the number of recorded uses.
	Count int `yaml:"count" json:"count"`
	// LastUsed is the most recent use, RFC 3339.
	LastUsed string `yaml:"lastUsed" json:"lastUsed"`
	// TopWantTypes are the most-used want types, most first (up to 3).
	TopWantTypes []WantTypeCount `yaml:"topWantTypes" json:"topWantTypes"`
}

// WantTypeCount is how many times one want type used a value.
type WantTypeCount struct {
	Type  string `yaml:"type" json:"type"`
	Count int    `yaml:"count" json:"count"`
}

// ThingLabelWrite is the body of POST /things/{id}/labels — one label put on.
// A picture-url label (ThingPictureLabel) has the words in the picture read.
type ThingLabelWrite struct {
	Key   string `yaml:"key" json:"key"`
	Value string `yaml:"value" json:"value"`
}

// ThingLabelsPatch is the body of PATCH /things/{id}/labels — several labels
// put on (Set, replacing a value already there) and taken off (Remove) at
// once, told as one change. A picture-url in Set has the picture read too.
type ThingLabelsPatch struct {
	Set    map[string]string `yaml:"set,omitempty" json:"set,omitempty"`
	Remove []string          `yaml:"remove,omitempty" json:"remove,omitempty"`
}

// ThingLabels is PATCH /things/{id}/labels: the thing's labels after the
// change, all of them.
type ThingLabels struct {
	ID     string            `yaml:"id" json:"id"`
	Labels map[string]string `yaml:"labels" json:"labels"`
}

// ThingsDeleted is DELETE /things?ids=a,b: the ids deleted, and those asked
// for that were not there.
type ThingsDeleted struct {
	Deleted []string `yaml:"deleted" json:"deleted"`
	Missing []string `yaml:"missing" json:"missing"`
}

// ThingExport is one thing as POST /things/export writes it out and POST
// /things/import reads it back in: its entry and its labels. Import matches
// it by Catalog and Value — one already there keeps its id, and ID here is not
// used — and lays Labels on it.
type ThingExport struct {
	ID        string            `yaml:"id,omitempty" json:"id,omitempty"`
	Catalog   string            `yaml:"catalog" json:"catalog"`
	Value     string            `yaml:"value" json:"value"`
	CreatedAt string            `yaml:"createdAt,omitempty" json:"createdAt,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// ThingsImported is POST /things/import: the ids imported, in the file's order.
type ThingsImported struct {
	Imported []string `yaml:"imported" json:"imported"`
}

// ThingIDPrefix starts every thing's id.
const ThingIDPrefix = "thg-"

// ThingPictureLabel is the label holding a thing's picture; putting it on has
// the words in the picture read.
const ThingPictureLabel = "picture-url"
