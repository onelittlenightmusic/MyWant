package server

import (
	"fmt"
	"math"
	"strconv"

	mywant "mywant/engine/core"
)

// WantCreationHook is called for every want in a batch just before it is added
// to the ChainBuilder.  Implementations may mutate the want in place (e.g. to
// inject labels or params).  If a hook returns an error the whole batch is
// aborted with HTTP 400.
type WantCreationHook interface {
	// Name returns a short identifier used in logs.
	Name() string
	// Run is invoked once per want.  allWants is the full current list of
	// already-deployed wants (read-only context).  newBatch is the slice of
	// wants being created in this request (may be used to avoid conflicts).
	Run(want *mywant.Want, allWants []*mywant.Want, newBatch []*mywant.Want) error
}

// RegisterWantCreationHook appends a hook to the server's creation pipeline.
func (s *Server) RegisterWantCreationHook(h WantCreationHook) {
	s.wantCreationHooks = append(s.wantCreationHooks, h)
}

// runWantCreationHooks executes all registered hooks for every want in the batch.
func (s *Server) runWantCreationHooks(batch []*mywant.Want, allWants []*mywant.Want) error {
	for _, want := range batch {
		for _, hook := range s.wantCreationHooks {
			if err := hook.Run(want, allWants, batch); err != nil {
				return fmt.Errorf("hook %q failed for want %q: %w", hook.Name(), want.Metadata.Name, err)
			}
		}
	}
	return nil
}

// ── Built-in hook: OrderKey assignment ───────────────────────────────────────

// OrderKeyHook assigns a monotonically increasing OrderKey to wants that don't
// have one, preserving the stable list-view sort order.
type OrderKeyHook struct{}

func (h *OrderKeyHook) Name() string { return "order-key" }

func (h *OrderKeyHook) Run(want *mywant.Want, allWants []*mywant.Want, newBatch []*mywant.Want) error {
	if want.Metadata.OrderKey != "" {
		return nil
	}
	// Find the current maximum OrderKey across existing wants and already-assigned batch members.
	var lastKey string
	for _, w := range allWants {
		if w.Metadata.OrderKey > lastKey {
			lastKey = w.Metadata.OrderKey
		}
	}
	for _, bw := range newBatch {
		if bw.Metadata.ID == want.Metadata.ID {
			continue
		}
		if bw.Metadata.OrderKey > lastKey {
			lastKey = bw.Metadata.OrderKey
		}
	}
	want.Metadata.OrderKey = mywant.GenerateOrderKeyAfter(lastKey)
	return nil
}

// ── Built-in hook: want type defaults ─────────────────────────────────────────

// WantTypeDefaultsHook injects defaults from the registered WantTypeDefinition
// into the want spec (currently: Requires).
type WantTypeDefaultsHook struct {
	builder interface {
		GetWantTypeDefinition(typeName string) *mywant.WantTypeDefinition
	}
}

func (h *WantTypeDefaultsHook) Name() string { return "want-type-defaults" }

func (h *WantTypeDefaultsHook) Run(want *mywant.Want, _ []*mywant.Want, _ []*mywant.Want) error {
	typeDef := h.builder.GetWantTypeDefinition(want.Metadata.Type)
	if typeDef == nil {
		return nil
	}
	if len(want.Spec.Requires) == 0 && len(typeDef.Requires) > 0 {
		want.Spec.Requires = typeDef.Requires
	}
	return nil
}

// ── Built-in hook: memo recording ────────────────────────────────────────────

// ThingHook records parameter values into the ThingStore when the parameter's
// WantTypeDefinition declares a non-empty SubType.
type ThingHook struct {
	memo    *ThingStore
	events  *ThingEventStore
	builder interface {
		GetWantTypeDefinition(typeName string) *mywant.WantTypeDefinition
	}
}

func (h *ThingHook) Name() string { return "memo" }

