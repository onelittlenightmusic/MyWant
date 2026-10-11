package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// CaptureTarget is the one tile — a want, a thing, or a want's answer
// (`<want id>/result/<key>`) — whose constellation what gets captured goes
// into.
//
// The board names it (the Y slot's capture skill), making the constellation
// then, holding the tile alone, so the group has its name before anything is
// caught into it. Everything that takes something in from outside — the
// browser extension's copy offer, the phone's share sheet and clipboard offer
// — then captures with /things/share's `capture`: the thing is pinned AND
// added to this constellation.
//
// Kept here, by the server, rather than in any one page: an extension on
// another site and a phone aim at the same net as the board does.
type CaptureTarget struct {
	ID            string `json:"id"            yaml:"id"`
	Kind          string `json:"kind"          yaml:"kind"`
	Constellation string `json:"constellation" yaml:"constellation"`
	Label         string `json:"label"         yaml:"label"`
}

// captureTargetStore persists the target to ~/.mywant/capture-target.yaml.
// One target; none is no file (or an empty one).
type captureTargetStore struct {
	path string
	mu   sync.Mutex
}

var captureTargets = &captureTargetStore{}

func (c *captureTargetStore) file() string {
	if c.path != "" {
		return c.path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "capture-target.yaml"
	}
	return filepath.Join(home, ".mywant", "capture-target.yaml")
}

// Get answers the target, or nil when the net is down.
func (c *captureTargetStore) Get() *CaptureTarget {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := os.ReadFile(c.file())
	if err != nil {
		return nil
	}
	var t CaptureTarget
	if yaml.Unmarshal(data, &t) != nil || t.ID == "" || t.Constellation == "" {
		return nil
	}
	return &t
}

// Set aims the net, or puts it down (nil).
func (c *captureTargetStore) Set(t *CaptureTarget) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t == nil {
		if err := os.Remove(c.file()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.file()), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(c.file(), data, 0o644)
}

// GET /api/v1/capture-target → {target: {...} | null}
func (s *Server) getCaptureTarget(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]any{"target": captureTargets.Get()})
}

// PUT /api/v1/capture-target   body: {target: {id, kind?, constellation, label?} | null}
//
// The constellation is expected to exist already — the board makes it, with
// the tile in it, before aiming the net — and is not made here.
func (s *Server) putCaptureTarget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Target *CaptureTarget `json:"target"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	t := body.Target
	if t != nil {
		t.ID = strings.TrimSpace(t.ID)
		t.Constellation = strings.TrimSpace(t.Constellation)
		if t.ID == "" || t.Constellation == "" {
			s.JSONError(w, r, http.StatusBadRequest, "invalid capture target", "id and constellation are required")
			return
		}
		if err := validateConstellationName(t.Constellation); err != nil {
			s.JSONError(w, r, http.StatusBadRequest, "invalid constellation name", err.Error())
			return
		}
		if t.Kind != "want" {
			t.Kind = "thing"
		}
		if t.Label == "" {
			t.Label = t.ID
		}
	}
	if err := captureTargets.Set(t); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "could not save the capture target", err.Error())
		return
	}
	go broadcastSSE("capture_target_changed", t)
	s.JSONResponse(w, http.StatusOK, map[string]any{"target": t})
}
