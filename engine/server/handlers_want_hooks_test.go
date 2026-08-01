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
