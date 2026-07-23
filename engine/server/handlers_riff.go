package server

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	mywant "mywant/engine/core"

	ws "github.com/onelittlenightmusic/want-spec"
)

// getRiffs assembles the raw material for a riff — the things this world has
// NAMED (aura definitions) plus the toys it offers (effect want types) plus its
// deployed source wants — and returns a handful of proposed absurd wirings.
// GET /api/v1/riff?n=5[&seed=123]
//
// This is the minimal generative loop: read the proposals and see whether "it
// uses MY world" already raises a smile. Nothing is deployed here; a proposal
// carries the reaction want type so a later accept step can create the want.
func (s *Server) getRiffs(w http.ResponseWriter, r *http.Request) {
	n := 5
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = parsed
		}
	}
	// Default seed varies by the minute so repeated calls drift; an explicit
	// seed makes the surfacing reproducible.
	seed := time.Now().UnixNano()
	if v := r.URL.Query().Get("seed"); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed = parsed
		}
	}

	in := s.buildRiffInput()
	proposals := mywant.GenerateRiffs(in, n, seed)
	if proposals == nil {
		proposals = []mywant.RiffProposal{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"proposals": proposals,
		"vocabulary": map[string]int{
			"named":     len(in.Named),
			"sources":   len(in.Sources),
			"reactions": len(in.Reactions),
		},
	})
}

// deployRiff turns a proposal into reality — structurally, straight from the
// proposal's fields, never by round-tripping the sentence through an LLM (which
// discards the structure the riff already had). It creates the reaction want
// (the toy); the reaction type is a known want type, so this always works.
//
// Wiring the *trigger* is a separate layer: source triggers can later be bound
// to the reaction, and named-place triggers need a place consumer that does not
// exist yet — so for now the toy is created and the intended trigger is recorded
// as a label, not lost. POST /api/v1/riff/deploy with a RiffProposal body.
func (s *Server) deployRiff(w http.ResponseWriter, r *http.Request) {
	var p mywant.RiffProposal
	if err := DecodeRequest(r, &p); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if p.ReactionType == "" {
		s.JSONError(w, r, http.StatusBadRequest, "reactionType is required", "")
		return
	}

	suffix := time.Now().UnixMilli() % 100000
	reactionID := generateWantID()
	name := fmt.Sprintf("riff-%s-%d", p.ReactionType, suffix)
	dtos := []*ws.Want{{
		Metadata: ws.Metadata{
			ID:   reactionID,
			Name: name,
			Type: p.ReactionType,
			Labels: map[string]string{
				"origin":            "riff",
				"riff-trigger-kind": p.TriggerKind,
				"riff-trigger-name": p.TriggerName,
			},
		},
		Spec: ws.WantSpec{Params: map[string]any{}},
	}}

	// A named-place trigger ("「会社」に着いたら…") gets a real geofence: a
	// place_arrival want that watches the live location against that named place
	// and fires this reaction on arrival. Source triggers don't wire yet, so
	// they deploy the reaction alone (the intent stays on the reaction's labels).
	wired := false
	if p.TriggerKind == "named" && p.TriggerName != "" {
		dtos = append(dtos, &ws.Want{
			Metadata: ws.Metadata{
				ID:   generateWantID(),
				Name: fmt.Sprintf("arrival-%s-%d", p.TriggerName, suffix),
				Type: "place_arrival",
				Labels: map[string]string{
					"origin":            "riff",
					"riff-trigger-name": p.TriggerName,
				},
			},
			Spec: ws.WantSpec{
				Params: map[string]any{
					"place":            p.TriggerName,
					"reaction_want_id": reactionID,
				},
				// Import the device position the location want publishes to
				// global state, rather than reading its want state directly.
				Imports: map[string]string{
					"device_lat": "here_lat",
					"device_lng": "here_lng",
				},
			},
		})
		// Making the two ends meet is riff's job: the geofence imports
		// device_lat/device_lng, so ensure the location want actually exposes
		// them. Doing this by hand every time is exactly the friction riff
		// exists to remove.
		s.ensureDeviceLocationExposes()
		wired = true
	}

	runtime := mywant.WantDTOSliceToRuntime(dtos)
	if err := s.validateWantTypes(runtime); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Unknown reaction want type", err.Error())
		return
	}
	ids, err := s.globalBuilder.AddWantsAsyncWithTracking(runtime)
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to deploy riff", err.Error())
		return
	}

	wantID := ""
	if len(ids) > 0 {
		wantID = ids[0]
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"wantId": wantID,
		"name":   name,
		"text":   p.Text,
		"wired":  wired, // true = trigger geofence deployed; false = reaction only
	})
}

// ensureDeviceLocationExposes makes every location want publish its lat/lng to
// the global keys place_arrival imports (device_lat/device_lng), adding the
// exposes and re-registering if they are missing. Idempotent: a location that
// already exposes them is left alone. This is what lets a place riff "just
// work" without the user hand-wiring expose/import.
func (s *Server) ensureDeviceLocationExposes() {
	want := map[string]string{"lat": "device_lat", "lng": "device_lng"}
	for _, w := range s.globalBuilder.GetWants() {
		if w.Metadata.Type != "location" {
			continue
		}
		have := map[string]bool{}
		for _, e := range w.Spec.Exposes {
			if want[e.CurrentState] == e.As {
				have[e.CurrentState] = true
			}
		}
		exposes := w.Spec.Exposes
		changed := false
		for local, global := range want {
			if !have[local] {
				exposes = append(exposes, ws.ExposeEntry{CurrentState: local, As: global})
				changed = true
			}
		}
		if !changed {
			continue
		}
		cfg := &mywant.Want{Metadata: w.Metadata, Spec: w.Spec}
		cfg.Spec.Exposes = exposes
		s.globalBuilder.UpdateWant(cfg)
	}
}

// buildRiffInput gathers named things, deployed source wants, and effect want
// types from live state. Kept out of the handler so it can grow (filtering,
// per-character vocabulary) without touching the HTTP plumbing.
func (s *Server) buildRiffInput() mywant.RiffInput {
	in := mywant.RiffInput{}

	// Named vocabulary: every catalog definition anyone has made.
	for _, mark := range mywant.AllAuraDefinitions() {
		in.Named = append(in.Named, mywant.RiffNamedThing{
			Kind: mark.Target.Kind,
			Name: mark.Target.Name,
		})
	}

	// Reactions and sources are opt-in: a want type declares its riff role with
	// a `riffable` label rather than being guessed from category or excluded by a
	// blacklist. This is what keeps a nonsensical pairing (arrive → aura, which
	// is a persistent painter, not a one-shot effect) or a noise trigger
	// (gauge/claude-info) out — a type only appears if it named itself fit.
	//   riffable: reaction — a one-shot effect the world can perform on trigger
	//   riffable: source   — an evocative thing whose change makes a good trigger
	defs := s.globalBuilder.AllWantTypeDefinitions()
	sourceType := map[string]bool{}
	for name, def := range defs {
		if def == nil {
			continue
		}
		switch def.Metadata.Labels["riffable"] {
		case "reaction":
			in.Reactions = append(in.Reactions, mywant.RiffReaction{Type: name, Title: def.Metadata.Title})
		case "source":
			sourceType[name] = true
		}
	}

	for _, want := range s.globalBuilder.GetWants() {
		if sourceType[want.Metadata.Type] {
			in.Sources = append(in.Sources, mywant.RiffSource{Type: want.Metadata.Type, Name: want.Metadata.Name})
		}
	}
	return in
}
