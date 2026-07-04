package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[AuraEraseWant, AuraEraseLocals]("aura_erase")
	})
}

type AuraEraseLocals struct{}

// AuraEraseWant is a persistent effect want that targets a single character
// (drive-want-style binding) and erases aura cells within a radius of that
// character's current position. It is a separate want instance from any
// "aura" want it clears — an erase is delivered via POST
// /api/v1/webhooks/{id} with {"action":"erase","x":N,"y":M}, and clears
// matching cells across every "aura" want in the graph.
type AuraEraseWant struct{ Want }

func (a *AuraEraseWant) GetLocals() *AuraEraseLocals {
	return CheckLocalsInitialized[AuraEraseLocals](&a.Want)
}

func (a *AuraEraseWant) Initialize() {
	if chars := a.GetStringSliceParam("characters"); len(chars) > 0 {
		a.SetCurrent("characters", chars)
	}
	a.SetCurrent("erase_radius", a.GetIntParam("erase_radius", 1))
}

func (a *AuraEraseWant) IsAchieved() bool { return false }

// Progress drains every webhook_queue entry accumulated since the last
// tick — see AuraWant.Progress for why this uses AppendState/DrainState
// instead of the single-slot webhook_payload pattern.
func (a *AuraEraseWant) Progress() {
	entries := a.DrainState("webhook_queue")
	if len(entries) == 0 {
		return
	}
	radius := a.eraseRadius()
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		pm, ok := m["payload"].(map[string]any)
		if !ok {
			continue
		}
		if action, _ := pm["action"].(string); action != "erase" {
			continue
		}
		x, ok1 := intFromAny(pm["x"])
		y, ok2 := intFromAny(pm["y"])
		if !ok1 || !ok2 {
			continue
		}
		eraseAuraCellsNear(x, y, radius)
	}
}

func (a *AuraEraseWant) eraseRadius() int {
	if raw, ok := a.GetCurrent("erase_radius"); ok {
		if r, ok := intFromAny(raw); ok {
			return r
		}
	}
	return 1
}

// eraseAuraCellsNear clears any cell within a Chebyshev (square) radius of
// (x, y) from every "aura" want's "cells" state.
func eraseAuraCellsNear(x, y, radius int) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return
	}
	for _, want := range cb.GetWants() {
		if want.Metadata.Type != "aura" {
			continue
		}
		raw, ok := cb.GetWantStateValue(want.Metadata.ID, "cells")
		if !ok {
			continue
		}
		cells := cellsFromAny(raw)
		filtered := make([]AuraCell, 0, len(cells))
		changed := false
		for _, c := range cells {
			if absInt(c.X-x) <= radius && absInt(c.Y-y) <= radius {
				changed = true
				continue
			}
			filtered = append(filtered, c)
		}
		if changed {
			cb.StoreWantState(want.Metadata.ID, "cells", filtered)
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
