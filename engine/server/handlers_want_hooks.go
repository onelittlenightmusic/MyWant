package server

import (
	"fmt"
	"math"
	"sort"
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
	// canvasLabelHidden marks a want that has a cell but is not drawn in it.
	canvasLabelHidden = "mywant.io/canvas-hidden"
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
	// A cell is taken by something you can see standing in it. A hidden want
	// has coordinates for its own bookkeeping and draws nothing, so treating it
	// as furniture pushed real tiles out of the way for no visible reason.
	//
	// Every character carries one: a chat want, hidden, that rides along at
	// whatever cell they are standing on. So the cell under your own feet was
	// permanently occupied by an invisible tile, and a want deployed where you
	// stand always arrived somewhere else — which looked like the board
	// refusing to put anything under you, and was never a rule anybody wrote.
	if labels[canvasLabelHidden] == "true" {
		return
	}
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

// canvasNearCursor asks for whoever is at the controls: the live cursor that
// moved most recently, or the CursorMan if it is on the board and nobody else
// is. Spelled as a word because a deploy file that says "near the cursor"
// should not have to know an id.
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
	var state map[string]any
	for _, w := range allWants {
		if w.Metadata.ID == guiStateWantID {
			state = w.GetAllState()
			break
		}
	}

	if target == canvasNearCursor {
		return resolveCursorSentinel(state)
	}

	cursorsMu.RLock()
	e, ok := lastCursorPos[target]
	cursorsMu.RUnlock()
	if ok {
		return int(math.Round(e.X)), int(math.Round(e.Y)), true
	}
	if state == nil {
		return 0, 0, false
	}
	x, okX := cursorCoord(state[cursorStateXPrefix+target])
	y, okY := cursorCoord(state[cursorStateYPrefix+target])
	if !okX || !okY {
		return 0, 0, false
	}
	return int(math.Round(x)), int(math.Round(y)), true
}

// resolveCursorSentinel answers "near the cursor" — meaning whoever is at the
// controls, not any particular character.
//
// The characters a browser has ever published for are asked first, most
// recently seen winning: somebody whose browser reports a position IS a person
// driving, and with two players the one who moved last is the one who just
// deployed this. Last-seen, not currently-live — see lastCursorPos. Presence
// times out in eight seconds and a deploy can easily land in the gap.
//
// The CursorMan's own keys (the unsuffixed canvas_cursor_x/y) come second, and
// only while it is on the board. They are plain gui_state keys that default to
// 0, so on a board where the CursorMan has never been summoned they read as a
// perfectly confident (0, 0) — which is how the first version of this put wants
// in the top-left corner of the world and reported success. An unset key is not
// a position, and cursor_visible is what tells the two apart.
func resolveCursorSentinel(state map[string]any) (int, int, bool) {
	var newest cursorEntry
	found := false
	cursorsMu.RLock()
	for _, e := range lastCursorPos {
		if !found || e.LastSeen > newest.LastSeen {
			newest, found = e, true
		}
	}
	cursorsMu.RUnlock()
	if found {
		return int(math.Round(newest.X)), int(math.Round(newest.Y)), true
	}

	if state == nil {
		return 0, 0, false
	}
	if visible, _ := state["cursor_visible"].(bool); !visible {
		return 0, 0, false
	}
	x, okX := cursorCoord(state["canvas_cursor_x"])
	y, okY := cursorCoord(state["canvas_cursor_y"])
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
		for _, d := range ringCells(radius) {
			if isFree(cx+d.dx, cy+d.dy) {
				return cx + d.dx, cy + d.dy, true
			}
		}
	}
	return 0, 0, false
}

type ringOffset struct{ dx, dy int }

// ringCells is one Chebyshev ring, in the order a person would look.
//
// The ring used to be walked as two nested loops from -radius, which made the
// first cell tried the top-left DIAGONAL every time. So a want asked for a cell
// somebody was already standing on did not arrive beside them, it arrived up
// and to the left — reliably, since the loop order never varied — and from the
// board that read as the tile landing in the wrong place by (-1, -1) rather
// than as a free cell being chosen.
//
// Two things decide the order instead. Orthogonal neighbours come before
// diagonal ones, because "next to" means sharing an edge before it means
// sharing a corner — that is Manhattan distance, which separates the two within
// a ring the Chebyshev radius treats as equal. Then it goes clockwise from due
// east, so the first answer is the cell to the right, where a reader of a
// left-to-right board looks for the next thing.
func ringCells(radius int) []ringOffset {
	out := make([]ringOffset, 0, 8*radius)
	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			// Ring, not disc: the interior was covered by earlier radii.
			if absInt(dx) != radius && absInt(dy) != radius {
				continue
			}
			out = append(out, ringOffset{dx, dy})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		mi := absInt(out[i].dx) + absInt(out[i].dy)
		mj := absInt(out[j].dx) + absInt(out[j].dy)
		if mi != mj {
			return mi < mj
		}
		return ringAngle(out[i]) < ringAngle(out[j])
	})
	return out
}

// ringAngle is the compass bearing of an offset, clockwise from due east.
//
// Canvas rows count downward, so atan2 taken with y as-is already turns
// clockwise on screen; shifting into [0, 2π) puts east at the start rather than
// on the wrap.
func ringAngle(d ringOffset) float64 {
	a := math.Atan2(float64(d.dy), float64(d.dx))
	if a < 0 {
		a += 2 * math.Pi
	}
	return a
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