func (h *ThingHook) Run(want *mywant.Want, _ []*mywant.Want, _ []*mywant.Want) error {
	typeDef := h.builder.GetWantTypeDefinition(want.Metadata.Type)
	if typeDef == nil {
		return nil
	}
	for _, pd := range typeDef.Parameters {
		if pd.SubType == "" {
			continue
		}
		// Skip if recordMemo is explicitly false.
		if !pd.ShouldRecordThing() {
			continue
		}
		val, ok := want.Spec.Params[pd.Name]
		if !ok {
			continue
		}
		str, ok := val.(string)
		if !ok || str == "" {
			continue
		}
		if err := h.memo.Record(pd.SubType, str); err != nil {
			mywant.WarnLog("[ThingHook] failed to record %s=%q: %v", pd.SubType, str, err)
		}
		if h.events != nil {
			_ = h.events.Record(ThingEvent{
				Catalog:  subtypeToKey(pd.SubType),
				Subtype:  pd.SubType,
				Value:    str,
				Source:   MemoSourceWantParam,
				WantID:   want.Metadata.ID,
				WantType: want.Metadata.Type,
			})
		}
	}
	return nil
}

// ── Built-in hook: canvas coordinate assignment ───────────────────────────────

const (
	canvasLabelX        = "mywant.io/canvas-x"
	canvasLabelY        = "mywant.io/canvas-y"
	canvasLabelRotation = "mywant.io/canvas-rotation"
	canvasLabelLength   = "mywant.io/canvas-length"
)

// CanvasTileSizeHook sets default canvas-rotation (0) and canvas-length (0, a
// single cell) on wants that do not already have them.  Must run before
// CanvasCoordinateHook.
//
// Every want type starts as a 1×1 tile. Certain categories (travel, transport,
// tunnel) used to default to length 1 — a tile two cells long — but a tile's
// size is a layout choice the user makes by stretching it on the canvas, not a
// property of what the want is, and having some types arrive pre-stretched only
// made the canvas harder to arrange. Length stays freely settable per want; it
// just is no longer decided by category.
// No builder dependency: the default no longer depends on the want type at all.
type CanvasTileSizeHook struct{}

func (h *CanvasTileSizeHook) Name() string { return "canvas-tile-size" }

func (h *CanvasTileSizeHook) Run(want *mywant.Want, _ []*mywant.Want, _ []*mywant.Want) error {
	if want.GetLabel(canvasLabelRotation) == "" {
		want.SetLabel(canvasLabelRotation, "0")
	}
	if want.GetLabel(canvasLabelLength) == "" {
		want.SetLabel(canvasLabelLength, "0")
	}
	return nil
}

// tileFootprint returns all grid cells occupied by a want anchored at (ax, ay).
// rotation: 0=right, 90=down, 180=left, 270=up. length = extra cells beyond the anchor.
func tileFootprint(ax, ay, rotation, length int) [][2]int {
	span := length + 1
	cells := make([][2]int, span)
	for i := range span {
		switch rotation {
		case 90:
			cells[i] = [2]int{ax, ay + i}
		case 180:
			cells[i] = [2]int{ax - i, ay}
		case 270:
			cells[i] = [2]int{ax, ay - i}
		default: // 0
			cells[i] = [2]int{ax + i, ay}
		}
	}
	return cells
}

// markWantOccupied adds all cells of a want (including multi-cell spans) into occupied.
func markWantOccupied(w *mywant.Want, occupied map[[2]int]bool) {
	// One snapshot under the read lock: reading label-by-label would let a
	// concurrent move land between the x and y reads.
	labels := w.GetLabels()
	rx, errX := strconv.Atoi(labels[canvasLabelX])
	ry, errY := strconv.Atoi(labels[canvasLabelY])
	if errX != nil || errY != nil {
		return
	}
	rot := 0
	length := 0
	if v, err := strconv.Atoi(labels[canvasLabelRotation]); err == nil {
		rot = v
	}
	if v, err := strconv.Atoi(labels[canvasLabelLength]); err == nil {
		length = v
	}
	for _, c := range tileFootprint(rx, ry, rot, length) {
		occupied[c] = true
	}
}

