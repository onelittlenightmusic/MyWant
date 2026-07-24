package types

import (
	"math"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[PlaceArrivalWant, PlaceArrivalLocals]("place_arrival")
	})
}

// PlaceArrivalWant is the geofence half of a named-place riff: it watches the
// live location want against a place you named (an aura definition) and, the
// moment you cross into that place's radius, fires a reaction want. It is the
// first "trigger" implementation — "「会社」に着いたら〜する" made real — and the
// same shape (watch a signal, fire on a transition) is what source triggers
// will reuse.
//
// It reads the live position through imported global state (here_lat/here_lng,
// which the location want publishes), not by reaching into another want — so
// the reconcile loop's import gate also holds Progress until a position exists.
// It fires only on the outside→inside transition, so it triggers on arrival,
// not continuously while you are there.
type PlaceArrivalLocals struct{}

type PlaceArrivalWant struct{ Want }

func (p *PlaceArrivalWant) GetLocals() *PlaceArrivalLocals {
	return CheckLocalsInitialized[PlaceArrivalLocals](&p.Want)
}

func (p *PlaceArrivalWant) Initialize() {
	if _, ok := p.GetCurrent("inside"); !ok {
		p.SetCurrent("inside", false)
	}
}

// Never "done": a geofence keeps watching for the next arrival.
func (p *PlaceArrivalWant) IsAchieved() bool { return false }

func (p *PlaceArrivalWant) Progress() {
	place := p.GetStringParam("place", "")
	reactionID := p.GetStringParam("reaction_want_id", "")
	radius := p.GetFloatParam("radius_m", 120)
	if place == "" || reactionID == "" {
		return
	}

	placeLat, placeLng, ok := resolvePlaceCoords(place)
	if !ok {
		return // the place hasn't been named (or has no coordinates) yet
	}
	// here_lat/here_lng are imported from the location want's published global
	// state (see spec.imports set at deploy). The reconcile loop's import gate
	// means Progress only runs once these resolve, so they are present here.
	hereLat := GetInternal(p, "here_lat", math.NaN())
	hereLng := GetInternal(p, "here_lng", math.NaN())
	if math.IsNaN(hereLat) || math.IsNaN(hereLng) {
		return
	}

	dist := haversineMeters(placeLat, placeLng, hereLat, hereLng)
	p.SetCurrent("distance_m", math.Round(dist))

	nowInside := dist <= radius
	wasInside, _ := p.GetCurrent("inside")
	p.SetCurrent("inside", nowInside)

	// Fire only on the outside→inside edge: arrival, not presence.
	if nowInside && wasInside != true {
		fireReaction(reactionID)
		// The effect belongs to whoever arrived — the character whose device is
		// reporting this position — so play it on their cursor too, and it
		// animates on every client via the cursor stream (see FireCharacterEffect).
		if effectType, charID := reactionEffectAndArriver(reactionID); effectType != "" && charID != "" {
			FireCharacterEffect(charID, effectType)
		}
		p.StoreLog("[PLACE-ARRIVAL] arrived at %q (%.0fm ≤ %.0fm) — fired %s", place, dist, radius, reactionID)
	}
}

// reactionEffectAndArriver resolves the reaction want's type (the effect to
// play) and the character who arrived — the owner of the location want feeding
// the position. Either may be empty if not resolvable (e.g. a CLI-deployed riff
// with no owning character), in which case the effect still updates state but
// no cursor animates.
func reactionEffectAndArriver(reactionID string) (effectType, characterID string) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return "", ""
	}
	if want, _, ok := cb.FindWantByID(reactionID); ok {
		effectType = want.Metadata.Type
	}
	for _, w := range cb.GetWants() {
		if w.Metadata.Type == "location" {
			if id := w.Metadata.Labels["mywant.io/owner-character"]; id != "" {
				characterID = id
				break
			}
		}
	}
	return effectType, characterID
}

// resolvePlaceCoords looks the named place up in the aura catalog. The naming UI
// tags a value by its subType, so a place can land under a few kinds depending
// on what field it was named from; try the ones that carry coordinates.
func resolvePlaceCoords(name string) (lat, lng float64, ok bool) {
	for _, kind := range []string{"place", "location_coordinate", "location", "coordinate"} {
		def, found := ResolveAuraDefinition(kind, name, "")
		if !found {
			continue
		}
		m, isMap := def.(map[string]any)
		if !isMap {
			continue
		}
		lat = ToFloat64(firstOf(m, "lat", "latitude"), math.NaN())
		lng = ToFloat64(firstOf(m, "lng", "lon", "longitude"), math.NaN())
		if !math.IsNaN(lat) && !math.IsNaN(lng) {
			return lat, lng, true
		}
	}
	return 0, 0, false
}

// fireReaction triggers a reaction want the same way its own webhook would:
// dropping {action:"trigger"} into webhook_payload with a fresh timestamp, which
// the reaction's ConsumeWebhookAction picks up on its next Progress. Nanosecond
// precision guarantees the timestamp differs from the last, so it never dedupes.
func fireReaction(reactionID string) {
	StoreWantState(reactionID, "webhook_payload", map[string]any{"action": "trigger"})
	StoreWantState(reactionID, "webhook_received_at", time.Now().Format(time.RFC3339Nano))
}

func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// haversineMeters returns the great-circle distance between two lat/lng points.
func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371000.0 // meters
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadius * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
