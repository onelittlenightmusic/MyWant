package server

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

// Art: a picture laid on the board's floor under something on it — a thing,
// a want, a constellation. Drawn on the phone (Image Playground) or chosen (a
// picture it already wears, a file), when asked for in its sidebar and kept
// once it has been looked at (the GUI's ArtMaker). "東京タワー" stands on a
// drawing of a tower.
//
// What has it says so with its URL: a thing and a want by a label of its own
// (mywant.io/art); a constellation, which has no record of its own, by a label
// every member carries (constellation-art/<name>), as its colour is carried.
// A drawing or a file is kept next to the config under a name of its own, so
// a rename moves nothing; a picture chosen from somewhere is kept as its URL.
//
//	GET    /api/v1/art/files/{file}     a kept picture
//	POST   /api/v1/art/{kind}/{id}      multipart image (PNG), or JSON {url}
//	POST   /api/v1/art/{kind}/{id}/scale  JSON {scale} — how large it lies
//	DELETE /api/v1/art/{kind}/{id}
//
// How large it lies on the floor rides on the URL itself, as its fragment
// (#scale=1.5): so it goes wherever the URL goes — onto every member of a
// constellation, to a new name — and asks nothing of whoever keeps it.
const (
	artLabel     = "mywant.io/art"
	maxArt       = 4 << 20
	artURLPrefix = "/api/v1/art/files/"
)

// Only names this file hands out: a-<hex>.png.
var artFileName = regexp.MustCompile(`^a-[0-9a-f]{16}\.png$`)

func (s *Server) artDir() (string, error) {
	// "thing-art": where the first of them were kept, before wants had any.
	dir := filepath.Join(filepath.Dir(s.config.ConfigPath), "thing-art")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// artOf is the art a thing, a want or a constellation has now, and whether it
// exists at all.
func (s *Server) artOf(kind, id string) (string, bool) {
	switch kind {
	case "thing":
		if !s.thingKnown(id) {
			return "", false
		}
		return s.thingLabels.Get(id)[artLabel], true
	case "want":
		if s.globalBuilder == nil {
			return "", false
		}
		want, _, found := s.globalBuilder.FindWantByID(id)
		if !found {
			return "", false
		}
		return want.GetLabels()[artLabel], true
	case "constellation":
		if len(s.allMembersOfConstellation(id)) == 0 {
			return "", false
		}
		return s.anyConstellationArt(id), true
	}
	return "", false
}

// setArt gives it art (url) or, with "", takes it away.
func (s *Server) setArt(kind, id, url string) error {
	switch kind {
	case "thing":
		defer boardThingsChanged(id)
		if url == "" {
			return s.thingLabels.Remove(id, artLabel)
		}
		return s.thingLabels.Set(id, artLabel, url)
	case "want":
		if url == "" {
			return s.globalBuilder.QueueWantRemoveLabel(id, artLabel)
		}
		return s.globalBuilder.QueueWantAddLabel(id, artLabel, url)
	case "constellation":
		s.setConstellationArtEverywhere(id, url)
		boardThingsChanged(s.membersOfConstellation("thing", id)...)
	}
	return nil
}

// removeArtFile deletes the file an art URL names, if it is one kept here —
// the shared ones and the first per-kind ones alike; a chosen picture's URL
// names nothing of ours.
func (s *Server) removeArtFile(url string) {
	dir, err := s.artDir()
	if err != nil {
		return
	}
	if i := strings.IndexAny(url, "?#"); i >= 0 {
		url = url[:i]
	}
	if file, ok := strings.CutPrefix(url, artURLPrefix); ok && artFileName.MatchString(file) {
		os.Remove(filepath.Join(dir, file))
		return
	}
	if file, ok := strings.CutPrefix(url, constellationArtURLPrefix); ok && constellationArtFile.MatchString(file) {
		os.Remove(filepath.Join(dir, file))
		return
	}
	if id, ok := strings.CutPrefix(url, thingArtURLPrefix); ok && id != "" && !strings.Contains(id, "/") {
		os.Remove(thingArtPath(dir, id))
	}
}

// GET /api/v1/art/files/{file}
func (s *Server) serveArtFile(w http.ResponseWriter, r *http.Request) {
	file := mux.Vars(r)["file"]
	dir, err := s.artDir()
	if err != nil || !artFileName.MatchString(file) {
		http.NotFound(w, r)
		return
	}
	serveArtPNG(w, r, filepath.Join(dir, file))
}

// serveArtPNG answers with a kept picture. A new drawing is a new file (or a
// new ?v=), so a copy can be kept for good.
func serveArtPNG(w http.ResponseWriter, r *http.Request, path string) {
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/png")
	http.ServeFile(w, r, path)
}

// POST /api/v1/art/{kind}/{id}   multipart: image (image/png), or JSON: {url}
func (s *Server) putArt(w http.ResponseWriter, r *http.Request) {
	kind, id := mux.Vars(r)["kind"], mux.Vars(r)["id"]
	old, ok := s.artOf(kind, id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "nothing of that kind and id", kind+"/"+id)
		return
	}
	var url string
	if given, isURL := s.artURLGiven(w, r); isURL {
		if given == "" {
			return
		}
		url = given
	} else {
		dir, err := s.artDir()
		if err != nil {
			s.JSONError(w, r, http.StatusInternalServerError, "Failed to access art directory", err.Error())
			return
		}
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		file := "a-" + hex.EncodeToString(b) + ".png"
		if !s.receiveArtPNG(w, r, filepath.Join(dir, file)) {
			return
		}
		url = artURLPrefix + file
	}
	// A new picture lies as large as the last was set to.
	url = withArtScale(url, artScaleOf(old))
	if err := s.setArt(kind, id, url); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to keep art", err.Error())
		return
	}
	if withArtScale(old, 1) != withArtScale(url, 1) {
		s.removeArtFile(old)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"url": url})
}

