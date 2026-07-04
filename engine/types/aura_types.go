package types

import (
	"math"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[AuraWant, AuraLocals]("aura")
	})
}

type AuraLocals struct{}

// AuraCell is one painted grid cell: every character color currently present
// there. Multiple colors stack (no checkerboard mixing) rather than
// overwriting each other.
type AuraCell struct {
	X      int      `json:"x"`
	Y      int      `json:"y"`
	Colors []string `json:"colors"`
}

// auraFillCap bounds the enclosure flood-fill so an open trail (one that
// never closes a loop) can't scan the whole canvas looking for a wall.
const auraFillCap = 2000

// AuraWant is a persistent effect want that targets a single character
// (drive-want-style binding) and paints that character's color onto grid
// cells. A placement is delivered via POST /api/v1/webhooks/{id} with
// {"action":"place","x":N,"y":M}. When a placement closes a loop of this
// want's own cells, the enclosed interior is filled with the same color.
type AuraWant struct{ Want }

func (a *AuraWant) GetLocals() *AuraLocals {
	return CheckLocalsInitialized[AuraLocals](&a.Want)
}

func (a *AuraWant) Initialize() {
	if chars := a.GetStringSliceParam("characters"); len(chars) > 0 {
		a.SetCurrent("characters", chars)
	}
	if _, ok := a.GetCurrent("cells"); !ok {
		a.SetCurrent("cells", []AuraCell{})
	}

	// Painted ground is accumulated progress, not configuration — it must
	// survive a restart (e.g. triggered by editing this want's params, such
	// as changing "characters"). Without this, prepareForRestart() wipes
	// "cells" back to its zero value on every restart since it has no
	// declared initialValue to preserve.
	if a.Spec.ResetOnRestart == nil {
		skip := false
		a.Spec.ResetOnRestart = &skip
	}
}

func (a *AuraWant) IsAchieved() bool { return false }

// Progress drains every webhook_queue entry accumulated since the last
// tick and applies each placement in order. Queue-based (AppendState/
// DrainState) rather than the single-slot webhook_payload/
// ConsumeWebhookAction pattern other user-control wants use, because aura
// placement is driven by movement — one webhook per grid cell walked
// through while x is held — and can arrive faster than the ~100ms
// reconcile tick consumes a single slot, silently dropping cells.
func (a *AuraWant) Progress() {
	entries := a.DrainState("webhook_queue")
	if len(entries) == 0 {
		return
	}
	cells := a.loadCells()
	changed := false
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		pm, ok := m["payload"].(map[string]any)
		if !ok {
			continue
		}
		if action, _ := pm["action"].(string); action != "place" {
			continue
		}
		x, ok1 := intFromAny(pm["x"])
		y, ok2 := intFromAny(pm["y"])
		if !ok1 || !ok2 {
			continue
		}
		actingCharacterID, _ := pm["characterId"].(string)
		payloadColor, _ := pm["color"].(string)
		color := a.characterColor(actingCharacterID, payloadColor)
		if color == "" {
			continue
		}
		cells = paintCell(cells, x, y, []string{color})
		cells = fillEnclosedRegions(cells, x, y)
		changed = true
	}
	if changed {
		a.SetCurrent("cells", cells)
	}
}

// characterColor resolves the color to paint with. When "characters" is
// set: prefers actingCharacterID's own color if it's one of the bound
// characters (so a want bound to more than one character paints each in
// its own color depending on who's actually placing), falling back to the
// first bound character when the acting character isn't identified or
// isn't in the list. When unbound: uses payloadColor (the client's
// "default cursorman" color — there's no character record to look one up
// from for an unbound instance).
func (a *AuraWant) characterColor(actingCharacterID, payloadColor string) string {
	raw, ok := a.GetCurrent("characters")
	if ok {
		if chars := stringSliceFromAny(raw); len(chars) > 0 {
			targetID := chars[0]
			if actingCharacterID != "" && containsString(chars, actingCharacterID) {
				targetID = actingCharacterID
			}
			character, ok := GetCharacter(targetID)
			if !ok {
				a.StoreLog("[aura] character %q not found — placement ignored", targetID)
				return ""
			}
			return character.Color
		}
	}
	if payloadColor != "" {
		return payloadColor
	}
	a.StoreLog("[aura] no character bound and no fallback color supplied — placement ignored")
	return ""
}

