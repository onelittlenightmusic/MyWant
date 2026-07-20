package server

import (
	"fmt"
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

// MemoHook records parameter values into the MemoStore when the parameter's
// WantTypeDefinition declares a non-empty SubType.
type MemoHook struct {
	memo    *MemoStore
	builder interface {
		GetWantTypeDefinition(typeName string) *mywant.WantTypeDefinition
	}
}

func (h *MemoHook) Name() string { return "memo" }

func (h *MemoHook) Run(want *mywant.Want, _ []*mywant.Want, _ []*mywant.Want) error {
	typeDef := h.builder.GetWantTypeDefinition(want.Metadata.Type)
	if typeDef == nil {
		return nil
	}
	for _, pd := range typeDef.Parameters {
		if pd.SubType == "" {
			continue
		}
		// Skip if recordMemo is explicitly false.
		if pd.RecordMemo != nil && !*pd.RecordMemo {
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
			mywant.WarnLog("[MemoHook] failed to record %s=%q: %v", pd.SubType, str, err)
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

// categoryDefaultLength maps want-type category → default extra-cell count.
// length=0 means 1×1, length=1 means 1×2, etc.
var categoryDefaultLength = map[string]int{
	"travel":    1,
	"transit":   1,
	"transport": 1,
	"tunnel":    1,
}

// CanvasTileSizeHook sets default canvas-rotation (0) and canvas-length (category-dependent)
// on wants that do not already have them.  Must run before CanvasCoordinateHook.
type CanvasTileSizeHook struct {
	builder interface {
		GetWantTypeDefinition(typeName string) *mywant.WantTypeDefinition
	}
}

func (h *CanvasTileSizeHook) Name() string { return "canvas-tile-size" }

func (h *CanvasTileSizeHook) Run(want *mywant.Want, _ []*mywant.Want, _ []*mywant.Want) error {
	if want.GetLabel(canvasLabelRotation) == "" {
		want.SetLabel(canvasLabelRotation, "0")
	}
	if want.GetLabel(canvasLabelLength) == "" {
		defaultLen := 0
		typeDef := h.builder.GetWantTypeDefinition(want.Metadata.Type)
		if typeDef != nil {
			if dl, ok := categoryDefaultLength[typeDef.Metadata.Category]; ok {
				defaultLen = dl
			}
		}
		want.SetLabel(canvasLabelLength, strconv.Itoa(defaultLen))
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

func (h *CanvasCoordinateHook) Run(want *mywant.Want, allWants []*mywant.Want, newBatch []*mywant.Want) error {
	if len(want.Metadata.OwnerReferences) > 0 {
		return nil
	}

	// Read this want's own rotation/length (set by CanvasTileSizeHook).
	myRot := 0
	myLen := 0
	if v, err := strconv.Atoi(want.GetLabel(canvasLabelRotation)); err == nil {
		myRot = v
	}
	if v, err := strconv.Atoi(want.GetLabel(canvasLabelLength)); err == nil {
		myLen = v
	}

	// Build occupied set, accounting for multi-cell spans of existing tiles.
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

	isFreeAt := func(x, y int) bool {
		for _, c := range tileFootprint(x, y, myRot, myLen) {
			if occupied[c] {
				return false
			}
		}
		return true
	}
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
			if isFreeAt(reqX, reqY) {
				placeAt(reqX, reqY)
				return nil
			}
			for radius := 1; radius <= 100; radius++ {
				for dx := -radius; dx <= radius; dx++ {
					for dy := -radius; dy <= radius; dy++ {
						if absInt(dx) != radius && absInt(dy) != radius {
							continue
						}
						if isFreeAt(reqX+dx, reqY+dy) {
							placeAt(reqX+dx, reqY+dy)
							return nil
						}
					}
				}
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
