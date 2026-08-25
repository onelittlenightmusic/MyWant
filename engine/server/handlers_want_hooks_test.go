package server

import (
	"testing"

	mywant "mywant/engine/core"
)

// Every want type starts as a single cell. Certain categories (travel,
// transport, tunnel) used to arrive pre-stretched to two cells, which made the
// canvas harder to arrange and could not be undone from the UI — the frontend
// applied the same category default as a hard floor.
func TestCanvasTileSizeHookDefaultsToSingleCell(t *testing.T) {
	hook := &CanvasTileSizeHook{}

	for _, wantType := range []string{"transit_search", "caddy", "cloudflare", "hotel", "flight", "queue"} {
		w := &mywant.Want{Metadata: mywant.Metadata{ID: "id-" + wantType, Type: wantType}}
		if err := hook.Run(w, nil, nil); err != nil {
			t.Fatalf("%s: hook returned %v", wantType, err)
		}
		if got := w.GetLabel(canvasLabelLength); got != "0" {
			t.Errorf("%s: canvas-length = %q, want \"0\" (a single cell)", wantType, got)
		}
		if got := w.GetLabel(canvasLabelRotation); got != "0" {
			t.Errorf("%s: canvas-rotation = %q, want \"0\"", wantType, got)
		}
	}
}

// A length the caller already chose is the user's layout decision and must
// survive — this is what keeps a deliberately stretched tile stretched.
func TestCanvasTileSizeHookKeepsExplicitValues(t *testing.T) {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: "id-wall", Type: "wall"}}
	w.SetLabel(canvasLabelLength, "4")
	w.SetLabel(canvasLabelRotation, "90")

	if err := hookRun(t, w); err != nil {
		t.Fatalf("hook returned %v", err)
	}
	if got := w.GetLabel(canvasLabelLength); got != "4" {
		t.Errorf("canvas-length = %q, want it left at \"4\"", got)
	}
	if got := w.GetLabel(canvasLabelRotation); got != "90" {
		t.Errorf("canvas-rotation = %q, want it left at \"90\"", got)
	}
}

func hookRun(t *testing.T, w *mywant.Want) error {
	t.Helper()
	return (&CanvasTileSizeHook{}).Run(w, nil, nil)
}

// A want asked for an occupied cell should arrive BESIDE it, and "beside"
// should mean the cell a person would look at first.
//
// The ring used to be walked from -radius in both axes, so the answer was
// always the top-left diagonal: on the board a new tile appeared one cell up
// and one cell left of the character every time, which read as a placement bug
// rather than as a free cell being picked.
func TestSpiralPrefersTheCellToTheRight(t *testing.T) {
	// Only the centre is taken.
	free := func(x, y int) bool { return !(x == 5 && y == 5) }

	x, y, ok := spiralFreeCell(5, 5, 0, free)
	if !ok {
		t.Fatal("no free cell found")
	}
	if x != 6 || y != 5 {
		t.Fatalf("crowded out of (5,5): got (%d,%d), want the cell to the right (6,5)", x, y)
	}
}

// Every orthogonal neighbour comes before any diagonal one: sharing an edge is
// nearer than sharing a corner, however the Chebyshev radius counts it.
func TestRingCellsPutEdgesBeforeCorners(t *testing.T) {
	ring := ringCells(1)
	if len(ring) != 8 {
		t.Fatalf("ring of radius 1 has %d cells, want 8", len(ring))
	}
	want := []ringOffset{
		{1, 0}, {0, 1}, {-1, 0}, {0, -1}, // east, south, west, north
		{1, 1}, {-1, 1}, {-1, -1}, {1, -1}, // then the corners, same turn
	}
	for i, w := range want {
		if ring[i] != w {
			t.Fatalf("ring[%d] = %v, want %v (full order %v)", i, ring[i], w, ring)
		}
	}
}

// A want asked for the cell you are standing on should arrive there.
//
// Every character carries a hidden chat want that rides at their own cell. It
// draws nothing, but it was counted as furniture, so the cell under your feet
// was permanently taken and anything deployed there was bumped aside — which
// read as a rule against placing a want on yourself. There is no such rule.
func TestHiddenWantsDoNotOccupyTheirCell(t *testing.T) {
	hidden := &mywant.Want{}
	hidden.Metadata.ID = "chat-chr-1"
	hidden.SetLabel(canvasLabelX, "4")
	hidden.SetLabel(canvasLabelY, "7")
	hidden.SetLabel(canvasLabelHidden, "true")

	occupied := map[[2]int]bool{}
	markWantOccupied(hidden, occupied)
	if occupied[[2]int{4, 7}] {
		t.Fatal("a hidden want claimed its cell; nothing is drawn there")
	}

	// And a visible one still does.
	visible := &mywant.Want{}
	visible.Metadata.ID = "gate"
	visible.SetLabel(canvasLabelX, "4")
	visible.SetLabel(canvasLabelY, "7")
	markWantOccupied(visible, occupied)
	if !occupied[[2]int{4, 7}] {
		t.Fatal("a visible want failed to claim its cell")
	}
}
