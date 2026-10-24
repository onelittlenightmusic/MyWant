package server

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// A thing's art: a small picture drawn for it — on the phone, by Image
// Playground, when asked for in the thing's sidebar and kept once it has been
// looked at (the GUI's ThingArtSection) — that the board shows on its ball and
// its tag when the thing has no picture of its own (a photo, a page's
// screenshot). "東京タワー" wears a drawing of a tower rather than only a
// place's pin.
//
// The picture is a file next to the config, one per thing; the thing says it
// has one by a label holding its URL, with the time it was drawn so a new
// drawing is fetched rather than cached.
const (
	thingArtLabel = "mywant.io/art"
	maxThingArt   = 4 << 20
)

func (s *Server) thingArtDir() (string, error) {
	dir := filepath.Join(filepath.Dir(s.config.ConfigPath), "thing-art")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

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
	dir, err := s.thingArtDir()
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to access art directory", err.Error())
		return
	}
	path := thingArtPath(dir, mux.Vars(r)["id"])
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	// The URL carries the time it was drawn (?v=), so a copy can be kept.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/png")
	http.ServeFile(w, r, path)
}

// POST /api/v1/things/art/{id}   multipart: image (image/png)
//                                 or JSON: {url} — a picture it already has
func (s *Server) uploadThingArt(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !s.thingKnown(id) {
		s.JSONError(w, r, http.StatusNotFound, "no such thing", id)
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
		// Not drawn: a drawing kept before goes.
		os.Remove(thingArtPath(dir, id))
		url = given
	} else {
		if !s.receiveArtPNG(w, r, thingArtPath(dir, id)) {
			return
		}
		url = "/api/v1/things/art/" + id + "?v=" + strconv.FormatInt(time.Now().UnixMilli(), 36)
	}
	if err := s.thingLabels.Set(id, thingArtLabel, url); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to set art label", err.Error())
		return
	}
	boardThingsChanged(id)
	s.JSONResponse(w, http.StatusOK, map[string]any{"url": url})
}

// DELETE /api/v1/things/art/{id} — the drawing taken away.
func (s *Server) deleteThingArt(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if dir, err := s.thingArtDir(); err == nil {
		os.Remove(thingArtPath(dir, id))
	}
	_ = s.thingLabels.Remove(id, thingArtLabel)
	boardThingsChanged(id)
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "removed"})
}

// receiveArtPNG keeps the request's multipart "image" (a PNG) at dest — written
// aside and renamed, so a reader never gets half a PNG — or answers the error
// and says false.
func (s *Server) receiveArtPNG(w http.ResponseWriter, r *http.Request, dest string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxThingArt)
	if err := r.ParseMultipartForm(maxThingArt); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "File too large or bad multipart", err.Error())
		return false
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Missing 'image' field", err.Error())
		return false
	}
	defer file.Close()
	if ct := header.Header.Get("Content-Type"); ct != "" && ct != "image/png" {
		s.JSONError(w, r, http.StatusBadRequest, "Art must be image/png", ct)
		return false
	}
	tmp := dest + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to create file", err.Error())
		return false
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		os.Remove(tmp)
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to save file", err.Error())
		return false
	}
	out.Close()
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to save file", err.Error())
		return false
	}
	return true
}

// artURLGiven reads a JSON body {url} — art that is a picture already
// somewhere, chosen rather than drawn: whether the body is one, and its URL,
// or "" with the error answered. Only a page's own path or http(s).
func (s *Server) artURLGiven(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return "", false
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return "", true
	}
	u := strings.TrimSpace(body.URL)
	if !(strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//")) && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		s.JSONError(w, r, http.StatusBadRequest, "url must be a path or http(s)", u)
		return "", true
	}
	return u, true
}
