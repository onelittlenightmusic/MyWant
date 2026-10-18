package server

import (
	"sort"
	"strings"
	"time"

	mywant "mywant/engine/core"
	types "mywant/engine/types"
)

// Finding the photos a question about a want's answers is about — the
// server's half of types/answer_photos.go, which reads and asks.
//
// "ゴルフの最新スコアは？": the want is found by what the question calls it —
// its name, its type, or one of the type's aliases ("ゴルフ" for
// smartgolf_check_reserved). Its answers, newest first; each answer's
// constellations (it is a member as a thing is: `<want id>/result/<key>`);
// and in those, the things that are photos (a picture-url). One photo is a
// question for each in turn, newest answer first.
//
// Only a question that asks after what a photo shows is looked at this way —
// a score, a photo, a picture — so 「ゴルフの予約は？」 is still the want's own
// answer, read the ordinary way.

// photoQuestionWords are how a question says it is after what a photo shows.
var photoQuestionWords = []string{"写真", "画像", "スコア", "score", "photo", "picture", "打数", "成績", "結果"}

// Where a photo's reading is kept on its thing (thing_picture_text.go): the
// image it was read from (a new image is read again), and the one blob
// readings were kept as before they had fields of their own.
const (
	thingPictureReadLabel     = "picture-read"
	thingPictureReadFromLabel = "picture-read-from"
)

// photoReadingLabel is a want type's rules for reading the photos joined to
// its answers — what in such a photo answers what: smartgolf's "the score is
// the 87（+15） at the end of the Player 1 row; the HOLE and PAR rows are hole
// numbers and pars, not scores". The model reads the photo's words by them;
// nothing here knows what a score card looks like.
const photoReadingLabel = "photo-reading"

// answerSep joins a want id and an answer's key into the answer's id.
const answerSep = "/result/"

func (s *Server) registerAnswerPhotos() {
	types.AnswerPhotoFinder = s.findAnswerPhotos
	types.AnswerPhotoReadSaver = func(id, imageURL string, read types.PhotoReading) {
		s.keepPhotoReading(id, imageURL, "mywant", read)
	}
	// What an answer was read off: the robot goes and stands on it.
	types.AnswerEvidence = func(id string) { go s.walkRobotToEvidence(id) }
	types.AnswerReferentNote = func(question string) string {
		if ref := s.resolveAnswerReferent(question, time.Now()); ref != nil {
			return ref.text()
		}
		return ""
	}
}

func (s *Server) findAnswerPhotos(question string) []types.AnswerPhoto {
	q := strings.ToLower(question)
	if s.globalBuilder == nil || s.thingLabels == nil {
		return nil
	}

	// The wants the question names (wantsNamedIn) — and, when it names a time
	// too (「今朝のゴルフ」), only their answers from then (answer_referent.go).
	wants, title := s.wantsNamedIn(q)
	if len(wants) == 0 {
		return nil
	}
	// Only a question after what a photo shows: a photo word, or a field the
	// named wants' photos are read into (photo_fields.go) — 「ゴルフのコースは？」.
	specs := s.photoTypeSpecs()
	asked := containsAnyWord(q, photoQuestionWords)
	for _, w := range wants {
		asked = asked || types.PhotoFieldAsked(q, specs[w.Metadata.Type].Fields) != ""
	}
	if !asked {
		return nil
	}
	var only map[string]bool
	if ref := s.resolveAnswerReferent(q, time.Now()); ref != nil {
		only = ref.answerIDs()
	}
	// How each type's photos are read, as its YAML says (photoReadingLabel).
	rules := map[string]string{}
	if wts, err := s.fmWantTypes(); err == nil {
		for _, t := range wts {
			if r := strings.Join(strings.Fields(t.Labels[photoReadingLabel]), " "); r != "" {
				rules[t.Name] = r
			}
		}
	}
	typeOf := map[string]string{}
	for _, w := range wants {
		typeOf[w.Metadata.ID] = w.Metadata.Type
	}

	// Who is in which constellation, from the thing labels (an answer is kept
	// there as a thing is).
	all := s.thingLabels.All()
	membersOf := map[string][]string{}
	for id, labels := range all {
		for k, v := range labels {
			if v == "true" && strings.HasPrefix(k, constellationLabelPrefix) {
				name := strings.TrimPrefix(k, constellationLabelPrefix)
				membersOf[name] = append(membersOf[name], id)
			}
		}
	}
	constellationsOf := map[string][]string{}
	for name, members := range membersOf {
		sort.Strings(members)
		for _, m := range members {
			constellationsOf[m] = append(constellationsOf[m], name)
		}
	}
	values := map[string]string{}
	created := map[string]string{}
	for _, e := range s.thingStore.Entries() {
		values[e.ID] = e.Value
		created[e.ID] = e.CreatedAt
	}

	// Newest answer first, across every want named.
	type answerOf struct {
		wantID string
		e      mywant.ResultHistoryEntry
	}
	var answers []answerOf
	for _, w := range wants {
		for _, e := range w.BuildHistory().ResultHistory {
			answers = append(answers, answerOf{w.Metadata.ID, e})
		}
	}
	sort.SliceStable(answers, func(i, j int) bool { return answers[i].e.Timestamp.After(answers[j].e.Timestamp) })
	seen := map[string]bool{}
	var out []types.AnswerPhoto
	for _, a := range answers {
		e := a.e
		answer := a.wantID + answerSep + answerKey(e)
		if only != nil && !only[answer] {
			continue
		}
		var photos []string
		for _, c := range constellationsOf[answer] {
			for _, m := range membersOf[c] {
				if m == answer || seen[m] || strings.Contains(m, answerSep) {
					continue
				}
				if all[m][thingPictureLabel] == "" {
					continue
				}
				seen[m] = true
				photos = append(photos, m)
			}
		}
		// The newest photo of an answer first.
		sort.SliceStable(photos, func(a, b int) bool { return created[photos[a]] > created[photos[b]] })
		about := answerWhen(e) + " " + title
		for _, id := range photos {
			typ := typeOf[a.wantID]
			p := types.AnswerPhoto{ID: id, Name: values[id], About: strings.TrimSpace(about), ImageURL: all[id][thingPictureLabel], Rules: rules[typ]}
			if spec, ok := specs[typ]; ok {
				p.FieldSpec = spec.Fields
				p.Fields = photoFieldsOf(all[id], typ, spec)
			}
			if all[id][thingPictureReadFromLabel] == p.ImageURL {
				p.Read = photoReadingOf(all[id])
			}
			out = append(out, p)
		}
	}
	return out
}

// answerWhen is the time an answer is about, as a person reads it: a
// reservation's own time when it says one, else when it was answered.
func answerWhen(e mywant.ResultHistoryEntry) string {
	return answerTime(e).Format("1/2 15:04")
}

func containsAnyWord(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, strings.ToLower(w)) {
			return true
		}
	}
	return false
}
