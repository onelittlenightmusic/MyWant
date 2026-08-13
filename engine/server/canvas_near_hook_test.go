package server

import (
	"strconv"
	"testing"
	"time"

	mywant "mywant/engine/core"
)

// A deployment file cannot know where anyone is standing, so it names who
// instead. These tests are the whole contract of that: who gets asked, in what
// order, and where the want lands relative to them.

func guiStateWant(kv map[string]any) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: guiStateWantID, Type: "gui_state"}}
	for k, v := range kv {
		w.StoreState(k, v)
	}
	return w
}

func tileAt(id string, x, y int) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: id, Type: "wall"}}
	w.SetLabel(canvasLabelX, strconv.Itoa(x))
	w.SetLabel(canvasLabelY, strconv.Itoa(y))
	return w
}

func nearWant(target string) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: "new-1", Type: "rpg_try_keys"}}
	w.SetLabel(canvasLabelNear, target)
	return w
}

func cellOf(t *testing.T, w *mywant.Want) (string, string) {
	t.Helper()
	return w.GetLabel(canvasLabelX), w.GetLabel(canvasLabelY)
}

// Beside, never underneath: a want on the exact cell a player stands on reads
// as the board swallowing them.
func TestCanvasNearPlacesBesideNotUnder(t *testing.T) {
	setLiveCursor(t, "chr-hero", 25, 64)

	w := nearWant("chr-hero")
	if err := (&CanvasNearHook{}).Run(w, nil, []*mywant.Want{w}); err != nil {
		t.Fatalf("hook returned %v", err)
	}
	x, y := cellOf(t, w)
	if x == "" || y == "" {
		t.Fatalf("no coordinate assigned (x=%q y=%q)", x, y)
	}
	if x == "25" && y == "64" {
		t.Error("placed on the character's own cell; expected an adjacent one")
	}
	if !within(t, x, y, 25, 64, 1) {
		t.Errorf("placed at (%s,%s), which is not adjacent to (25,64)", x, y)
	}
}

// A tab open beats a remembered position: the live cursor is the character
// reporting where they are now, gui_state is where they were left.
func TestCanvasNearPrefersLiveCursorOverGUIState(t *testing.T) {
	setLiveCursor(t, "chr-hero", 25, 64)
	all := []*mywant.Want{guiStateWant(map[string]any{
		"canvas_cursor_x_chr-hero": 3,
		"canvas_cursor_y_chr-hero": 3,
	})}

	w := nearWant("chr-hero")
	if err := (&CanvasNearHook{}).Run(w, all, []*mywant.Want{w}); err != nil {
		t.Fatalf("hook returned %v", err)
	}
	x, y := cellOf(t, w)
	if !within(t, x, y, 25, 64, 1) {
		t.Errorf("placed at (%s,%s); expected next to the live cursor at (25,64)", x, y)
	}
}

// Nobody playing them: fall back to where gui_state left them.
func TestCanvasNearFallsBackToGUIState(t *testing.T) {
	clearCursors(t)
	all := []*mywant.Want{guiStateWant(map[string]any{
		"canvas_cursor_x_chr-away": 7.0,
		"canvas_cursor_y_chr-away": 9.0,
	})}

	w := nearWant("chr-away")
	if err := (&CanvasNearHook{}).Run(w, all, []*mywant.Want{w}); err != nil {
		t.Fatalf("hook returned %v", err)
	}
	x, y := cellOf(t, w)
	if !within(t, x, y, 7, 9, 1) {
		t.Errorf("placed at (%s,%s); expected next to (7,9)", x, y)
	}
}

// The sentinel reads the unsuffixed keys — the CursorMan, who has no id.
func TestCanvasNearCursorSentinel(t *testing.T) {
	clearCursors(t)
	all := []*mywant.Want{guiStateWant(map[string]any{
		"canvas_cursor_x": 12,
		"canvas_cursor_y": 4,
	})}

	w := nearWant(canvasNearCursor)
	if err := (&CanvasNearHook{}).Run(w, all, []*mywant.Want{w}); err != nil {
		t.Fatalf("hook returned %v", err)
	}
	x, y := cellOf(t, w)
	if !within(t, x, y, 12, 4, 1) {
		t.Errorf("placed at (%s,%s); expected next to the CursorMan at (12,4)", x, y)
	}
}

