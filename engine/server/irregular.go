package server

// The moments out of the ordinary — a train late, a reservation changed or
// taken away, a backup failed — as the types say them (a timing's irregular).
//
// An irregular moment a want hears (spec.hear) is attended: that want answers
// it as its type does, and the robot leaves it be. One nobody hears is the
// robot's concern — not to deal with every time itself, but to see that some
// want attends to it. What it needs done its timing says (requires:
// reschedule, notify …); the capabilities that meet that (attends), with the
// wants to ask — or none, and one to be made — are what the robot would
// propose. Neither names the other's type, and where they are on the board
// does not matter. Each irregular moment is said in the robot's chat
// (irregular: attended by whom, or the attendants it could have) and kept,
// one JSON line, in ~/.mywant/irregular.jsonl. The proposal itself is to
// come.

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	mywant "mywant/engine/core"
)

type irregularRecord struct {
	At       string         `json:"at"`
	WantID   string         `json:"want_id"`
	Want     string         `json:"want"`
	WantType string         `json:"want_type"`
	Timings  []string       `json:"timings"`
	Needs    []string       `json:"needs,omitempty"` // what its timings require
	Result   map[string]any `json:"result,omitempty"`
	// HeardBy: the wants that attend to it (they hear it). None, and
	// Attendants are the wants that could.
	HeardBy    []string             `json:"heard_by,omitempty"`
	Attendants []irregularAttendant `json:"attendants,omitempty"`
}

// irregularHearers are the wants that hear src at one of these timings, as a
// murmur is delivered (hearMurmur): of its type, at the timing, and src among
// the nearest they hear.
func (s *Server) irregularHearers(src *mywant.Want, timings []string) []string {
	if s.globalBuilder == nil {
		return nil
	}
	wants := s.globalBuilder.GetWants()
	var out []string
	for _, w := range wants {
		if w.Metadata.ID == src.Metadata.ID {
			continue
		}
		for _, h := range w.GetSpec().Hear {
			if h.Type == src.Metadata.Type && hearsAt(h.Timings, timings) && amongNearest(src, w, wants, src.Metadata.Type, h.Nearest) {
				out = append(out, w.Metadata.Name)
				break
			}
		}
	}
	return out
}

// irregularAttendant is what could attend to an irregular moment: a
// capability that meets what it needs (attends against its timing's
// requires), and the wants to ask — none, and one is to be made.
type irregularAttendant struct {
	Type        string   `json:"type"`
	Capability  string   `json:"capability"`
	Description string   `json:"description"`
	Meets       []string `json:"meets"` // what of the moment's needs it meets
	Wants       []string `json:"wants,omitempty"`
}

func (a irregularAttendant) String() string {
	on := "new"
	if len(a.Wants) > 0 {
		on = strings.Join(a.Wants, "|")
	}
	return a.Type + "#" + a.Capability + "[" + strings.Join(a.Meets, "+") + "](" + on + ")"
}

// irregularNeeds is what src's irregular timings need done (their
// requires), each once, in the order said.
func irregularNeeds(src *mywant.Want, timings []string) []string {
	def := src.WantTypeDefinition
	if def == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, name := range timings {
		for _, t := range def.Timings {
			if t.Name != name {
				continue
			}
			for _, r := range t.Requires {
				if r = strings.TrimSpace(r); r != "" && !seen[r] {
					seen[r] = true
					out = append(out, r)
				}
			}
		}
	}
	return out
}

// meets is what of needs a capability attends to.
func meets(attends, needs []string) []string {
	var out []string
	for _, n := range needs {
		for _, a := range attends {
			if strings.TrimSpace(a) == n {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

// irregularAttendants are the capabilities that meet what src's irregular
// timings need (their attends against the timings' requires): every want's
// own (restart: retry) and its type's on src itself first, then other types'
// — on the wants of the type on the board, or one to be made.
func (s *Server) irregularAttendants(src *mywant.Want, needs []string) []irregularAttendant {
	if len(needs) == 0 {
		return nil
	}
	var own, others []irregularAttendant
	for _, c := range stdCaps {
		if m := meets(c.Attends, needs); len(m) > 0 {
			own = append(own, irregularAttendant{Type: src.Metadata.Type, Capability: c.Name, Description: c.Description, Meets: m, Wants: []string{src.Metadata.Name}})
		}
	}
	types, _ := s.robotWantTypes()
	for _, t := range types {
		def := s.typeDefinition(t.Name)
		if def == nil {
			continue
		}
		for _, c := range def.Capabilities {
			m := meets(c.Attends, needs)
			if len(m) == 0 {
				continue
			}
			a := irregularAttendant{Type: t.Name, Capability: c.Name, Description: c.Description, Meets: m}
			if t.Name == src.Metadata.Type {
				// Its own type's: the want the moment is about, not the others
				// of its kind (a route late is searched again; the other routes
				// are not late).
				a.Wants = []string{src.Metadata.Name}
				own = append(own, a)
				continue
			}
			if s.globalBuilder != nil {
				for _, w := range s.globalBuilder.GetWants() {
					if w.Metadata.Type == t.Name && w.GetLabels()[robotArchived] != "true" {
						a.Wants = append(a.Wants, w.Metadata.Name)
					}
				}
			}
			others = append(others, a)
		}
	}
	return append(own, others...)
}

// noteIrregular says an irregular moment in the robot's chat and keeps it.
func (s *Server) noteIrregular(w *mywant.Want, timings []string, out mywant.ResultHistoryEntry) {
	if w == nil || len(timings) == 0 {
		return
	}
	rec := irregularRecord{
		At: time.Now().Format(time.RFC3339), WantID: w.Metadata.ID, Want: w.Metadata.Name,
		WantType: w.Metadata.Type, Timings: timings,
	}
	rec.Needs = irregularNeeds(w, timings)
	fields := map[string]any{"want": w.Metadata.Name, "type": w.Metadata.Type, "timing": strings.Join(timings, ","), "needs": strings.Join(rec.Needs, ",")}
	if rec.HeardBy = s.irregularHearers(w, timings); len(rec.HeardBy) > 0 {
		fields["heard_by"] = strings.Join(rec.HeardBy, ",")
	} else {
		// Nobody attends to it: the wants that could.
		rec.Attendants = s.irregularAttendants(w, rec.Needs)
		fields["unattended"] = "yes"
		var could []string
		for _, a := range rec.Attendants {
			could = append(could, a.String())
		}
		fields["could_attend"] = strings.Join(could, ", ")
	}
	s.robotChatTrace("irregular", fields, []string{"want", "type", "timing", "needs", "heard_by", "unattended", "could_attend"})
	if m, ok := out.Result.(map[string]any); ok {
		rec.Result = m
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(initiativeHomeFile("irregular.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(line, '\n'))
}
