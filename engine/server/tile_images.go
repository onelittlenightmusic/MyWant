package server

// Want tiles as pictures: a want's tile as the board draws it, kept as a PNG
// for whatever cannot draw the board itself — a notification's picture on a
// phone or in the browser, a widget. The server does not draw it: whoever has
// a browser does (the GUI's tile page made into a picture, ?png=1 — the app,
// the browser extension) and hands it in; the latest is kept, one a want.
//
//	PUT /api/v1/wants/{id}/tile.png   (a PNG, up to tileImageMax)
//	GET /api/v1/wants/{id}/tile.png   → the PNG, or 404 when none was handed in

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gorilla/mux"
)

const tileImageMax = 2 << 20 // 2 MiB

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// safeWantID keeps a want id to what can name a file.
var safeWantID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func tileImagesDir() string { return initiativeHomeFile("tiles") }

func tileImagePath(id string) (string, bool) {
	if !safeWantID.MatchString(id) {
		return "", false
	}
	return filepath.Join(tileImagesDir(), id+".png"), true
}

// putTileImage handles PUT /api/v1/wants/{id}/tile.png.
func (s *Server) putTileImage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.handleOptions(w, r)
		return
	}
	id := mux.Vars(r)["id"]
	path, ok := tileImagePath(id)
	if !ok || s.findWantByIDInAll(id) == nil {
		s.JSONError(w, r, http.StatusNotFound, "want not found", id)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, tileImageMax+1))
	if err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "could not read the picture", err.Error())
		return
	}
	if len(data) > tileImageMax || !bytes.HasPrefix(data, pngMagic) {
		s.JSONError(w, r, http.StatusBadRequest, "want a PNG up to 2 MiB", "")
		return
	}
	if err := os.MkdirAll(tileImagesDir(), 0o755); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "could not keep the picture", err.Error())
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "could not keep the picture", err.Error())
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "could not keep the picture", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"wantId": id, "bytes": len(data), "at": time.Now().Format(time.RFC3339)})
}

// getTileImage handles GET /api/v1/wants/{id}/tile.png.
func (s *Server) getTileImage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.handleOptions(w, r)
		return
	}
	path, ok := tileImagePath(mux.Vars(r)["id"])
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	// Asked again each time: the tile changes with the want's answer.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "tile.png", info.ModTime(), f)
}
