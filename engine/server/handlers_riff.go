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

	name := fmt.Sprintf("riff-%s-%d", p.ReactionType, time.Now().UnixMilli()%100000)
	dto := &ws.Want{
		Metadata: ws.Metadata{
			ID:   generateWantID(),
			Name: name,
			Type: p.ReactionType,
			Labels: map[string]string{
				"origin":            "riff",
				"riff-trigger-kind": p.TriggerKind,
				"riff-trigger-name": p.TriggerName,
			},
		},
		Spec: ws.WantSpec{Params: map[string]any{}},
	}

	runtime := mywant.WantDTOSliceToRuntime([]*ws.Want{dto})
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
	})
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

	// Toys (reactions) and deployed sources come from the want-type registry and
	// the live wants. A want type in the "effect" category is a toy; a deployed
	// want that is not itself an effect (and not world-plumbing) can be a
	// trigger source.
	defs := s.globalBuilder.AllWantTypeDefinitions()
	effectTypes := map[string]bool{}
	for name, def := range defs {
		if def == nil {
			continue
		}
		if def.Metadata.Category == "effect" {
			effectTypes[name] = true
			in.Reactions = append(in.Reactions, mywant.RiffReaction{
				Type:  name,
				Title: def.Metadata.Title,
			})
		}
	}

	for _, want := range s.globalBuilder.GetWants() {
		t := want.Metadata.Type
		// A single noise trigger ("system-scheduler が変わったら…") kills the whole
		// list's charm, so the filter is deliberately strict: framework-managed
		// system wants, world-machinery types, and effects themselves are all out.
		// What survives should read as a real thing in the user's life.
		if t == "" || want.Metadata.IsSystemWant || effectTypes[t] || isRiffPlumbing(t) {
			continue
		}
		in.Sources = append(in.Sources, mywant.RiffSource{Type: t, Name: want.Metadata.Name})
	}
	return in
}

// isRiffPlumbing excludes want types that are structural/world-machinery rather
// than evocative real-world triggers, so riffs stay grounded in things that
// mean something to the user. (System-managed wants are excluded separately via
// Metadata.IsSystemWant; this covers user-deployable-but-not-evocative types.)
func isRiffPlumbing(wantType string) bool {
	switch wantType {
	case "aura", "aura_erase", "wall", "world", "robot", "going", "gear", "direction",
		"note", "timer", "button", "switch", "dynamic_background", "caddy", "cloudflare",
		"ngrok", "localtunnel", "managed_process", "managed_launch", "web_inspector", "replay":
		return true
	}
	return false
}
