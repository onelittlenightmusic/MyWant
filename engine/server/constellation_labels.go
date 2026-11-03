package server

import (
	"encoding/json"
	"strings"
)

// A constellation's own labels (not its members') — key/value pairs to find
// and group it by, as every object has. A constellation has no record of its
// own to keep them in: it is a facade over its members' labels. So they ride
// on every member, as its colour does ("constellation-labels/<name>" = the
// labels as compact JSON, keys sorted), joining and leaving with them and
// carried to a new name on a rename.
const constellationLabelsPrefix = "constellation-labels/"

func constellationLabelsKey(name string) string { return constellationLabelsPrefix + name }

func constellationLabelsNameFromKey(key string) string {
	if name, ok := strings.CutPrefix(key, constellationLabelsPrefix); ok {
		return name
	}
	return ""
}

// encodeConstellationLabels is the value the members carry; "" for none.
func encodeConstellationLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	data, err := json.Marshal(labels) // keys sorted
	if err != nil {
		return ""
	}
	return string(data)
}

// decodeConstellationLabels reads what the members carry; nil for none or
// for what is not labels.
func decodeConstellationLabels(v string) map[string]string {
	if v == "" {
		return nil
	}
	var labels map[string]string
	if err := json.Unmarshal([]byte(v), &labels); err != nil || len(labels) == 0 {
		return nil
	}
	return labels
}

// anyConstellationLabels is what a member of either kind carries for the
// name's labels, encoded; "" for none.
func (s *Server) anyConstellationLabels(name string) string {
	return s.anyConstellationLabel(constellationLabelsKey(name))
}

// setConstellationLabelsEverywhere writes the labels (or, with none, clears
// them) on every current member, of either kind.
func (s *Server) setConstellationLabelsEverywhere(name string, labels map[string]string) {
	v := encodeConstellationLabels(labels)
	s.setConstellationLabel("thing", name, constellationLabelsKey(name), v)
	s.setConstellationLabel("want", name, constellationLabelsKey(name), v)
}