func (a *AuraWant) loadCells() []AuraCell {
	raw, ok := a.GetCurrent("cells")
	if !ok {
		return nil
	}
	return cellsFromAny(raw)
}

// --- shared helpers (also used by AuraEraseWant) ---

func intFromAny(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(math.Round(n)), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

func stringSliceFromAny(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// cellsFromAny normalizes a "cells" current-state value back into []AuraCell,
// handling both the freshly-set native type and the map-shaped form it takes
// on after a reload from persisted/JSON state.
func cellsFromAny(raw any) []AuraCell {
	switch v := raw.(type) {
	case []AuraCell:
		return v
	case []any:
		out := make([]AuraCell, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			x, _ := intFromAny(m["x"])
			y, _ := intFromAny(m["y"])
			out = append(out, AuraCell{X: x, Y: y, Colors: stringSliceFromAny(m["colors"])})
		}
		return out
	default:
		return nil
	}
}

// paintCell adds each of colors to the cell at (x, y), creating it if
// missing and de-duplicating colors already present there.
func paintCell(cells []AuraCell, x, y int, colors []string) []AuraCell {
	for i := range cells {
		if cells[i].X != x || cells[i].Y != y {
			continue
		}
		for _, color := range colors {
			if !containsString(cells[i].Colors, color) {
				cells[i].Colors = append(cells[i].Colors, color)
			}
		}
		return cells
	}
	deduped := make([]string, 0, len(colors))
	for _, color := range colors {
		if !containsString(deduped, color) {
			deduped = append(deduped, color)
		}
	}
	return append(cells, AuraCell{X: x, Y: y, Colors: deduped})
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

type auraPoint struct{ X, Y int }

// fillEnclosedRegions checks, from the just-painted (originX, originY), each
// unpainted 4-neighbor for whether it belongs to a bounded pocket fully
// walled in by this want's own painted cells. Any such pocket gets painted
// with the same colors as the origin cell.
func fillEnclosedRegions(cells []AuraCell, originX, originY int) []AuraCell {
	fillColors := colorsAt(cells, originX, originY)
	if len(fillColors) == 0 {
		return cells
	}

	occupied := map[auraPoint]bool{}
	minX, minY, maxX, maxY := originX, originY, originX, originY
	for _, c := range cells {
		occupied[auraPoint{c.X, c.Y}] = true
		if c.X < minX {
			minX = c.X
		}
		if c.X > maxX {
			maxX = c.X
		}
		if c.Y < minY {
			minY = c.Y
		}
		if c.Y > maxY {
			maxY = c.Y
		}
	}
	minX--
	minY--
	maxX++
	maxY++

	visited := map[auraPoint]bool{}
	dirs := []auraPoint{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, d := range dirs {
		start := auraPoint{originX + d.X, originY + d.Y}
		if occupied[start] || visited[start] {
			continue
		}

		region := []auraPoint{start}
		visited[start] = true
		queue := []auraPoint{start}
		escaped := start.X <= minX || start.X >= maxX || start.Y <= minY || start.Y >= maxY

		for len(queue) > 0 && !escaped && len(region) <= auraFillCap {
			p := queue[0]
			queue = queue[1:]
			for _, d := range dirs {
				np := auraPoint{p.X + d.X, p.Y + d.Y}
				if occupied[np] || visited[np] {
					continue
				}
				visited[np] = true
				if np.X <= minX || np.X >= maxX || np.Y <= minY || np.Y >= maxY {
					escaped = true
					break
				}
				region = append(region, np)
				queue = append(queue, np)
			}
		}

		if !escaped && len(region) <= auraFillCap {
			for _, p := range region {
				cells = paintCell(cells, p.X, p.Y, fillColors)
				occupied[p] = true
			}
		}
	}
	return cells
}

func colorsAt(cells []AuraCell, x, y int) []string {
	for _, c := range cells {
		if c.X == x && c.Y == y {
			return c.Colors
		}
	}
	return nil
}