// How large art lies, as the fragment of its URL says: from a small print to a
// wide one, 1 the floor's own size.
const (
	minArtScale = 0.4
	maxArtScale = 3
)

// withArtScale is url with its fragment saying scale — none at all for 1.
func withArtScale(url string, scale float64) string {
	if i := strings.IndexByte(url, '#'); i >= 0 {
		url = url[:i]
	}
	if scale == 1 {
		return url
	}
	return url + "#scale=" + strconv.FormatFloat(scale, 'f', -1, 64)
}

// artScaleOf is the scale an art URL's fragment says, 1 when it says none.
func artScaleOf(url string) float64 {
	_, frag, ok := strings.Cut(url, "#scale=")
	if !ok {
		return 1
	}
	v, err := strconv.ParseFloat(frag, 64)
	if err != nil || v < minArtScale || v > maxArtScale {
		return 1
	}
	return v
}

// POST /api/v1/art/{kind}/{id}/scale   JSON: {scale}
func (s *Server) scaleArt(w http.ResponseWriter, r *http.Request) {
	kind, id := mux.Vars(r)["kind"], mux.Vars(r)["id"]
	old, ok := s.artOf(kind, id)
	if !ok || old == "" {
		s.JSONError(w, r, http.StatusNotFound, "no art there", kind+"/"+id)
		return
	}
	var body struct {
		Scale float64 `json:"scale"`
	}
	if err := DecodeRequest(r, &body); err != nil || body.Scale < minArtScale || body.Scale > maxArtScale {
		s.JSONError(w, r, http.StatusBadRequest, "scale must be between 0.4 and 3", "")
		return
	}
	url := withArtScale(old, body.Scale)
	if err := s.setArt(kind, id, url); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to keep art", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"url": url})
}

// DELETE /api/v1/art/{kind}/{id}
func (s *Server) deleteArt(w http.ResponseWriter, r *http.Request) {
	kind, id := mux.Vars(r)["kind"], mux.Vars(r)["id"]
	old, ok := s.artOf(kind, id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "nothing of that kind and id", kind+"/"+id)
		return
	}
	if err := s.setArt(kind, id, ""); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to take art away", err.Error())
		return
	}
	s.removeArtFile(old)
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "removed"})
}

// receiveArtPNG keeps the request's multipart "image" (a PNG) at dest — written
// aside and renamed, so a reader never gets half a PNG — or answers the error
// and says false.
func (s *Server) receiveArtPNG(w http.ResponseWriter, r *http.Request, dest string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxArt)
	if err := r.ParseMultipartForm(maxArt); err != nil {
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
