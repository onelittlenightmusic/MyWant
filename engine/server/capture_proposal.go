package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	mywant "mywant/engine/core"
)

// Capturing into a whole want: which of its answers a capture belongs with,
// proposed once the capture is read, and joined when the person says yes.
//
// The capture skill aimed at a want's answer (a history dot) names the
// constellation first, and everything caught goes straight in. Aimed at the
// whole want, it names nothing: a score card caught into the SmartGolf want
// belongs with ONE of its reservations — the one it was played on — and that
// is known only once the card is read. So a capture into a want is saved and
// pinned, read (its words, thing_picture_text.go; its fields by the want's
// type, photo_fields.go), and then the server proposes: this reservation,
// in this constellation — the one the reservation is already in, or a new
// one named after it. The board asks (CaptureProposalBubble); yes joins the
// two, no leaves the capture pinned on its own.
//
// Which answer: the one whose time is nearest before the capture's — the
// type's photo-match field when it names one (smartgolf: played_at, when the
// card says the round was played), else when the photo was taken, else when
// the thing was saved — within half a day; failing that, the nearest at all.

const (
	// On a captured thing: the want it was captured into, waiting to be read
	// and proposed.
	capturePendingLabel = "capture-pending-want"
	// On it, once read: the proposal, as JSON (captureProposal).
	captureProposalLabel = "capture-proposal"
	// A want type's field its photos are matched to answers by.
	photoMatchLabel = "photo-match"
	// On a captured thing: until when whatever captured it asks about it
	// itself (the extension's copy offer, waiting for the proposal) — the
	// board leaves it alone till then (RFC3339).
	captureAskedUntilLabel = "capture-asked-until"
)

// captureAskWait is how long a sender that asks about its captures itself
// keeps the board from asking — a photo is read, then proposed, meanwhile.
const captureAskWait = 5 * time.Minute

// captureProposal is what the board asks about one capture.
type captureProposal struct {
	Thing      string `json:"thing"`
	ThingLabel string `json:"thingLabel"`
	Want       string `json:"want"`
	WantName   string `json:"wantName"`
	// The answer proposed: its board id, and what it is for a person.
	Answer      string `json:"answer"`
	AnswerLabel string `json:"answerLabel"`
	// The constellation proposed, and whether it is one the answer is in.
	Constellation string `json:"constellation"`
	Existing      bool   `json:"existing"`
	// Why this answer: the capture's own time, as read.
	By string `json:"by,omitempty"`
	// Until when whatever captured it asks about it itself (RFC3339): the
	// board waits till then.
	AskedUntil string `json:"askedUntil,omitempty"`
}

// captureThing captures a thing into the target — the one way a capture is
// made, whether the thing was just shared (/things/share) or is on the board
// already (POST /capture/things/{thing}): into a whole want, read and then
// proposed one of its answers; into a thing or an answer, joined in the
// constellation it is in, else proposed it; into a named constellation,
// added. A proposal made at once is returned. With `ask`, the sender asks
// about the proposals itself, and the board leaves them alone a while.
func (s *Server) captureThing(thingID string, target *CaptureTarget, ask bool) *captureProposal {
	if ask {
		_ = s.thingLabels.Set(thingID, captureAskedUntilLabel, time.Now().Add(captureAskWait).UTC().Format(time.RFC3339))
	}
	switch {
	case target.Constellation == "" && target.Kind == "want":
		s.captureIntoWant(thingID, target.ID)
		// A photo already read — on the board before it was caught — is read
		// by the want's type now, and proposed (photo_fields.go).
		s.readPhotoFieldsSoon()
	case target.Constellation == "":
		return s.captureIntoTile(thingID, target)
	default:
		s.setConstellationMembership("thing", target.Constellation, thingID, true)
	}
	return nil
}

