package server

// character_timings.go — when what happens to a character is worth telling.
//
// A want type names the moments of its answers worth telling (its timings:
// core/result_timings.go). A character has them too (want-spec's Character:
// spec.timings), read off the character's own state (want-spec's
// CharacterState*): how they feel, what they stand on, what they heard last,
// and — as they come back from an errand (errand.go) — how it went. Each is
// read when something happens to them: they hear something
// (character_memory.go), come back from an errand, step onto a want
// (story_robot.go). A timing that holds is kept in their story
// (character.timing), murmured in their bubble, and — notify — told as a
// notice from them, the same notice a want's is (notices.go): the iPhone
// shows it as its own notification, and their sheet's 通知 lists it.
//
// Every character has the standard timings (want-spec's
// CharacterStandardTimings: an errand that found other than expected, told;
// a mood turned uneasy, murmured) unless their spec names one the same.
//
// A character as a document — want-spec's Character, apiVersion mywant.io/v1:
//
//	GET  /api/v1/characters/{id}?format=yaml   the character, as one
//	POST /api/v1/characters/apply              one applied (YAML or JSON)

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	mywant "mywant/engine/core"

	ws "github.com/onelittlenightmusic/want-spec"
	"gopkg.in/yaml.v3"
)

const storyKindCharacterTiming = "character.timing"

// What each character's watched fields were when last read, by character.
var (
	characterSeenMu sync.Mutex
	characterSeen   = map[string]map[string]any{}
)

// characterState is a character's state now — the fields their timings
// read — with what happened just now laid over it (an errand's verdict).
func (s *Server) characterState(c mywant.Character, now map[string]any) map[string]any {
	m := memories.get(c.ID)
	state := map[string]any{
		ws.CharacterStateMood:            m.Mood,
		ws.CharacterStateIntensity:       m.Intensity,
		ws.CharacterStateMoodUneasy:      m.Mood == "worried" || m.Mood == "sad",
		ws.CharacterStateErrandVerdict:   "",
		ws.CharacterStateErrandDifferent: false,
	}
	if place, near := s.placeOf(c); place != "" && !near {
		state[ws.CharacterStatePlace] = place
	} else {
		state[ws.CharacterStatePlace] = ""
	}
	if len(m.Memories) > 0 {
		state[ws.CharacterStateHeard] = m.Memories[0].Say
	}
	for k, v := range now {
		state[k] = v
	}
	return state
}

// characterAbout is the want a moment is about: one named with it, else the
// one they heard from last.
type characterAbout struct {
	wantID, wantName, wantType string
}

// characterMoment reads a character's timings now that something happened to
// them, and tells of those that hold. now: what just happened (an errand's
// fields); about: the want it was about, when known.
func (s *Server) characterMoment(id string, now map[string]any, about *characterAbout) {
	c, ok := mywant.GetCharacter(id)
	if !ok {
		return
	}
	defs := ws.CharacterTimings(c.Timings)
	state := s.characterState(*c, now)
	characterSeenMu.Lock()
	seen := characterSeen[id]
	if seen == nil {
		seen = map[string]any{}
		characterSeen[id] = seen
	}
	held := mywant.TimingsAt(defs, seen, func(f string) any { return state[f] })
	characterSeenMu.Unlock()
	if len(held) == 0 {
		return
	}
	if about == nil {
		if m := memories.get(id).Memories; len(m) > 0 {
			about = &characterAbout{wantID: m[0].WantID, wantName: m[0].WantName, wantType: m[0].WantType}
		}
	}
	for _, name := range held {
		for _, d := range defs {
			if d.Name == name {
				s.tellCharacterTiming(*c, d, state, about)
				break
			}
		}
	}
}

// tellCharacterTiming tells of one timing that held: in their story, their
// bubble (murmur) and a notice from them (notify).
func (s *Server) tellCharacterTiming(c mywant.Character, d ws.TimingDef, state map[string]any, about *characterAbout) {
	text := characterTimingText(d, state)
	detail := map[string]string{"timing": d.Name, "said": text}
	if d.Notify {
		detail["notify"] = "true"
	}
	ev := ws.StoryEvent{Character: c.ID, Kind: storyKindCharacterTiming, Detail: detail}
	if about != nil && about.wantID != "" {
		ev.To = s.storyWant(about.wantID)
	}
	story.add(ev)
	log.Printf("[character-timing] %s %s: %s", c.Name, d.Name, firstLine(text, 120))
	if d.Murmur {
		if isRobotCharacter(c) {
			_ = s.sayBody(bodyOf(c.ID), text, speechNotice)
		} else {
			recordSpeechAs(c.ID, text, "say", speechNotice)
		}
	}
	if d.Notify {
		s.tellCharacterNotice(c, d, text, about)
	}
}

// characterTimingText is what is said of a timing: an errand's report as it
// was said; else the timing's description with what they heard last.
func characterTimingText(d ws.TimingDef, state map[string]any) string {
	if r, _ := state[ws.CharacterStateErrandReport].(string); r != "" && d.If == ws.CharacterStateErrandDifferent {
		return r
	}
	what := d.Description
	if what == "" {
		what = d.Name
	}
	// A timing about where they are says where: 「別の場所に来た：戸塚の天気」.
	if d.OnChange == ws.CharacterStatePlace || d.If == ws.CharacterStatePlace {
		if place, _ := state[ws.CharacterStatePlace].(string); place != "" {
			return fmt.Sprintf("%s：%s", what, place)
		}
	}
	if heard, _ := state[ws.CharacterStateHeard].(string); heard != "" {
		return fmt.Sprintf("%s：「%s」", what, heard)
	}
	return what
}