// tileGeometry reads a want's own rotation/length (set by CanvasTileSizeHook).
func tileGeometry(w *mywant.Want) (rot, length int) {
	if v, err := strconv.Atoi(w.GetLabel(canvasLabelRotation)); err == nil {
		rot = v
	}
	if v, err := strconv.Atoi(w.GetLabel(canvasLabelLength)); err == nil {
		length = v
	}
	return rot, length
}

// canvasOccupancy returns the cells already taken — by everything on the board
// and by everything else arriving in the same batch — together with a test for
// whether `want` fits at a given anchor, its full footprint included.
//
// Batch members matter as much as the board: three wants deployed together are
// placed one at a time, and each has to see where the previous two landed or
// all three claim the same cell.
func canvasOccupancy(want *mywant.Want, allWants, newBatch []*mywant.Want) (map[[2]int]bool, func(x, y int) bool) {
	occupied := make(map[[2]int]bool)
	for _, w := range allWants {
		markWantOccupied(w, occupied)
	}
	for _, bw := range newBatch {
		if bw.Metadata.ID == want.Metadata.ID {
			continue
		}
		markWantOccupied(bw, occupied)
	}
	rot, length := tileGeometry(want)
	return occupied, func(x, y int) bool {
		for _, c := range tileFootprint(x, y, rot, length) {
			if occupied[c] {
				return false
			}
		}
		return true
	}
}

// ── Built-in hook: "put this near someone" ───────────────────────────────────

// canvasLabelNear names whoever a want wants to arrive beside: a character id,
// or the sentinel below for the CursorMan.
const canvasLabelNear = "mywant.io/canvas-near"

// canvasNearCursor asks for the CursorMan — the robot cursor, whose position
// lives in the unsuffixed gui_state keys. Spelled as a word because a deploy
// file that says "near the cursor" should not have to know an id.
const canvasNearCursor = "cursor"

// CanvasNearHook turns "near so-and-so" into a coordinate.
//
// A deployment file is written before the game is played, so it cannot say
// where to put anything: the one thing it knows is who the new want is FOR.
// Coordinates are what the board wants, and only the server knows both — it is
// holding every player's live position — so the translation belongs here rather
// than in whatever wrote the YAML. The file states the intent; this resolves it
// at the moment of deployment.
//
// Runs before CanvasCoordinateHook, which then does the real work: this only
// seeds canvas-x/y, and the seeding is deliberately a full placement search so
// the coordinate handed on is one that fits.
type CanvasNearHook struct{}

func (h *CanvasNearHook) Name() string { return "canvas-near" }

func (h *CanvasNearHook) Run(want *mywant.Want, allWants []*mywant.Want, newBatch []*mywant.Want) error {
	target := want.GetLabel(canvasLabelNear)
	if target == "" {
		return nil
	}
	// An explicit position wins. Somebody said exactly where; "near someone" is
	// the vaguer of the two answers and has no business overruling it.
	if want.GetLabel(canvasLabelX) != "" && want.GetLabel(canvasLabelY) != "" {
		return nil
	}
	cx, cy, ok := resolveCanvasNear(target, allWants)
	if !ok {
		// Nobody by that name, or they have never been anywhere. Not an error
		// worth failing a deployment over — the want still gets placed, just by
		// the ordinary scan, which is exactly where it would have gone before
		// anyone thought to ask for this.
		return nil
	}

	_, isFreeAt := canvasOccupancy(want, allWants, newBatch)
	// From radius 1: beside them, not underneath them. See spiralFreeCell.
	x, y, found := spiralFreeCell(cx, cy, 1, isFreeAt)
	if !found {
		return nil
	}
	want.SetLabel(canvasLabelX, strconv.Itoa(x))
	want.SetLabel(canvasLabelY, strconv.Itoa(y))
	return nil
}

