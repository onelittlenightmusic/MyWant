package server

import (
	"sort"
	"strings"
	"time"

	mywant "mywant/engine/core"
)

// A want's constellation: one for the want, not one for each of its answers.
//
// What is joined to a want's answers — the photo of the score card taken at
// the practice a SmartGolf reservation was for — goes into ONE constellation,
// a capture into the want itself (its hub is the want's tile), named after the
// want ("ゴルフ", the type's alias, else the want's name). Which answer each
// member is for is a label on the member, in the constellation's name:
// "constellation-answer/<name>" = the answer's key (answerKey). The answers
// themselves (`<want id>/result/<key>`) are no longer members; the want is,
// as the hub of any capture is one of its members (joinAnswer).
//
// It used to be a constellation per answer, holding the answer and its photos
// — a want played forty times was forty constellations on the board. Those are
// folded into the want's on the way up (migrateAnswerConstellations).

const constellationAnswerPrefix = "constellation-answer/"

func constellationAnswerKey(name string) string { return constellationAnswerPrefix + name }

// wantHub is the constellation a want's answers are joined in: the one
// already captured into the want, else the name it would have.
func (s *Server) wantHub(w *mywant.Want) (name string, existing bool) {
	if n := s.constellationCapturedInto(w.Metadata.ID); n != "" {
		return n, true
	}
	title := w.Metadata.Name
	if a := s.fmTypeAliases()[w.Metadata.Type]; len(a) > 0 && a[0] != "" {
		title = a[0]
	}
	if title == "" {
		title = w.Metadata.ID
	}
	return strings.ReplaceAll(title, "/", "."), false
}

