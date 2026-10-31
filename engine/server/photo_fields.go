package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	types "mywant/engine/types"
)

// Reading a photo's words into fields when it is joined to a want's answer —
// the server's half of types/photo_fields.go.
//
// A photo thing has its words once it is saved (thing_picture_text.go). Joined
// — in a constellation with a want's answer, as the capture skill joins a
// score card to the reservation it was played on — it is read again, by the
// rules of that want's type (its photo-fields label), into fields of its own:
// photo-score, photo-course, photo-played_at. Then the robot answers "今朝の
// ゴルフのスコアは？" from photo-score, with no model and the same answer every
// time.
//
// Who reads: whoever has a model, by model work's order (model_jobs.go) — a
// device that can (the iPhone app, a Mac's Chrome with fmtool), else this
// server's own (fmtool on a Mac). Every time things change, any photo joined
// and not yet read by its rules is read, in the background: the prompt is
// written here and the answer checked here, whoever ran the model — only
// values in the photo's words are kept.

const (
	// A want type's fields for the photos joined to its answers, one a line:
	// "name | word, word | rule" (types.ParsePhotoFields).
	photoFieldsLabel = "photo-fields"
	// On the photo: which type it was read as, and from what (a fingerprint of
	// the rules and the words — either changing reads it again).
	photoReadAsLabel     = "photo-read-as"
	photoFieldsFromLabel = "photo-fields-from"
	// Each field, on the photo: photo-<name>.
	photoFieldPrefix = "photo-"
)

// photoTypeSpec is how one type's photos are read.
type photoTypeSpec struct {
	About  string // the type's photo-reading: what such a photo is
	Fields []types.PhotoField
	raw    string
}

// photoTypeSpecs are the types that read their photos into fields.
func (s *Server) photoTypeSpecs() map[string]photoTypeSpec {
	out := map[string]photoTypeSpec{}
	wts, err := s.fmWantTypes()
	if err != nil {
		return out
	}
	for _, t := range wts {
		raw := t.Labels[photoFieldsLabel]
		if fields := types.ParsePhotoFields(raw); len(fields) > 0 {
			about := strings.Join(strings.Fields(t.Labels[photoReadingLabel]), " ")
			out[t.Name] = photoTypeSpec{About: about, Fields: fields, raw: raw}
		}
	}
	return out
}

// photoToRead is one photo to read into fields.
type photoToRead struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Fingerprint string   `json:"fingerprint"`
	Prompt      string   `json:"prompt"`
	Fields      []string `json:"fields"`
	lines       []string
	spec        photoTypeSpec
}

// photoFieldsReader is how the fields are read and checked; raised when that
// changes, so every photo is read again.
const photoFieldsReader = "2"

func photoFingerprint(typ string, spec photoTypeSpec, text string) string {
	h := sha1.Sum([]byte(photoFieldsReader + "\x00" + typ + "\x00" + spec.raw + "\x00" + spec.About + "\x00" + text))
	return hex.EncodeToString(h[:8])
}

