package server

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"path/filepath"

	"github.com/gorilla/mux"
)

// A thing's first art: one file per thing, named by its id, served from the
// URL a thing's mywant.io/art label still holds. Kept so those pictures go on
// showing; art is kept by art.go now, for wants and constellations as well.
const thingArtURLPrefix = "/api/v1/things/art/"

// Named by the hash of the thing's id, so no id reaches outside the folder.
func thingArtPath(dir, id string) string {
	h := sha1.Sum([]byte(id))
	return filepath.Join(dir, hex.EncodeToString(h[:])+".png")
}

func (s *Server) thingKnown(id string) bool {
	for _, e := range s.thingStore.Entries() {
		if e.ID == id {
			return true
		}
	}
	return false
}

// GET /api/v1/things/art/{id}
func (s *Server) serveThingArt(w http.ResponseWriter, r *http.Request) {
	dir, err := s.artDir()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	serveArtPNG(w, r, thingArtPath(dir, mux.Vars(r)["id"]))
}
