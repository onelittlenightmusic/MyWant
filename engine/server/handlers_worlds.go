package server

import (
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

// WorldSummary is the list-view shape returned by GET /api/v1/worlds.
type WorldSummary struct {
	Name       string `json:"name"`
	WantCount  int    `json:"want_count"`
	ModifiedAt string `json:"modified_at"`
	Current    bool   `json:"current"`
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

		summaries = append(summaries, WorldSummary{
			Name:       name,
			WantCount:  count,
			ModifiedAt: modifiedAt,
			Current:    name == s.config.CurrentWorld,
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
		if len(wants) > 0 {
			wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(wants)
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
			// AddWantsAsyncWithTracking re-initializes each want, which can
			// reset state to its declared defaults — restore the state we
			// actually loaded from the snapshot (same fix-up importWants does).
			for _, want := range wants {
				if rw, _, found := s.globalBuilder.FindWantByID(want.Metadata.ID); found {
					mywant.StoreStateMulti(rw, want.GetAllState())
				}
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
