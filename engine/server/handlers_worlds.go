package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

// worldsDir returns ~/.mywant/worlds (a sibling of config.yaml), creating it
// if it doesn't exist yet.
func (s *Server) worldsDir() (string, error) {
	dir := filepath.Join(filepath.Dir(s.config.ConfigPath), "worlds")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func worldFilePath(dir, name string) string {
	return filepath.Join(dir, name+".yaml")
}

// worldThumbPath returns <worldsDir>/thumbs/<name>.png — the canvas screenshot
// the GUI uploads periodically for the world that is currently open. The
// subdirectory keeps them out of listWorlds' *.yaml scan.
func worldThumbPath(dir, name string) string {
	return filepath.Join(dir, "thumbs", name+".png")
}

// safeWorldName rejects names that would escape the worlds directory.
func safeWorldName(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\`) && name != "." && name != ".."
}

// WorldSummary is the list-view shape returned by GET /api/v1/worlds.
type WorldSummary struct {
	Name       string `json:"name"`
	WantCount  int    `json:"want_count"`
	ModifiedAt string `json:"modified_at"`
	Current    bool   `json:"current"`
	// ThumbnailAt is the mtime (RFC3339) of the world's canvas screenshot, or ""
	// when none has been captured yet. Doubles as a cache-buster for the
	// GET /api/v1/worlds/{name}/thumbnail URL.
	ThumbnailAt string `json:"thumbnail_at,omitempty"`
}

func (s *Server) listWorlds(w http.ResponseWriter, r *http.Request) {
	dir, err := s.worldsDir()
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to access worlds directory", err.Error())
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to list worlds", err.Error())
		return
	}

	summaries := make([]WorldSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		path := filepath.Join(dir, entry.Name())

		modifiedAt := ""
		if info, err := entry.Info(); err == nil {
			modifiedAt = info.ModTime().Format(time.RFC3339)
		}

		count := 0
		if wants, err := mywant.LoadWantsFromMemoryFile(path); err == nil {
			count = len(wants)
		}

		thumbnailAt := ""
		if info, err := os.Stat(worldThumbPath(dir, name)); err == nil {
			thumbnailAt = info.ModTime().Format(time.RFC3339)
		}

		summaries = append(summaries, WorldSummary{
			Name:        name,
			WantCount:   count,
			ModifiedAt:  modifiedAt,
			Current:     name == s.config.CurrentWorld,
			ThumbnailAt: thumbnailAt,
		})
	}

	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })

	s.JSONResponse(w, http.StatusOK, summaries)
}

// saveWorldSnapshot writes every currently-running non-system want to
// <worldsDir>/<name>.yaml, overwriting any existing snapshot of that name.
func (s *Server) saveWorldSnapshot(name string) error {
	dir, err := s.worldsDir()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(s.exportableWants(false))
	if err != nil {
		return err
	}
	return os.WriteFile(worldFilePath(dir, name), data, 0644)
}

// saveWorld handles POST /api/v1/worlds/{name}/save — snapshots the currently
// running non-system wants to <name>.yaml immediately, without switching
// worlds or touching Config.CurrentWorld. Useful right after bulk-importing
// wants into the active world (e.g. a stage-conversion tool) so the result is
// persisted without waiting for the next world switch.
func (s *Server) saveWorld(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if name == "" {
		s.JSONError(w, r, http.StatusBadRequest, "World name is required", "")
		return
	}
	if err := s.saveWorldSnapshot(name); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to save world", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"name": name})
}

// exportWorld handles GET /api/v1/worlds/{name}/export — downloads a world's
// snapshot as YAML. The currently-open world is snapshotted first so the
// download reflects the live wants rather than the last switch.
func (s *Server) exportWorld(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if !safeWorldName(name) {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid world name", name)
		return
	}

	if name == s.config.CurrentWorld {
		if err := s.saveWorldSnapshot(name); err != nil {
			s.JSONError(w, r, http.StatusInternalServerError, "Failed to snapshot current world", err.Error())
			return
		}
	}

	dir, err := s.worldsDir()
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to access worlds directory", err.Error())
		return
	}

	data, err := os.ReadFile(worldFilePath(dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			s.JSONError(w, r, http.StatusNotFound, "World not found", name)
			return
		}
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to read world", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+".yaml\"")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// importWorld handles POST /api/v1/worlds/{name}/import — stores an uploaded
// wants YAML as <name>.yaml, creating a new world without opening it. Refuses
// to clobber an existing world unless ?overwrite=true is passed.
func (s *Server) importWorld(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if !safeWorldName(name) {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid world name", name)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Failed to read request body", err.Error())
		return
	}

	// Validate before writing so a malformed upload can't create a broken world.
	var wants []*mywant.Want
	if err := yaml.Unmarshal(data, &wants); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid wants YAML", err.Error())
		return
	}
	for _, want := range wants {
		if want == nil || want.Metadata.ID == "" {
			s.JSONError(w, r, http.StatusBadRequest, "Imported wants must have IDs", "")
			return
		}
	}

	dir, err := s.worldsDir()
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to access worlds directory", err.Error())
		return
	}

	path := worldFilePath(dir, name)
	overwrite := strings.ToLower(r.URL.Query().Get("overwrite")) == "true"
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			s.JSONError(w, r, http.StatusConflict, "World already exists", name)
			return
		}
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to write world", err.Error())
		return
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"name":       name,
		"want_count": len(wants),
	})
}

// clearNonSystemWants deletes every currently-running non-system want and
// waits (bounded, ~2s) for the deletions to take effect before returning, so
// a subsequent world-load doesn't race with wants still being torn down.
func (s *Server) clearNonSystemWants() error {
	if s.globalBuilder == nil {
		return nil
	}
	current := s.globalBuilder.GetAllWantStates()
	ids := make([]string, 0, len(current))
	for _, want := range current {
		if !want.Metadata.IsSystemWant {
			ids = append(ids, want.Metadata.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if err := s.globalBuilder.QueueWantDelete(ids); err != nil {
		return err
	}
	for i := 0; i < 100; i++ {
		stillPresent := false
		for _, id := range ids {
			if _, _, found := s.globalBuilder.FindWantByID(id); found {
				stillPresent = true
				break
			}
		}
		if !stillPresent {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

// openWorld switches the running server to a named world snapshot:
//  1. auto-saves the currently-open world (or "default" if none is open yet,
//     so a first-time switch never silently discards existing wants)
//  2. clears all current non-system wants
//  3. loads <name>.yaml if it exists (a brand-new name just starts empty)
//  4. records <name> as the current world (persisted to config.yaml)
func (s *Server) openWorld(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if name == "" {
		s.JSONError(w, r, http.StatusBadRequest, "World name is required", "")
		return
	}
	if s.globalBuilder == nil {
		s.JSONError(w, r, http.StatusServiceUnavailable, "Server not ready", "")
		return
	}

	saveAs := s.config.CurrentWorld
	if saveAs == "" {
		saveAs = "default"
	}
	if err := s.saveWorldSnapshot(saveAs); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to auto-save current world", err.Error())
		return
	}

	if err := s.clearNonSystemWants(); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to clear current wants", err.Error())
		return
	}

	dir, err := s.worldsDir()
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to access worlds directory", err.Error())
		return
	}

	loadedCount := 0
	targetPath := worldFilePath(dir, name)
	if _, statErr := os.Stat(targetPath); statErr == nil {
		wants, err := mywant.LoadWantsFromMemoryFile(targetPath)
		if err != nil {
			s.JSONError(w, r, http.StatusInternalServerError, "Failed to load world", err.Error())
			return
		}

		// Snapshots can contain "always visible" system wants (e.g. robot) that
		// clearNonSystemWants never tore down, since they're long-lived
		// infrastructure. Re-adding them via AddWantsAsyncWithTracking would
		// collide with the still-running instance, so update those in place
		// instead of adding them as new wants.
		newWants := make([]*mywant.Want, 0, len(wants))
		existingWants := make([]*mywant.Want, 0)
		for _, want := range wants {
			if _, _, found := s.globalBuilder.FindWantByID(want.Metadata.ID); found {
				existingWants = append(existingWants, want)
			} else {
				newWants = append(newWants, want)
			}
		}

		for _, want := range existingWants {
			s.globalBuilder.UpdateWant(want)
		}

		if len(newWants) > 0 {
			wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(newWants)
			if err != nil {
				s.JSONError(w, r, http.StatusInternalServerError, "Failed to load world wants", err.Error())
				return
			}
			for i := 0; i < 100; i++ {
				if s.globalBuilder.AreWantsAdded(wantIDs) {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
		// AddWantsAsyncWithTracking re-initializes each want, which can
		// reset state to its declared defaults — restore the state we
		// actually loaded from the snapshot (same fix-up importWants does).
		for _, want := range wants {
			if rw, _, found := s.globalBuilder.FindWantByID(want.Metadata.ID); found {
				mywant.StoreStateMulti(rw, want.GetAllState())
			}
		}
		loadedCount = len(wants)
	}

	s.config.CurrentWorld = name
	s.saveFrontendConfig()

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"name":       name,
		"want_count": loadedCount,
	})
}

// uploadWorldThumbnail handles POST /api/v1/worlds/{name}/thumbnail — stores a
// PNG screenshot of the want canvas for that world. The GUI captures one
// periodically while a world is open, so the worlds page can show each world by
// what its canvas looks like. Body is multipart/form-data with an "image" field.
func (s *Server) uploadWorldThumbnail(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if !safeWorldName(name) {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid world name", name)
		return
	}

	const maxSize = 8 << 20 // 8 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	if err := r.ParseMultipartForm(maxSize); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "File too large or bad multipart", err.Error())
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Missing 'image' field", err.Error())
		return
	}
	defer file.Close()

	if ct := header.Header.Get("Content-Type"); ct != "" && ct != "image/png" {
		s.JSONError(w, r, http.StatusBadRequest, "Thumbnail must be image/png", ct)
		return
	}

	dir, err := s.worldsDir()
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to access worlds directory", err.Error())
		return
	}
	destPath := worldThumbPath(dir, name)
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to create thumbs directory", err.Error())
		return
	}

	// Write to a temp file and rename so a reader never sees a half-written PNG.
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to create file", err.Error())
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		os.Remove(tmpPath)
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to save file", err.Error())
		return
	}
	out.Close()
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to store thumbnail", err.Error())
		return
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"name":         name,
		"thumbnail_at": time.Now().Format(time.RFC3339),
	})
}

// serveWorldThumbnail handles GET /api/v1/worlds/{name}/thumbnail.
func (s *Server) serveWorldThumbnail(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if !safeWorldName(name) {
		http.NotFound(w, r)
		return
	}
	dir, err := s.worldsDir()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	path := worldThumbPath(dir, name)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	// Callers append ?t=<thumbnail_at>, so a long cache is safe.
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	http.ServeFile(w, r, path)
}
