package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
)

// A constellation's art: a picture drawn for it, as for a thing (thing_art.go),
// laid on the board's floor under the group. There is no constellation ledger
// to hang it on, so its URL rides on every member as its colour does
// ("constellation-art/<name>"), joining and leaving with them and carried to a
// new name on a rename.
//
// The file is named by a random id rather than by the constellation's name, so
// a rename does not have to move it; a new drawing replaces the old file.
const constellationArtPrefix = "constellation-art/"

func constellationArtKey(name string) string { return constellationArtPrefix + name }

func constellationArtNameFromKey(key string) string {
	if name, ok := strings.CutPrefix(key, constellationArtPrefix); ok {
		return name
	}
	return ""
}

const constellationArtURLPrefix = "/api/v1/constellations/art/"

// Only names this file hands out: c-<hex>.png.
var constellationArtFile = regexp.MustCompile(`^c-[0-9a-f]{16}\.png$`)

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

// removeConstellationArtFile deletes the file an art URL names, if it is one
// of ours.
func (s *Server) removeConstellationArtFile(url string) {
	file, ok := strings.CutPrefix(url, constellationArtURLPrefix)
	if !ok {
		return
	}
	if i := strings.IndexByte(file, '?'); i >= 0 {
		file = file[:i]
	}
	if !constellationArtFile.MatchString(file) {
		return
	}
	if dir, err := s.thingArtDir(); err == nil {
		os.Remove(filepath.Join(dir, file))
	}
}

// GET /api/v1/constellations/art/{file}
func (s *Server) serveConstellationArt(w http.ResponseWriter, r *http.Request) {
	file := mux.Vars(r)["file"]
	dir, err := s.thingArtDir()
	if err != nil || !constellationArtFile.MatchString(file) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(dir, file)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	// A new drawing is a new file, so a copy can be kept.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/png")
	http.ServeFile(w, r, path)
}

// POST /api/v1/constellations/{name}/art   multipart: image (image/png)
//                                           or JSON: {url} — a picture already somewhere
func (s *Server) uploadConstellationArt(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if len(s.allMembersOfConstellation(name)) == 0 {
		s.JSONError(w, r, http.StatusNotFound, "no such constellation", name)
		return
	}
	dir, err := s.thingArtDir()
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to access art directory", err.Error())
		return
	}
	var url string
	if given, ok := s.artURLGiven(w, r); ok {
		if given == "" {
			return
		}
		url = given
	} else {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		file := "c-" + hex.EncodeToString(b) + ".png"
		if !s.receiveArtPNG(w, r, filepath.Join(dir, file)) {
			return
		}
		url = constellationArtURLPrefix + file
	}
	old := s.anyConstellationArt(name)
	s.setConstellationArtEverywhere(name, url)
	if old != url {
		s.removeConstellationArtFile(old)
	}
	boardThingsChanged(s.membersOfConstellation("thing", name)...)
	s.JSONResponse(w, http.StatusOK, map[string]any{"url": url})
}

// DELETE /api/v1/constellations/{name}/art
func (s *Server) deleteConstellationArt(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	s.removeConstellationArtFile(s.anyConstellationArt(name))
	s.setConstellationArtEverywhere(name, "")
	boardThingsChanged(s.membersOfConstellation("thing", name)...)
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "removed"})
}