// Three wants deployed together must not all claim the same cell.
func TestCanvasNearSpreadsABatch(t *testing.T) {
	setLiveCursor(t, "chr-hero", 25, 64)

	batch := []*mywant.Want{nearWant("chr-hero"), nearWant("chr-hero"), nearWant("chr-hero")}
	for i, w := range batch {
		w.Metadata.ID = "new-" + strconv.Itoa(i)
	}
	seen := map[string]bool{}
	for _, w := range batch {
		if err := (&CanvasNearHook{}).Run(w, nil, batch); err != nil {
			t.Fatalf("hook returned %v", err)
		}
		x, y := cellOf(t, w)
		if seen[x+","+y] {
			t.Fatalf("two wants placed on the same cell (%s,%s)", x, y)
		}
		seen[x+","+y] = true
	}
}

// An explicit position is the more specific answer and wins.
func TestCanvasNearYieldsToExplicitCoordinates(t *testing.T) {
	setLiveCursor(t, "chr-hero", 25, 64)

	w := nearWant("chr-hero")
	w.SetLabel(canvasLabelX, "1")
	w.SetLabel(canvasLabelY, "2")
	if err := (&CanvasNearHook{}).Run(w, nil, []*mywant.Want{w}); err != nil {
		t.Fatalf("hook returned %v", err)
	}
	if x, y := cellOf(t, w); x != "1" || y != "2" {
		t.Errorf("explicit (1,2) was overwritten with (%s,%s)", x, y)
	}
}

// An unknown character is not worth failing a deployment over: the want falls
// through to the ordinary scan, which is where it would have gone anyway.
func TestCanvasNearUnknownTargetIsNotAnError(t *testing.T) {
	clearCursors(t)

	w := nearWant("chr-nobody")
	if err := (&CanvasNearHook{}).Run(w, nil, []*mywant.Want{w}); err != nil {
		t.Fatalf("hook returned %v", err)
	}
	if x, y := cellOf(t, w); x != "" || y != "" {
		t.Errorf("assigned (%s,%s) for a character it cannot find; expected to leave it to the scan", x, y)
	}
}

// The cell the character stands on is free, and the search still skips it —
// but a neighbour that is taken must also be skipped.
func TestCanvasNearAvoidsOccupiedNeighbours(t *testing.T) {
	setLiveCursor(t, "chr-hero", 0, 0)

	// Ring the character in, leaving exactly one gap at (1,1).
	var all []*mywant.Want
	n := 0
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			if dx == 1 && dy == 1 {
				continue
			}
			all = append(all, tileAt("w"+strconv.Itoa(n), dx, dy))
			n++
		}
	}

	w := nearWant("chr-hero")
	if err := (&CanvasNearHook{}).Run(w, all, []*mywant.Want{w}); err != nil {
		t.Fatalf("hook returned %v", err)
	}
	if x, y := cellOf(t, w); x != "1" || y != "1" {
		t.Errorf("placed at (%s,%s); the only free neighbour was (1,1)", x, y)
	}
}

func setLiveCursor(t *testing.T, id string, x, y float64) {
	t.Helper()
	cursorsMu.Lock()
	cursors[id] = cursorEntry{X: x, Y: y, LastSeen: time.Now().UnixMilli()}
	cursorsMu.Unlock()
	t.Cleanup(func() { clearCursors(t) })
}

func clearCursors(t *testing.T) {
	t.Helper()
	cursorsMu.Lock()
	cursors = map[string]cursorEntry{}
	cursorsMu.Unlock()
}

// within reports whether the assigned cell sits on the Chebyshev ring of the
// given radius around (cx, cy).
func within(t *testing.T, xs, ys string, cx, cy, radius int) bool {
	t.Helper()
	x, y := atoi(t, xs), atoi(t, ys)
	return absInt(x-cx) <= radius && absInt(y-cy) <= radius
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	v, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("not a number: %q", s)
	}
	return v
}