// POST /api/v1/capture/things/{thing}  {ask?: bool}
// — a thing on the board already, caught into the capture target as one just
// shared would be (captureThing). Answers what became of it: `proposal` when
// one is asked at once, `waiting` when it is being read to be proposed.
func (s *Server) captureExistingThing(w http.ResponseWriter, r *http.Request) {
	thing := mux.Vars(r)["thing"]
	var body struct {
		Ask bool `json:"ask"`
	}
	_ = DecodeRequest(r, &body)
	target := captureTargets.Get()
	if target == nil {
		s.JSONError(w, r, http.StatusConflict, "no capture target", "aim the net first")
		return
	}
	if target.ID == thing {
		s.JSONError(w, r, http.StatusBadRequest, "the target itself", thing)
		return
	}
	p := s.captureThing(thing, target, body.Ask)
	boardThingsChanged(thing)
	reply := map[string]any{"target": target}
	switch {
	case p != nil:
		reply["proposal"] = p
	case target.Constellation == "" && target.Kind == "want":
		reply["waiting"] = true
	default:
		reply["constellation"] = target.Constellation
		if target.Constellation == "" {
			reply["constellation"] = s.constellationOf(target.ID)
		}
	}
	s.JSONResponse(w, http.StatusOK, reply)
}

// captureIntoWant marks a thing captured into a want: read, and proposed.
func (s *Server) captureIntoWant(thingID, wantID string) {
	_ = s.thingLabels.Set(thingID, capturePendingLabel, wantID)
	// A thing with nothing to read — not a photo — is proposed at once.
	if s.thingLabels.All()[thingID][thingPictureLabel] == "" {
		s.proposeCapture(thingID)
	}
}

// captureIntoTile joins a capture to the thing or answer the net is aimed at
// with no constellation named: into the one the tile is in already, at once;
// else the board asks, offering a name after the tile — so a constellation is
// named when the first thing is caught into it, not when the net is aimed.
// The proposal is answered, for one waiting: whatever captured it asks the
// name then and there (the extension's copy offer, the phone), else the
// board does.
func (s *Server) captureIntoTile(thingID string, t *CaptureTarget) *captureProposal {
	if name := s.constellationOf(t.ID); name != "" {
		s.setConstellationMembership("thing", name, thingID, true)
		return nil
	}
	name := t.Name
	if name == "" {
		name = t.Label
	}
	p := captureProposal{
		Thing: thingID, ThingLabel: s.thingValue(thingID),
		Answer: t.ID, AnswerLabel: t.Label,
		Constellation: strings.ReplaceAll(name, "/", "."),
	}
	if i := strings.Index(t.ID, answerSep); i >= 0 {
		p.Want = t.ID[:i]
	}
	raw, _ := json.Marshal(p)
	_ = s.thingLabels.Set(thingID, captureProposalLabel, string(raw))
	boardThingsChanged(thingID)
	go broadcastSSE("capture_proposals_changed", map[string]any{"thing": thingID})
	return &p
}

