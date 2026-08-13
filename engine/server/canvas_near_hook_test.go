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

// A browser's own report beats gui_state: it is the character saying where it
// is, gui_state is where something else last wrote it down.
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

// With nobody publishing a cursor, the sentinel falls back to the CursorMan's
// own keys — but only while it is actually on the board.
func TestCanvasNearCursorSentinel(t *testing.T) {
	clearCursors(t)
	all := []*mywant.Want{guiStateWant(map[string]any{
		"cursor_visible":  true,
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

// The unsuffixed keys are plain gui_state entries that default to 0, so on a
// board where the CursorMan was never summoned they read as a confident (0, 0)
// — which put wants in the corner of the world and called it success.
func TestCanvasNearCursorIgnoresAnAbsentCursorMan(t *testing.T) {
	clearCursors(t)
	all := []*mywant.Want{guiStateWant(map[string]any{
		"cursor_visible":  false,
		"canvas_cursor_x": 0,
		"canvas_cursor_y": 0,
	})}

	w := nearWant(canvasNearCursor)
	if err := (&CanvasNearHook{}).Run(w, all, []*mywant.Want{w}); err != nil {
		t.Fatalf("hook returned %v", err)
	}
	if x, y := cellOf(t, w); x != "" || y != "" {
		t.Errorf("placed at (%s,%s) beside a CursorMan that is not on the board", x, y)
	}
}

// "Near the cursor" means whoever is at the controls. With two players, the one
// who moved most recently is the one who just deployed this.
func TestCanvasNearCursorPicksTheMostRecentPlayer(t *testing.T) {
	clearCursors(t)
	putCursor("chr-old", cursorEntry{X: 3, Y: 3, LastSeen: time.Now().Add(-20 * time.Second).UnixMilli()})
	putCursor("chr-now", cursorEntry{X: 40, Y: 8, LastSeen: time.Now().UnixMilli()})
	t.Cleanup(func() { clearCursors(t) })

	w := nearWant(canvasNearCursor)
	if err := (&CanvasNearHook{}).Run(w, nil, []*mywant.Want{w}); err != nil {
		t.Fatalf("hook returned %v", err)
	}
	x, y := cellOf(t, w)
	if !within(t, x, y, 40, 8, 1) {
		t.Errorf("placed at (%s,%s); expected next to the player who moved last, at (40,8)", x, y)
	}
}

// setLiveCursor publishes a position the way updateCursor does: into both the
// presence map and the durable one.
// The presence map is pruned to eight seconds, which is right for drawing
// other people's cursors and wrong for asking where somebody is. A deploy that
// landed in the gap between two publishes used to fall through to the corner of
// the world — the bug this whole distinction exists for.
func TestCanvasNearUsesAPositionOlderThanPresenceTTL(t *testing.T) {
	clearCursors(t)
	// Published a minute ago and since pruned from `cursors`, as a real GET
	// /api/v1/cursors would have done.
	cursorsMu.Lock()
	lastCursorPos["chr-hero"] = cursorEntry{X: 27, Y: 62, LastSeen: time.Now().Add(-time.Minute).UnixMilli()}
	cursorsMu.Unlock()
	t.Cleanup(func() { clearCursors(t) })

	for _, target := range []string{"chr-hero", canvasNearCursor} {
		w := nearWant(target)
		if err := (&CanvasNearHook{}).Run(w, nil, []*mywant.Want{w}); err != nil {
			t.Fatalf("%s: hook returned %v", target, err)
		}
		x, y := cellOf(t, w)
		if x == "" || !within(t, x, y, 27, 62, 1) {
			t.Errorf("near %q placed at (%s,%s); expected next to (27,62)", target, x, y)
		}
	}
}

func setLiveCursor(t *testing.T, id string, x, y float64) {
	t.Helper()
	putCursor(id, cursorEntry{X: x, Y: y, LastSeen: time.Now().UnixMilli()})
	t.Cleanup(func() { clearCursors(t) })
}

func putCursor(id string, e cursorEntry) {
	cursorsMu.Lock()
	cursors[id] = e
	lastCursorPos[id] = e
	cursorsMu.Unlock()
}

func clearCursors(t *testing.T) {
	t.Helper()
	cursorsMu.Lock()
	cursors = map[string]cursorEntry{}
	lastCursorPos = map[string]cursorEntry{}
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
