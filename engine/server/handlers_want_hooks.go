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
	canvasLabelX = "mywant.io/canvas-x"
	canvasLabelY = "mywant.io/canvas-y"
)

// CanvasCoordinateHook assigns mywant.io/canvas-x and canvas-y labels to wants
// that do not already have them.  It places each new want at the next free grid
// cell, scanning row-by-row from (0,0), skipping cells occupied by existing or
// already-assigned wants in the same batch.
type CanvasCoordinateHook struct{}

func (h *CanvasCoordinateHook) Name() string { return "canvas-coordinate" }

func (h *CanvasCoordinateHook) Run(want *mywant.Want, allWants []*mywant.Want, newBatch []*mywant.Want) error {
	// Skip if coordinates already set (e.g. want was explicitly positioned or is a child).
	if want.Metadata.Labels == nil {
		want.Metadata.Labels = make(map[string]string)
	}
	if want.Metadata.Labels[canvasLabelX] != "" && want.Metadata.Labels[canvasLabelY] != "" {
		return nil
	}
	// Skip child wants (they are managed by their parent Target).
	if len(want.Metadata.OwnerReferences) > 0 {
		return nil
	}

	// Build occupied set from existing wants + already-assigned batch members.
	type cell struct{ x, y int }
	occupied := make(map[cell]bool)

	for _, w := range allWants {
		if w.Metadata.Labels == nil {
			continue
		}
		rx, errX := strconv.Atoi(w.Metadata.Labels[canvasLabelX])
		ry, errY := strconv.Atoi(w.Metadata.Labels[canvasLabelY])
		if errX == nil && errY == nil {
			occupied[cell{rx, ry}] = true
		}
	}
	for _, bw := range newBatch {
		if bw.Metadata.ID == want.Metadata.ID {
			continue // skip self
		}
		if bw.Metadata.Labels == nil {
			continue
		}
		rx, errX := strconv.Atoi(bw.Metadata.Labels[canvasLabelX])
		ry, errY := strconv.Atoi(bw.Metadata.Labels[canvasLabelY])
		if errX == nil && errY == nil {
			occupied[cell{rx, ry}] = true
		}
	}

	// Find the next free cell scanning left-to-right, top-to-bottom (row width = 10).
	const rowWidth = 10
	for row := 0; ; row++ {
		for col := 0; col < rowWidth; col++ {
			c := cell{col, row}
			if !occupied[c] {
				want.Metadata.Labels[canvasLabelX] = strconv.Itoa(col)
				want.Metadata.Labels[canvasLabelY] = strconv.Itoa(row)
				occupied[c] = true // claim for the next want in the batch
				return nil
			}
		}
	}
}
