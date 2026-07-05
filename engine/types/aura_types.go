package types

import (
	"encoding/json"
	"math"
	"strconv"

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
	if _, ok := a.GetCurrent("active"); !ok {
		a.SetCurrent("active", false)
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
		action, _ := pm["action"].(string)
		switch action {
		case "activate":
			a.SetCurrent("active", true)
			// Recompute targets fresh rather than trusting the last-stored value:
			// "targets" is otherwise only refreshed by this want's own placements,
			// so a target want that moved (or was created) into already-painted
			// territory since the last placement would otherwise be missed here.
			a.updateTargets(a.loadCells())
			a.applyAuraDefaults()
		case "deactivate":
			a.SetCurrent("active", false)
		case "place":
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
	}
	if changed {
		a.SetCurrent("cells", cells)
		a.updateTargets(cells)
	}
}

// updateTargets recomputes "targets" — every other want whose canvas
// footprint overlaps one of this aura's painted cells — and derives this
// want's achieving_percentage as the average of those targets' own
// achieving_percentage (e.g. one target at 100% + one at 50% = 75%). With no
// targets, achieving_percentage is 0.
func (a *AuraWant) updateTargets(cells []AuraCell) {
	painted := map[auraPoint]bool{}
	for _, c := range cells {
		painted[auraPoint{c.X, c.Y}] = true
	}

	cb := GetGlobalChainBuilder()
	if cb == nil {
		return
	}
	selfID := a.Metadata.ID
	targetIDs := make([]string, 0)
	sum := 0.0
	for _, w := range cb.GetWants() {
		if w.Metadata.ID == selfID {
			continue
		}
		onAura := false
		for _, fp := range wantFootprint(w) {
			if painted[auraPoint{fp[0], fp[1]}] {
				onAura = true
				break
			}
		}
		if !onAura {
			continue
		}
		targetIDs = append(targetIDs, w.Metadata.ID)
		if raw, ok := w.GetCurrent("achieving_percentage"); ok {
			if pct, ok := floatFromAny(raw); ok {
				sum += pct
			}
		}
	}

	a.SetCurrent("targets", targetIDs)
	if len(targetIDs) == 0 {
		a.SetCurrent("achieving_percentage", 0.0)
		return
	}
	a.SetCurrent("achieving_percentage", sum/float64(len(targetIDs)))
}

// applyAuraDefaults selects/toggles each target want's aura-default value —
// the option or on/off state one of this aura's bound characters marked (via
// the x/X dog-ear mark on the choice/going/switch card) as their default for
// that want — the moment the aura is activated. Lets a saved per-character
// preference apply automatically to every want the aura currently covers.
func (a *AuraWant) applyAuraDefaults() {
	raw, ok := a.GetCurrent("characters")
	if !ok {
		return
	}
	chars := stringSliceFromAny(raw)
	if len(chars) == 0 {
		return
	}

	targetsRaw, _ := a.GetCurrent("targets")
	targetIDs := stringSliceFromAny(targetsRaw)
	if len(targetIDs) == 0 {
		return
	}

	cb := GetGlobalChainBuilder()
	if cb == nil {
		return
	}
	for _, targetID := range targetIDs {
		mark, ok := auraDefaultFor(chars, targetID)
		if !ok {
			continue
		}
		want, _, found := cb.FindWantByID(targetID)
		if !found {
			continue
		}
		applyAuraDefaultToWant(cb, want, mark)
	}
}

// auraDefaultFor returns the aura-default mark the first (in bind order) of
// characterIDs has set for targetID — an aura want can be bound to more than
// one character (see characterColor's actingCharacterID handling), and any of
// them may hold the relevant mark, not just the first-bound one.
func auraDefaultFor(characterIDs []string, targetID string) (AuraMark, bool) {
	for _, id := range characterIDs {
		character, ok := GetCharacter(id)
		if !ok {
			continue
		}
		if mark, ok := character.AuraDefaults[targetID]; ok {
			return mark, true
		}
	}
	return AuraMark{}, false
}

// AuraDefaultApplier is an optional extension a want type implements when it
// needs to validate or transform an aura-default mark itself instead of the
// generic apply-by-section/key path (e.g. "choice" must check the value is
// still one of its current options). Returning true means "handled" —
// implementing this interface opts a want type fully out of the generic path.
type AuraDefaultApplier interface {
	ApplyAuraDefault(section, key, value string) bool
}

// applyAuraDefaultToWant applies one aura-default mark to the target want.
// Want types that need their own validation/transform (see choice_types.go's
// ChoiceWant.ApplyAuraDefault) opt out of the generic path by implementing
// AuraDefaultApplier; everything else falls through to applyAuraDefaultGeneric.
func applyAuraDefaultToWant(cb *ChainBuilder, want *Want, mark AuraMark) {
	if fn, ok := cb.FindWantFunctionByID(want.Metadata.ID); ok {
		if applier, ok := fn.(AuraDefaultApplier); ok {
			applier.ApplyAuraDefault(mark.Section, mark.Key, mark.Value)
			return
		}
	}
	applyAuraDefaultGeneric(want, mark)
}

// applyAuraDefaultGeneric writes mark.Value into the section/key the mark
// declares, converting the string to match whatever type is already stored
// there (or already declared as a parameter) — no want-type knowledge needed.
func applyAuraDefaultGeneric(want *Want, mark AuraMark) {
	if mark.Section == "parameter" {
		existing, _ := want.GetParameter(mark.Key)
		want.UpdateParameter(mark.Key, convertAuraValue(existing, mark.Value))
		return
	}
	var existing any
	switch mark.Section {
	case "current":
		existing, _ = want.GetCurrent(mark.Key)
		want.SetCurrent(mark.Key, convertAuraValue(existing, mark.Value))
	case "plan":
		existing, _ = want.GetPlan(mark.Key)
		want.SetPlan(mark.Key, convertAuraValue(existing, mark.Value))
	case "goal":
		existing, _ = want.GetGoal(mark.Key)
		want.SetGoal(mark.Key, convertAuraValue(existing, mark.Value))
	case "internal":
		existing, _ = want.GetInternal(mark.Key)
		want.SetInternal(mark.Key, convertAuraValue(existing, mark.Value))
	}
}

// convertAuraValue converts an aura mark's string form back to the Go type
// already stored at the target key, mirroring the frontend's getChoiceValue
// encoding (object values JSON-stringified, everything else via String()).
func convertAuraValue(existing any, raw string) any {
	switch existing.(type) {
	case bool:
		return ToBool(raw, false)
	case float64, int:
		return ToFloat64(raw, 0)
	case map[string]any, []any:
		var decoded any
		if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
			return decoded
		}
	}
	return raw
}

