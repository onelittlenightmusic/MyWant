package server

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// What a want set down near others could hear of them, and they of it — the
// board's Hear (a want's own spec.hear, chosen from these): for each of the
// wants near it, both ways, every timing the speaker murmurs at, with what the
// hearer does with it (its type's hearing). The board says which wants are
// near; a timing already heard is left out.

type hearEnd struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type hearCandidate struct {
	Hearer  hearEnd `json:"hearer"`
	Speaker hearEnd `json:"speaker"`
	// Timing is the speaker's type's timing; TimingDescription says it
	// (「全部終わったとき」).
	Timing            string `json:"timing"`
	TimingDescription string `json:"timing_description,omitempty"`
	TimingIcon        string `json:"timing_icon,omitempty"`
	// Kind is how the hearer answers (say, propose, act), Action what it does.
	Kind   string `json:"kind"`
	Action string `json:"action"`
}

// hearCandidates handles GET /api/v1/wants/{id}/hear-candidates?peers=a,b.
func (s *Server) hearCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.handleOptions(w, r)
		return
	}
	self := s.findWantByIDInAll(mux.Vars(r)["id"])
	if self == nil {
		s.JSONError(w, r, http.StatusNotFound, "want not found", mux.Vars(r)["id"])
		return
	}
	out := []hearCandidate{}
	for _, id := range strings.Split(r.URL.Query().Get("peers"), ",") {
		id = strings.TrimSpace(id)
		if id == "" || id == self.Metadata.ID {
			continue
		}
		peer := s.findWantByIDInAll(id)
		if peer == nil {
			continue
		}
		// It hearing them first: it is the one just set down.
		out = append(out, hearCandidatesOf(self, peer)...)
		out = append(out, hearCandidatesOf(peer, self)...)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"candidates": out})
}

// hearCandidatesOf is what hearer could hear of speaker: each timing the
// speaker murmurs at that the hearer does not hear yet.
func hearCandidatesOf(hearer, speaker *mywant.Want) []hearCandidate {
	kind, action := hearingOf(hearer)
	var out []hearCandidate
	for _, t := range speaker.MurmurTimings() {
		if alreadyHears(hearer, speaker.Metadata.Type, t.Name) {
			continue
		}
		out = append(out, hearCandidate{
			Hearer:            endOf(hearer),
			Speaker:           endOf(speaker),
			Timing:            t.Name,
			TimingDescription: t.Description,
			TimingIcon:        t.Icon,
			Kind:              kind,
			Action:            action,
		})
	}
	return out
}

// hearingOf is what a want's type does with what it heard; a type that says
// nothing answers in a word.
func hearingOf(w *mywant.Want) (kind, action string) {
	if def := w.WantTypeDefinition; def != nil && def.Hearing != nil {
		kind, action = def.Hearing.Kind, def.Hearing.Description
	}
	if kind == "" {
		kind = "say"
	}
	if action == "" {
		action = "ひとこと返す"
	}
	return kind, action
}

// alreadyHears: the want hears that type at that timing already.
func alreadyHears(w *mywant.Want, typ, timing string) bool {
	for _, h := range w.GetSpec().Hear {
		if h.Type == typ && hearsAt(h.Timings, []string{timing}) {
			return true
		}
	}
	return false
}

func endOf(w *mywant.Want) hearEnd {
	return hearEnd{ID: w.Metadata.ID, Name: w.Metadata.Name, Type: w.Metadata.Type}
}