// photosToRead are the photos joined to an answer of a type with
// photo-fields, already read (their words), and not yet read by those rules.
func (s *Server) photosToRead() []photoToRead {
	if s.thingLabels == nil || s.globalBuilder == nil {
		return nil
	}
	specs := s.photoTypeSpecs()
	if len(specs) == 0 {
		return nil
	}
	typeOfWant := map[string]string{}
	for _, w := range s.globalBuilder.GetWants() {
		typeOfWant[w.Metadata.ID] = w.Metadata.Type
	}
	all := s.thingLabels.All()
	members := map[string][]string{}
	for id, labels := range all {
		for k, v := range labels {
			if v == "true" && strings.HasPrefix(k, constellationLabelPrefix) {
				name := strings.TrimPrefix(k, constellationLabelPrefix)
				members[name] = append(members[name], id)
			}
		}
	}
	// Each photo's type: the type of an answer it shares a constellation with
	// (the first, by name, when there are several).
	names := make([]string, 0, len(members))
	for n := range members {
		names = append(names, n)
	}
	sort.Strings(names)
	typeOfPhoto := map[string]string{}
	for _, n := range names {
		typ := ""
		for _, m := range members[n] {
			if i := strings.Index(m, answerSep); i >= 0 {
				if t := typeOfWant[m[:i]]; specs[t].Fields != nil {
					typ = t
					break
				}
			}
		}
		if typ == "" {
			continue
		}
		for _, m := range members[n] {
			if !strings.Contains(m, answerSep) && all[m][thingPictureLabel] != "" && typeOfPhoto[m] == "" {
				typeOfPhoto[m] = typ
			}
		}
	}
	// A capture into a whole want, waiting to be read by its type and
	// proposed (capture_proposal.go).
	for id, labels := range all {
		if w := labels[capturePendingLabel]; w != "" && labels[thingPictureLabel] != "" && typeOfPhoto[id] == "" {
			if t := typeOfWant[w]; specs[t].Fields != nil {
				typeOfPhoto[id] = t
			}
		}
	}
	var out []photoToRead
	for id, typ := range typeOfPhoto {
		labels := all[id]
		if !pictureReadCurrent(labels) {
			continue // its words first (thing_picture_text.go)
		}
		reading := photoReadingOf(labels)
		if reading == nil || len(reading.Lines) == 0 {
			continue
		}
		spec := specs[typ]
		fp := photoFingerprint(typ, spec, strings.Join(reading.Lines, "\n"))
		if labels[photoReadAsLabel] == typ && labels[photoFieldsFromLabel] == fp {
			continue
		}
		names := make([]string, len(spec.Fields))
		for i, f := range spec.Fields {
			names[i] = f.Name
		}
		out = append(out, photoToRead{
			ID: id, Type: typ, Fingerprint: fp, Fields: names,
			Prompt: types.PhotoFieldsPrompt(spec.About, spec.Fields, reading.Lines),
			lines:  reading.Lines, spec: spec,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// keepPhotoFields writes what was read onto the photo: each field, which type
// it was read as, and from what.
func (s *Server) keepPhotoFields(p photoToRead, values map[string]string) {
	// Fields of an older reading — a field the type no longer has, or another
	// type's — go first.
	keep := map[string]bool{photoReadAsLabel: true, photoFieldsFromLabel: true}
	for _, f := range p.spec.Fields {
		keep[photoFieldPrefix+f.Name] = true
	}
	for k := range s.thingLabels.All()[p.ID] {
		if strings.HasPrefix(k, photoFieldPrefix) && !keep[k] {
			_ = s.thingLabels.Remove(p.ID, k)
		}
	}
	for _, f := range p.spec.Fields {
		key := photoFieldPrefix + f.Name
		if v := values[f.Name]; v != "" {
			_ = s.thingLabels.Set(p.ID, key, v)
		} else {
			_ = s.thingLabels.Remove(p.ID, key)
		}
	}
	_ = s.thingLabels.Set(p.ID, photoReadAsLabel, p.Type)
	_ = s.thingLabels.Set(p.ID, photoFieldsFromLabel, p.Fingerprint)
	boardThingsChanged(p.ID)
	log.Printf("[things] read the photo %s as %s: %v", p.ID, p.Type, values)
}

// photoFieldsReading keeps one reading pass at a time.
var photoFieldsReading sync.Mutex

// photoFieldsDeviceWait is how long a device has to answer one photo: work in
// the background, nobody waiting on it.
const photoFieldsDeviceWait = time.Minute

// readPhotoFieldsSoon reads, in the background, every photo waiting to be
// read into fields — when there is a model to read with, a device's or this
// server's; else they wait for one (a device arriving asks again,
// pendingBrowserAction).
func (s *Server) readPhotoFieldsSoon() {
	if !serverHasModel() && !modelDeviceCan(modelOpGenerate, "", time.Now()) {
		// Nobody to read them yet; what is read already is proposed.
		s.proposeCaptures()
		return
	}
	go func() {
		if !photoFieldsReading.TryLock() {
			return
		}
		defer photoFieldsReading.Unlock()
		for _, p := range s.photosToRead() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			text, by, err := generateText(ctx, p.Prompt, false, photoFieldsDeviceWait)
			cancel()
			if err != nil {
				log.Printf("[things] could not read the photo %s as %s: %v", p.ID, p.Type, err)
				continue
			}
			// The words or the rules may have changed while it was being read:
			// an answer to an old prompt is not this photo's any more.
			if !s.stillToRead(p) {
				continue
			}
			s.keepPhotoFields(p, types.ParsePhotoFieldsAnswer(text, p.spec.Fields, p.lines))
			log.Printf("[things] the photo %s was read by the %s's model", p.ID, by)
		}
		// Captures into a want, read: which answer they go with.
		s.proposeCaptures()
	}()
}

// stillToRead says whether p is still waiting to be read as it was when its
// prompt was written.
func (s *Server) stillToRead(p photoToRead) bool {
	for _, q := range s.photosToRead() {
		if q.ID == p.ID {
			return q.Type == p.Type && q.Fingerprint == p.Fingerprint
		}
	}
	return false
}

// photoFieldsOf is a photo's fields as read by a type, or nil.
func photoFieldsOf(labels map[string]string, typ string, spec photoTypeSpec) map[string]string {
	if labels[photoReadAsLabel] != typ {
		return nil
	}
	out := map[string]string{}
	for _, f := range spec.Fields {
		if v := labels[photoFieldPrefix+f.Name]; v != "" {
			out[f.Name] = v
		}
	}
	return out
}