// wantFootprint returns every canvas grid cell a want occupies, including
// multi-cell spans, from its mywant.io/canvas-x/y/rotation/length labels.
// Mirrors the server package's CanvasCoordinateHook tile-footprint logic
// (unexported there); duplicated here since engine/types cannot import
// engine/server (server already imports types).
func wantFootprint(w *Want) [][2]int {
	if w.Metadata.Labels == nil {
		return nil
	}
	x, errX := strconv.Atoi(w.Metadata.Labels["mywant.io/canvas-x"])
	y, errY := strconv.Atoi(w.Metadata.Labels["mywant.io/canvas-y"])
	if errX != nil || errY != nil {
		return nil
	}
	rotation, _ := strconv.Atoi(w.Metadata.Labels["mywant.io/canvas-rotation"])
	length, _ := strconv.Atoi(w.Metadata.Labels["mywant.io/canvas-length"])
	span := length + 1
	cells := make([][2]int, span)
	for i := range span {
		switch rotation {
		case 90:
			cells[i] = [2]int{x, y + i}
		case 180:
			cells[i] = [2]int{x - i, y}
		case 270:
			cells[i] = [2]int{x, y - i}
		default:
			cells[i] = [2]int{x + i, y}
		}
	}
	return cells
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

func floatFromAny(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
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
