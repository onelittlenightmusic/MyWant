package server

import "mywant/engine/labels"

// ThingLabelStore gives thing values the labels (metadata) that the thing
// catalog itself can't hold — it is just catalogKey → []value, with no
// per-value slot. Labels are keyed by a value id ("catalogKey::value") and are
// a plain key→value string map, mirroring a want's metadata.labels.
// Constellations ride on top of this via the reserved "constellation/<name>"
// keys (see handlers_constellations.go).
//
// The store itself is the shared labels.FileStore (persisted to
// ~/.mywant/thing-labels.yaml); this type only keeps the names the server has
// always called it by.
type ThingLabelStore struct {
	*labels.FileStore
}

func newThingLabelStore() *ThingLabelStore {
	return &ThingLabelStore{labels.NewFileStore(thingPath("thing-labels.yaml"))}
}

// ValuesWithLabel returns the value ids carrying the given label key.
func (m *ThingLabelStore) ValuesWithLabel(key string) []string {
	return m.IDsWithLabel(key)
}