// resolveCanvasNear finds the cell a canvas-near target is standing on.
//
// Three places to look, in order of how current they are: a live cursor is
// someone with a tab open reporting where they are right now; the per-character
// gui_state keys are where a character was left when nobody is playing them;
// and the unsuffixed keys are the CursorMan. A character with a tab open and a
// stale gui_state entry must resolve to the tab, which is why the live map is
// asked first.
func resolveCanvasNear(target string, allWants []*mywant.Want) (int, int, bool) {
	if target != canvasNearCursor {
		cursorsMu.RLock()
		e, ok := cursors[target]
		cursorsMu.RUnlock()
		if ok && hasLiveCursor(target) {
			return int(math.Round(e.X)), int(math.Round(e.Y)), true
		}
	}

	var state map[string]any
	for _, w := range allWants {
		if w.Metadata.ID == guiStateWantID {
			state = w.GetAllState()
			break
		}
	}
	if state == nil {
		return 0, 0, false
	}

	xKey, yKey := "canvas_cursor_x", "canvas_cursor_y"
	if target != canvasNearCursor {
		xKey, yKey = cursorStateXPrefix+target, cursorStateYPrefix+target
	}
	x, okX := cursorCoord(state[xKey])
	y, okY := cursorCoord(state[yKey])
	if !okX || !okY {
		return 0, 0, false
	}
	return int(math.Round(x)), int(math.Round(y)), true
}

// CanvasCoordinateHook assigns mywant.io/canvas-x and canvas-y labels to wants.
// If a position is already set (e.g. from cursorman location), it verifies the
// footprint is free; if occupied it spirals outward to find the nearest free cell.
// Wants without a requested position are placed via a default row-by-row scan.
type CanvasCoordinateHook struct{}

func (h *CanvasCoordinateHook) Name() string { return "canvas-coordinate" }

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// spiralFreeCell walks Chebyshev rings outward from (cx, cy) and returns the
// first cell isFree accepts, or ok=false if the search gives up.
//
// minRadius is where the search starts: 0 means "(cx, cy) itself is a fine
// answer", 1 means "beside it, not on it". The second is what "put this near
// someone" wants — a want placed on the exact cell a player is standing on
// appears underneath them, which reads as the board swallowing them rather than
// as something arriving next to them.
func spiralFreeCell(cx, cy, minRadius int, isFree func(x, y int) bool) (int, int, bool) {
	if minRadius <= 0 && isFree(cx, cy) {
		return cx, cy, true
	}
	for radius := max(minRadius, 1); radius <= 100; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dy := -radius; dy <= radius; dy++ {
				// Ring, not disc: the interior was covered by earlier radii.
				if absInt(dx) != radius && absInt(dy) != radius {
					continue
				}
				if isFree(cx+dx, cy+dy) {
					return cx + dx, cy + dy, true
				}
			}
		}
	}
	return 0, 0, false
}

func (h *CanvasCoordinateHook) Run(want *mywant.Want, allWants []*mywant.Want, newBatch []*mywant.Want) error {
	if len(want.Metadata.OwnerReferences) > 0 {
		return nil
	}

	myRot, myLen := tileGeometry(want)
	occupied, isFreeAt := canvasOccupancy(want, allWants, newBatch)
	placeAt := func(x, y int) {
		want.SetLabel(canvasLabelX, strconv.Itoa(x))
		want.SetLabel(canvasLabelY, strconv.Itoa(y))
		for _, c := range tileFootprint(x, y, myRot, myLen) {
			occupied[c] = true
		}
	}

	// If a requested position was provided by the frontend (e.g. cursorman position),
	// try it first; if occupied spiral outward (Chebyshev rings) to find nearest free cell.
	if want.GetLabel(canvasLabelX) != "" && want.GetLabel(canvasLabelY) != "" {
		reqX, errX := strconv.Atoi(want.GetLabel(canvasLabelX))
		reqY, errY := strconv.Atoi(want.GetLabel(canvasLabelY))
		if errX == nil && errY == nil {
			if x, y, ok := spiralFreeCell(reqX, reqY, 0, isFreeAt); ok {
				placeAt(x, y)
				return nil
			}
		}
		// Clear labels so the fallback scan can reassign.
		want.SetLabel(canvasLabelX, "")
		want.SetLabel(canvasLabelY, "")
	}

	// Fallback: scan left-to-right, top-to-bottom from (0,0).
	const rowWidth = 10
	for row := 0; ; row++ {
		for col := range rowWidth {
			if isFreeAt(col, row) {
				placeAt(col, row)
				return nil
			}
		}
	}
}