// tellCharacterNotice is a notice from a character — kept to be rated, and
// told to every open page (the iPhone shows it as its own notification), as
// a want's is.
func (s *Server) tellCharacterNotice(c mywant.Character, d ws.TimingDef, text string, about *characterAbout) {
	id := "notif-" + time.Now().UTC().Format("20060102T150405.000000000")
	// Their name alone: their face is the notice's picture (their avatar,
	// drawn by the device), not a character in its title.
	title := c.Name
	labels := map[string]string{"reason": "character-timing", "timing": d.Name, "character": c.ID}
	rec := noticeRecord{ID: id, Character: c.ID, Text: text, Moment: d.Description, Timings: []string{d.Name}, Labels: labels}
	n := wantNotice{ID: id, WantType: "character:" + c.ID, Title: title, Body: text, Character: c.ID,
		Avatar: c.Avatar, Color: c.Color,
		Off: mywant.GetDeviceStore().Settings().NoticesOff, Labels: labels}
	if about != nil {
		rec.WantID, rec.Want, rec.WantType = about.wantID, about.wantName, about.wantType
		n.WantID = about.wantID
	}
	noteNotice(rec)
	broadcastSSE(noticeEvent, n)
}

// ── A character as a document ───────────────────────────────────────────────

// characterDocument is a character as want-spec's Character.
func characterDocument(c mywant.Character) ws.Character {
	return ws.Character{
		APIVersion: "mywant.io/v1", Kind: ws.CharacterKind,
		Metadata: ws.CharacterMetadata{ID: c.ID, Name: c.Name, Labels: c.Labels},
		Spec:     ws.CharacterSpec{Avatar: c.Avatar, Color: c.Color, Shape: c.Shape, Timings: c.Timings},
	}
}

// writeCharacterDocument answers with the character as a YAML document.
func writeCharacterDocument(w http.ResponseWriter, c mywant.Character) {
	out, err := yaml.Marshal(characterDocument(c))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(out)
}

// applyCharacter handles POST /api/v1/characters/apply: a Character document
// laid on the character it names (by id, else by name), or made. Its labels
// are added; its timings replace theirs; what it leaves out is left as it is.
func (s *Server) applyCharacter(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	var doc ws.Character
	// YAML reads JSON too.
	if err := yaml.Unmarshal(data, &doc); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid Character document", err.Error())
		return
	}
	if doc.Kind != "" && doc.Kind != ws.CharacterKind {
		s.JSONError(w, r, http.StatusBadRequest, "Not a Character", "kind is "+doc.Kind)
		return
	}
	if doc.Metadata.ID == "" && doc.Metadata.Name == "" {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid Character document", "metadata.id or metadata.name is required")
		return
	}
	var found *mywant.Character
	if doc.Metadata.ID != "" {
		found, _ = mywant.GetCharacter(doc.Metadata.ID)
	}
	if found == nil && doc.Metadata.Name != "" {
		for _, c := range mywant.ListCharacters() {
			if c.Name == doc.Metadata.Name {
				c := c
				found = &c
				break
			}
		}
	}
	var id string
	if found == nil {
		made := mywant.AddCharacter(mywant.Character{Name: doc.Metadata.Name, Avatar: doc.Spec.Avatar, Color: doc.Spec.Color, Shape: doc.Spec.Shape})
		id = made.ID
		s.ensureCharacterChatWant(id)
	} else {
		id = found.ID
		next := *found
		if doc.Metadata.Name != "" {
			next.Name = doc.Metadata.Name
		}
		if doc.Spec.Avatar != "" {
			next.Avatar = doc.Spec.Avatar
		}
		if doc.Spec.Color != "" {
			next.Color = doc.Spec.Color
		}
		if doc.Spec.Shape != "" {
			next.Shape = doc.Spec.Shape
		}
		mywant.UpdateCharacter(id, next)
	}
	robotBefore := false
	if c, ok := mywant.GetCharacter(id); ok {
		robotBefore = mywant.IsRobotCharacter(*c)
	}
	for k, v := range doc.Metadata.Labels {
		mywant.SetCharacterLabel(id, k, v)
	}
	if doc.Spec.Timings != nil {
		mywant.SetCharacterTimings(id, doc.Spec.Timings)
	}
	c, _ := mywant.GetCharacter(id)
	if id != robotCharacterID && mywant.IsRobotCharacter(*c) != robotBefore {
		s.ensureCharacterChatWant(id) // a robot now, or a person again
	}
	go broadcastSSE("character_changed", id)
	if strings.Contains(r.URL.Query().Get("format"), "yaml") || strings.Contains(r.Header.Get("Accept"), "yaml") {
		writeCharacterDocument(w, *c)
		return
	}
	s.JSONResponse(w, http.StatusOK, c)
}