// constellationOf is a constellation the tile is in (the first, by name), or "".
func (s *Server) constellationOf(id string) string {
	var names []string
	for k, v := range s.thingLabels.All()[id] {
		if v == "true" && strings.HasPrefix(k, constellationLabelPrefix) {
			names = append(names, strings.TrimPrefix(k, constellationLabelPrefix))
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return names[0]
}

// proposeCaptures proposes every capture waiting whose reading is done: read
// into fields, or with nothing more to read (its type has no photo-fields, or
// the photo has no words).
func (s *Server) proposeCaptures() {
	if s.thingLabels == nil {
		return
	}
	waiting := map[string]bool{}
	for _, p := range s.photosToRead() {
		waiting[p.ID] = true
	}
	for id, labels := range s.thingLabels.All() {
		if labels[capturePendingLabel] == "" || labels[captureProposalLabel] != "" || waiting[id] {
			continue
		}
		if labels[thingPictureLabel] != "" && !pictureReadCurrent(labels) && s.mayStillBeRead(id) {
			continue // its words first
		}
		s.proposeCapture(id)
	}
}

// captureReadWait is how long a capture waits for its words before it is
// proposed without them — a photo nobody here or on a phone is reading.
const captureReadWait = 10 * time.Minute

// mayStillBeRead says whether a capture's words may yet come: saved only a
// little while ago (this server reads in the background; a phone reads what
// it shared just after sharing it).
func (s *Server) mayStillBeRead(thingID string) bool {
	for _, e := range s.thingStore.Entries() {
		if e.ID == thingID {
			at, err := time.Parse(time.RFC3339, e.CreatedAt)
			return err != nil || time.Since(at) < captureReadWait
		}
	}
	return false
}

// proposeCapture works out and keeps the proposal for one capture.
func (s *Server) proposeCapture(thingID string) {
	labels := s.thingLabels.All()[thingID]
	wantID := labels[capturePendingLabel]
	if wantID == "" || s.globalBuilder == nil {
		return
	}
	var want *mywant.Want
	for _, w := range s.globalBuilder.GetWants() {
		if w.Metadata.ID == wantID {
			want = w
		}
	}
	if want == nil {
		_ = s.thingLabels.Remove(thingID, capturePendingLabel)
		return
	}
	at, by := s.captureTime(thingID, labels, want.Metadata.Type)
	entry, ok := nearestAnswer(want.BuildHistory().ResultHistory, at)
	if !ok {
		return // no answer to join it with yet
	}
	answer := wantID + answerSep + answerKey(entry)
	p := captureProposal{
		Thing: thingID, ThingLabel: s.thingValue(thingID),
		Want: wantID, WantName: want.Metadata.Name,
		Answer: answer, AnswerLabel: answerWhen(entry) + " " + answerSummary(entry),
		By: by,
	}
	// The constellation the answer is in already, else one named after it.
	if name := s.constellationOf(answer); name != "" {
		p.Constellation, p.Existing = name, true
	}
	if p.Constellation == "" {
		title := want.Metadata.Name
		if a := s.fmTypeAliases()[want.Metadata.Type]; len(a) > 0 {
			title = a[0]
		}
		p.Constellation = strings.ReplaceAll(title+" "+answerTime(entry).Format("1.2 15:04"), "/", ".")
	}
	raw, _ := json.Marshal(p)
	_ = s.thingLabels.Set(thingID, captureProposalLabel, string(raw))
	boardThingsChanged(thingID)
	go broadcastSSE("capture_proposals_changed", map[string]any{"thing": thingID})
}

// captureTime is the time a capture is matched to an answer by, and what it
// is: the type's photo-match field, else when the photo was taken, else when
// the thing was saved.
func (s *Server) captureTime(thingID string, labels map[string]string, wantType string) (time.Time, string) {
	if wts, err := s.fmWantTypes(); err == nil {
		for _, t := range wts {
			if t.Name != wantType {
				continue
			}
			if f := strings.TrimSpace(t.Labels[photoMatchLabel]); f != "" {
				if at, ok := parseLocalTime(labels[photoFieldPrefix+f]); ok {
					return at, f
				}
			}
		}
	}
	if at, ok := parseLocalTime(labels["picture-taken-at"]); ok {
		return at, "taken_at"
	}
	for _, e := range s.thingStore.Entries() {
		if e.ID == thingID {
			if at, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil {
				return at.In(time.Local), "saved"
			}
		}
	}
	return time.Now(), "now"
}

func parseLocalTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006/01/02 15:04:05", "2006/01/02 15:04", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// nearestAnswer is the answer a capture at `at` belongs with: the latest
// whose time is at most half an hour after it and half a day before it —
// a round is photographed after it starts — else the nearest at all.
func nearestAnswer(history []mywant.ResultHistoryEntry, at time.Time) (mywant.ResultHistoryEntry, bool) {
	var best mywant.ResultHistoryEntry
	found := false
	var bestT time.Time
	for _, e := range history {
		t := answerTime(e)
		if t.After(at.Add(30*time.Minute)) || at.Sub(t) > 12*time.Hour {
			continue
		}
		if !found || t.After(bestT) || (t.Equal(bestT) && e.Timestamp.After(best.Timestamp)) {
			best, bestT, found = e, t, true
		}
	}
	if found {
		return best, true
	}
	var gap time.Duration
	for _, e := range history {
		d := answerTime(e).Sub(at)
		if d < 0 {
			d = -d
		}
		if !found || d < gap || (d == gap && e.Timestamp.After(best.Timestamp)) {
			best, gap, found = e, d, true
		}
	}
	return best, found
}

// answerSummary is an answer in a few words: the values that say what it
// is about (a reservation's store and room), else its result cut short.
func answerSummary(e mywant.ResultHistoryEntry) string {
	if r, ok := e.Result.(map[string]any); ok {
		var parts []string
		for _, k := range []string{"room", "store", "name", "title", "summary"} {
			if v, ok := r[k].(string); ok && v != "" {
				parts = append(parts, v)
				break
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	raw, _ := json.Marshal(e.Result)
	return firstLine(string(raw), 40)
}

func (s *Server) thingValue(id string) string {
	for _, e := range s.thingStore.Entries() {
		if e.ID == id {
			return e.Value
		}
	}
	return id
}

// captureProposals are the proposals waiting for a person, oldest first.
func (s *Server) captureProposals() []captureProposal {
	var out []captureProposal
	for _, labels := range s.thingLabels.All() {
		var p captureProposal
		if raw := labels[captureProposalLabel]; raw != "" && json.Unmarshal([]byte(raw), &p) == nil {
			p.AskedUntil = labels[captureAskedUntilLabel]
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Thing < out[j].Thing })
	return out
}

// GET /api/v1/capture/proposals[?things=a,b&wait=25] — each with askedUntil
// when its sender is asking about it itself.
//
// With `things`, only theirs; with `wait` too (seconds, at most
// captureProposalMaxWait), answered once one of them is proposed — a photo
// being read meanwhile — or when the wait is up, with none. The one wait
// every sender shares: the extension's copy offer, the phone's card and its
// action button ask this and show what comes back.
func (s *Server) getCaptureProposals(w http.ResponseWriter, r *http.Request) {
	var only map[string]bool
	if raw := strings.TrimSpace(r.URL.Query().Get("things")); raw != "" {
		only = map[string]bool{}
		for _, id := range strings.Split(raw, ",") {
			if id = strings.TrimSpace(id); id != "" {
				only[id] = true
			}
		}
	}
	pick := func() []captureProposal {
		out := []captureProposal{}
		for _, p := range s.captureProposals() {
			if only == nil || only[p.Thing] {
				out = append(out, p)
			}
		}
		return out
	}
	list := pick()
	if wait, err := strconv.Atoi(r.URL.Query().Get("wait")); err == nil && wait > 0 && only != nil && len(list) == 0 {
		d := time.Duration(wait) * time.Second
		if d > captureProposalMaxWait {
			d = captureProposalMaxWait
		}
		deadline := time.NewTimer(d)
		defer deadline.Stop()
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
	waiting:
		for {
			select {
			case <-r.Context().Done():
				return
			case <-deadline.C:
				break waiting
			case <-tick.C:
				if list = pick(); len(list) > 0 {
					break waiting
				}
			}
		}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"proposals": list})
}

// captureProposalMaxWait bounds one wait for proposals; a sender waiting
// longer asks again.
const captureProposalMaxWait = 60 * time.Second

// POST /api/v1/capture/proposals/{thing}  {accept: bool, constellation?: string}
// — yes joins the answer and the capture in the constellation (the one
// proposed, or the name the person gave); no leaves the capture on its own.
func (s *Server) answerCaptureProposal(w http.ResponseWriter, r *http.Request) {
	thing := mux.Vars(r)["thing"]
	var body struct {
		Accept        bool   `json:"accept"`
		Constellation string `json:"constellation"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	var p captureProposal
	if raw := s.thingLabels.All()[thing][captureProposalLabel]; raw == "" || json.Unmarshal([]byte(raw), &p) != nil {
		s.JSONError(w, r, http.StatusNotFound, "no proposal for this thing", thing)
		return
	}
	name := p.Constellation
	if body.Accept {
		if c := strings.TrimSpace(body.Constellation); c != "" {
			name = strings.ReplaceAll(c, "/", ".")
		}
		if err := validateConstellationName(name); err != nil {
			s.JSONError(w, r, http.StatusBadRequest, "invalid constellation name", err.Error())
			return
		}
		s.setConstellationMembership("thing", name, p.Answer, true)
		s.setConstellationMembership("thing", name, thing, true)
		boardThingsChanged(p.Answer)
	}
	_ = s.thingLabels.Remove(thing, captureProposalLabel)
	_ = s.thingLabels.Remove(thing, capturePendingLabel)
	_ = s.thingLabels.Remove(thing, captureAskedUntilLabel)
	boardThingsChanged(thing)
	go broadcastSSE("capture_proposals_changed", map[string]any{"thing": thing})
	reply := map[string]any{"accepted": body.Accept}
	if body.Accept {
		reply["constellation"] = name
		reply["message"] = fmt.Sprintf("%s を %s に結びました（%s）", p.ThingLabel, p.AnswerLabel, name)
	}
	s.JSONResponse(w, http.StatusOK, reply)
}