// constellationCapturedInto is a constellation whose hub is the tile (the
// first, by name), or "".
func (s *Server) constellationCapturedInto(tile string) string {
	var names []string
	seen := map[string]bool{}
	for _, labels := range s.thingLabels.All() {
		for k, v := range labels {
			if name := constellationCaptureNameFromKey(k); name != "" && v == tile && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	if s.globalBuilder != nil {
		for _, w := range s.globalBuilder.GetWants() {
			for k, v := range w.Metadata.Labels {
				if name := constellationCaptureNameFromKey(k); name != "" && v == tile && !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return names[0]
}

// wantByID is the want with the id, or nil.
func (s *Server) wantByID(id string) *mywant.Want {
	if s.globalBuilder == nil {
		return nil
	}
	for _, w := range s.globalBuilder.GetWants() {
		if w.Metadata.ID == id {
			return w
		}
	}
	return nil
}

// joinAnswer puts a thing into the want's constellation (named `name`, or the
// want's own when ""), marked as being for the answer `key`. The name it went
// into.
func (s *Server) joinAnswer(wantID, key, thingID, name string) string {
	if name == "" {
		if w := s.wantByID(wantID); w != nil {
			name, _ = s.wantHub(w)
		} else {
			name = wantID
		}
	}
	s.captureInto(name, wantID, thingID)
	// The hub is a member too, as a tile caught into is.
	s.joinWantToHub(wantID, name)
	if key != "" {
		_ = s.thingLabels.Set(thingID, constellationAnswerKey(name), key)
	}
	return name
}

// joinWantToHub makes the want a member of its constellation, once.
func (s *Server) joinWantToHub(wantID, name string) {
	w := s.wantByID(wantID)
	if w == nil || w.Metadata.Labels[constellationKey(name)] != "" {
		return
	}
	s.setConstellationMembership("want", name, wantID, true)
}

// splitAnswerID is an answer id's want and key (`<want id>/result/<key>`).
func splitAnswerID(id string) (wantID, key string, ok bool) {
	i := strings.Index(id, answerSep)
	if i < 0 {
		return "", "", false
	}
	return id[:i], id[i+len(answerSep):], true
}

// answerMembers is, for each answer (`<want id>/result/<key>`), the things
// joined to it: members of a constellation captured into the want, marked
// for that answer — and, from before, members of a constellation the answer
// itself is in.
func (s *Server) answerMembers() map[string][]string {
	all := s.thingLabels.All()
	out := map[string][]string{}
	for id, labels := range all {
		for k, key := range labels {
			name, ok := strings.CutPrefix(k, constellationAnswerPrefix)
			if !ok || key == "" || labels[constellationKey(name)] == "" {
				continue
			}
			if want := labels[constellationCaptureKey(name)]; want != "" {
				answer := want + answerSep + key
				out[answer] = append(out[answer], id)
			}
		}
	}
	// The old way: the answer a member of the same constellation.
	membersOf := map[string][]string{}
	for id, labels := range all {
		for k, v := range labels {
			if v != "" && strings.HasPrefix(k, constellationLabelPrefix) {
				name := strings.TrimPrefix(k, constellationLabelPrefix)
				membersOf[name] = append(membersOf[name], id)
			}
		}
	}
	for _, members := range membersOf {
		for _, a := range members {
			if !strings.Contains(a, answerSep) {
				continue
			}
			for _, m := range members {
				if m != a && !strings.Contains(m, answerSep) {
					out[a] = append(out[a], m)
				}
			}
		}
	}
	for a, ms := range out {
		sort.Strings(ms)
		out[a] = dedupeSorted(ms)
	}
	return out
}

func dedupeSorted(ms []string) []string {
	out := ms[:0]
	for i, m := range ms {
		if i == 0 || m != ms[i-1] {
			out = append(out, m)
		}
	}
	return out
}

// migrateAnswerConstellations folds the constellations made per answer — an
// answer and what was joined to it — into their want's one: each member that
// is not an answer joins the want's constellation marked for that answer, and
// the old constellation goes. Run once the wants are loaded; a constellation
// whose want is gone is left as it is.
func (s *Server) migrateAnswerConstellations() {
	if s.thingLabels == nil {
		return
	}
	all := s.thingLabels.All()
	membersOf := map[string][]string{}
	for id, labels := range all {
		for k, v := range labels {
			if v != "" && strings.HasPrefix(k, constellationLabelPrefix) {
				name := strings.TrimPrefix(k, constellationLabelPrefix)
				membersOf[name] = append(membersOf[name], id)
			}
		}
	}
	names := make([]string, 0, len(membersOf))
	for n := range membersOf {
		names = append(names, n)
	}
	sort.Strings(names)
	moved := 0
	for _, old := range names {
		members := membersOf[old]
		sort.Strings(members)
		var answers []string
		for _, m := range members {
			if strings.Contains(m, answerSep) {
				answers = append(answers, m)
			}
		}
		if len(answers) == 0 {
			continue
		}
		wantID, key, _ := splitAnswerID(answers[0])
		w := s.wantByID(wantID)
		if w == nil {
			continue
		}
		hub, _ := s.wantHub(w)
		if hub == old {
			// Already the want's own name: only the answers leave it, and
			// what stays is marked.
			for _, m := range members {
				if strings.Contains(m, answerSep) {
					s.setConstellationMembership("thing", old, m, false)
				} else if all[m][constellationAnswerKey(old)] == "" {
					_ = s.thingLabels.Set(m, constellationAnswerKey(old), key)
				}
			}
			s.setConstellationCaptureEverywhere(old, wantID)
			moved++
			continue
		}
		color := s.anyConstellationColor(old)
		for _, m := range members {
			if strings.Contains(m, answerSep) {
				continue
			}
			s.joinAnswer(wantID, key, m, hub)
		}
		if color != "" && s.anyConstellationColor(hub) == "" {
			s.setConstellationColorEverywhere(hub, color)
		}
		s.removeArtFile(s.anyConstellationArt(old))
		for _, m := range s.allMembersOfConstellation(old) {
			s.setConstellationMembership(s.memberKindOf(m), old, m, false)
		}
		moved++
	}
	// A want's constellation made before its hub joined it: the want joins.
	for _, labels := range s.thingLabels.All() {
		for k := range labels {
			name, ok := strings.CutPrefix(k, constellationAnswerPrefix)
			if !ok || labels[constellationKey(name)] == "" {
				continue
			}
			if want := labels[constellationCaptureKey(name)]; want != "" && s.wantByID(want) != nil {
				if w := s.wantByID(want); w.Metadata.Labels[constellationKey(name)] == "" {
					s.setConstellationMembership("want", name, want, true)
					moved++
				}
			}
		}
	}
	if moved > 0 {
		boardThingsChanged()
	}
}

// migrateAnswerConstellationsSoon waits for the wants to be loaded, then
// folds (migrateAnswerConstellations).
func (s *Server) migrateAnswerConstellationsSoon() {
	go func() {
		for i := 0; i < 120; i++ {
			if s.globalBuilder != nil && len(s.globalBuilder.GetWants()) > 0 {
				time.Sleep(2 * time.Second) // the rest of them
				s.migrateAnswerConstellations()
				return
			}
			time.Sleep(time.Second)
		}
	}()
}

// joinAnswersIn takes a constellation asked for with one of a want's answers
// among its members — the wire tying a thing to an answer ball, an old client —
// as joining the rest into that want's constellation, marked for the answer:
// the answer itself is not made a member. The constellation they went into, and
// whether there was an answer among them at all.
func (s *Server) joinAnswersIn(members []string) (string, bool) {
	var answers, rest []string
	for _, m := range members {
		if strings.Contains(m, answerSep) {
			answers = append(answers, m)
		} else {
			rest = append(rest, m)
		}
	}
	if len(answers) == 0 {
		return "", false
	}
	// One answer for each of the rest: the first named.
	wantID, key, _ := splitAnswerID(answers[0])
	name := ""
	for _, m := range rest {
		name = s.joinAnswer(wantID, key, m, name)
	}
	if name == "" {
		if w := s.wantByID(wantID); w != nil {
			name, _ = s.wantHub(w)
		}
	}
	boardThingsChanged(append(rest, wantID)...)
	return name, true
}
