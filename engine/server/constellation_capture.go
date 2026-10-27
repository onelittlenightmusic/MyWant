package server

import "strings"

// A constellation is one of two sorts. An ORDER — what a wire on the board ties
// together, one after another — is drawn as a chain through its members. A
// CAPTURE — what the net (capture_target.go) caught into the tile it is aimed
// at — is drawn from that tile out to each of them: what was caught, and what
// it was caught for. The capture's tile rides on every member, as the colour
// does ("constellation-capture/<name>" = the tile's id), joining and leaving
// with them and carried to a new name on a rename; a constellation with none
// is an order.
const constellationCapturePrefix = "constellation-capture/"

func constellationCaptureKey(name string) string { return constellationCapturePrefix + name }

func constellationCaptureNameFromKey(key string) string {
	if name, ok := strings.CutPrefix(key, constellationCapturePrefix); ok {
		return name
	}
	return ""
}

// anyConstellationCapture is the tile a capture constellation was caught
// into, or "" for an order.
func (s *Server) anyConstellationCapture(name string) string {
	if t := s.constellationLabel("thing", constellationCaptureKey(name)); t != "" {
		return t
	}
	return s.constellationLabel("want", constellationCaptureKey(name))
}

// setConstellationCaptureEverywhere makes the constellation a capture into
// tile (or, with "", an order again) on every current member.
func (s *Server) setConstellationCaptureEverywhere(name, tile string) {
	s.setConstellationLabel("thing", name, constellationCaptureKey(name), tile)
	s.setConstellationLabel("want", name, constellationCaptureKey(name), tile)
}

// captureInto adds a caught thing to the constellation of the tile it was
// caught into, which is a capture into that tile from then on.
func (s *Server) captureInto(name, tile, thingID string) {
	s.setConstellationMembership("thing", name, thingID, true)
	if tile != "" {
		s.setConstellationCaptureEverywhere(name, tile)
	}
}

// Where a capture's hub was last seen ("x,y" in board cells) — so its ring and
// lines stay on the board after the hub itself is gone (an enemy fallen). The
// board writes it as the hub leaves; it rides on the members as the capture
// does.
const constellationCaptureAtPrefix = "constellation-capture-at/"

func constellationCaptureAtKey(name string) string { return constellationCaptureAtPrefix + name }

func constellationCaptureAtNameFromKey(key string) string {
	if name, ok := strings.CutPrefix(key, constellationCaptureAtPrefix); ok {
		return name
	}
	return ""
}

func (s *Server) anyConstellationCaptureAt(name string) string {
	if t := s.constellationLabel("thing", constellationCaptureAtKey(name)); t != "" {
		return t
	}
	return s.constellationLabel("want", constellationCaptureAtKey(name))
}

func (s *Server) setConstellationCaptureAtEverywhere(name, at string) {
	s.setConstellationLabel("thing", name, constellationCaptureAtKey(name), at)
	s.setConstellationLabel("want", name, constellationCaptureAtKey(name), at)
}
