package server

import (
	"os"
	"path/filepath"
	"strings"

	mywant "mywant/engine/core"

	"gopkg.in/yaml.v3"
)

// ── Where you were standing belongs to the world ─────────────────────────────
//
// A world remembers its wants and its things, and used to forget the one thing
// a person actually notices: where they were standing on it. Reopening a world
// put the character back at the middle of the board, which is the one place it
// certainly was not.
//
// That position lives in the gui_state want, together with everything else the
// GUI keeps there, so this saves the lot. Restoring a whole world's worth of
// GUI state is coarser than restoring a position — the sidebar and the search
// box come back too — but it is the shape asked for, and the alternative (a
// hand-maintained list of "position-ish" keys) drifts out of date the moment
// somebody adds a key and does not think of this file.
//
// It is a file of its own, beside the things and for the same reason: the
// worlds directory is enumerated by scanning *.yaml, so a sibling file would
// list itself as a world.

// guiStateVolatileKeys are the gui_state entries this refuses to carry.
//
// They are not GUI state at all — they are the engine's own bookkeeping on the
// want that happens to hold the GUI's state, present on every want there is.
// Writing them back from a world file would tell the gui_state want it had
// already achieved something, or hand it a stale "who wrote this last", which
// is a way to break the want rather than to restore a board.
var guiStateVolatileKeys = map[string]bool{
	"achieved":             true,
	"achieving_percentage": true,
	"completed":            true,
	"final_result":         true,
	"action_by_agent":      true,
	"source":               true,
}

// worldGUIStateDir returns <worldsDir>/gui, where every world's GUI state lives.
func worldGUIStateDir(dir string) string {
	return filepath.Join(dir, "gui")
}

// worldGUIStatePath returns <worldsDir>/gui/<name>.yaml.
func worldGUIStatePath(dir, name string) string {
	return filepath.Join(worldGUIStateDir(dir), name+".yaml")
}

// exportableGUIState is the gui_state want's state minus the engine bookkeeping
// above — everything the GUI itself put there, and nothing else.
func (s *Server) exportableGUIState() map[string]any {
	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil {
		return nil
	}
	// Deep, because this gets marshalled: a shallow copy would hand the YAML
	// encoder nested maps the GUI is still writing to. See GetAllStateDeep.
	out := map[string]any{}
	for k, v := range want.GetAllStateDeep() {
		if guiStateVolatileKeys[k] {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// persistGUIStateToOpenWorld writes the GUI state into whichever world is open,
// used when something other than a world switch needs it on disk.
//
// This is deliberately not called on every change. It marshals a snapshot of
// the want's state, and doing that on a hot path is how you turn a rare data
// race into a frequent one — the reconcile loop already marshals every want,
// and a second frequent serializer racing the same values took the server down
// once (see GetAllStateDeep). A world's GUI state is written when the world is
// left, which is the only moment it has to be right.
func (s *Server) persistGUIStateToOpenWorld() {
	dir, err := s.worldsDir()
	if err != nil {
		return
	}
	name := s.config.CurrentWorld
	if name == "" {
		name = defaultWorldName
	}
	if !safeWorldName(name) {
		return
	}
	_ = s.saveWorldGUIState(dir, name)
}

// saveWorldGUIState writes the open GUI state into the named world.
//
// A world with nothing to say about the GUI writes no file rather than an empty
// one, so "this world has never been left" stays distinguishable from "this
// world was left with everything closed".
func (s *Server) saveWorldGUIState(dir, name string) error {
	state := s.exportableGUIState()
	if state == nil {
		return nil
	}

	// Merge over what the world already remembers, rather than replacing it.
	//
	// The cursor keys carry two meanings at once: where a character is, and
	// where a character was. When one steps off the board the client clears
	// them — correct as presence ("do not draw me"), and destructive as memory:
	// the world was then saved with no position at all, and coming back put the
	// character at the spawn point every time. Absence here is the character
	// not being on the board, not an instruction to forget where they stood.
	if prior := readWorldGUIState(dir, name); prior != nil {
		merged := make(map[string]any, len(prior)+len(state))
		for k, v := range prior {
			merged[k] = v
		}
		for k, v := range state {
			merged[k] = v
		}
		state = merged
	}
	if err := os.MkdirAll(worldGUIStateDir(dir), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(worldGUIStatePath(dir, name), data, 0644)
}

// readWorldGUIState reads a world's saved GUI state, or nil when it has none.
// A world saved before this existed, or a hand-written one, simply has none.
func readWorldGUIState(dir, name string) map[string]any {
	data, err := os.ReadFile(worldGUIStatePath(dir, name))
	if err != nil {
		return nil
	}
	var state map[string]any
	if err := yaml.Unmarshal(data, &state); err != nil || len(state) == 0 {
		return nil
	}
	return state
}

// applyGUIState merges a world's saved GUI state onto the live gui_state want.
//
// Merge, not replace: a key the incoming world says nothing about keeps the
// value it has. That is what makes opening an older world — or one written by
// hand — leave the character where it is instead of teleporting it to the
// middle of a board that never recorded a position for it. Silence is not an
// instruction.
// Returns whether it restored a cursor position for anyone — what the world
// switch reports back, so the client knows whether the board already says where
// to stand or whether it should fall back to the world's spawn point.
func (s *Server) applyGUIState(state map[string]any) bool {
	if len(state) == 0 {
		return false
	}
	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil {
		return false
	}
	restoredCursor := false
	for k, v := range state {
		if guiStateVolatileKeys[k] {
			continue
		}
		if strings.HasPrefix(k, "canvas_cursor_x") || strings.HasPrefix(k, "canvas_cursor_y") {
			restoredCursor = true
		}
		want.StoreState(k, v)
		// Register the key the same way PUT /gui/state does. Most gui_state
		// keys are per-character (canvas_cursor_x_<id>) and so cannot be in the
		// static schema; a key that is not declared here is stored but dropped
		// from GET, which reads exactly like the restore never happened.
		if !mywant.Contains(want.ProvidedStateFields, k) {
			want.ProvidedStateFields = append(want.ProvidedStateFields, k)
		}
	}
	// Same bump-and-say every PUT /gui/state makes. Without the broadcast a tab
	// keeps showing the world it was looking at until its next poll, and
	// without the bump it would not even re-read then.
	resp := guiStateResponse{Seq: nextGUIStateSeq(), State: s.guiStateWithConfig(want)}
	go broadcastSSE("gui_state", resp)
	return restoredCursor
}

// copyWorldGUIState copies one world's saved GUI state to another name, for
// "save as" — the copy is the same board, so it opens the same way.
func copyWorldGUIState(dir, from, to string) {
	state := readWorldGUIState(dir, from)
	if state == nil {
		return
	}
	if err := os.MkdirAll(worldGUIStateDir(dir), 0755); err != nil {
		return
	}
	if data, err := yaml.Marshal(state); err == nil {
		_ = os.WriteFile(worldGUIStatePath(dir, to), data, 0644)
	}
}
