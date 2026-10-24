package server

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
)

// A constellation's art (art.go): there is no constellation ledger to hang it
// on, so its URL rides on every member as its colour does
// ("constellation-art/<name>"), joining and leaving with them and carried to
// a new name on a rename (handlers_constellations.go).
const constellationArtPrefix = "constellation-art/"

func constellationArtKey(name string) string { return constellationArtPrefix + name }

func constellationArtNameFromKey(key string) string {
	if name, ok := strings.CutPrefix(key, constellationArtPrefix); ok {
		return name
	}
	return ""
}

func (s *Server) anyConstellationArt(name string) string {
	if a := s.constellationLabel("thing", constellationArtKey(name)); a != "" {
		return a
	}
	return s.constellationLabel("want", constellationArtKey(name))
}

func (s *Server) setConstellationArtEverywhere(name, url string) {
	s.setConstellationLabel("thing", name, constellationArtKey(name), url)
	s.setConstellationLabel("want", name, constellationArtKey(name), url)
}

// The first constellation art's files, c-<hex>.png, served from the URL its
// members' labels still hold.
const constellationArtURLPrefix = "/api/v1/constellations/art/"

var constellationArtFile = regexp.MustCompile(`^c-[0-9a-f]{16}\.png$`)

// GET /api/v1/constellations/art/{file}
func (s *Server) serveConstellationArt(w http.ResponseWriter, r *http.Request) {
	file := mux.Vars(r)["file"]
	dir, err := s.artDir()
	if err != nil || !constellationArtFile.MatchString(file) {
		http.NotFound(w, r)
		return
	}
	serveArtPNG(w, r, filepath.Join(dir, file))
}
