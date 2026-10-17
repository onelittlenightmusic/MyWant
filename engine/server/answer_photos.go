package server

import (
	"encoding/json"
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

// Labels a photo's reading is kept under, on its thing, with the image it was
// read from (a new image is read again).
const (
	thingPictureReadLabel     = "picture-read"
	thingPictureReadFromLabel = "picture-read-from"
)

// answerSep joins a want id and an answer's key into the answer's id.
const answerSep = "/result/"

func (s *Server) registerAnswerPhotos() {
	types.AnswerPhotoFinder = s.findAnswerPhotos
	types.AnswerPhotoReadSaver = func(id, imageURL string, read types.PhotoReading) {
		data, err := json.Marshal(read)
		if err != nil {
			return
		}
		_ = s.thingLabels.Set(id, thingPictureReadLabel, string(data))
		_ = s.thingLabels.Set(id, thingPictureReadFromLabel, imageURL)
	}
	types.AnswerReferentNote = func(question string) string {
		if ref := s.resolveAnswerReferent(question, time.Now()); ref != nil {
			return ref.text()
		}
		return ""
	}
}

func (s *Server) findAnswerPhotos(question string) []types.AnswerPhoto {
	q := strings.ToLower(question)
	if !containsAnyWord(q, photoQuestionWords) || s.globalBuilder == nil || s.thingLabels == nil {
		return nil
	}

	// The wants the question names (wantsNamedIn) — and, when it names a time
	// too (「今朝のゴルフ」), only their answers from then (answer_referent.go).
	wants, title := s.wantsNamedIn(q)
	if len(wants) == 0 {
		return nil
	}
	var only map[string]bool
	if ref := s.resolveAnswerReferent(q, time.Now()); ref != nil {
		only = ref.answerIDs()
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
			p := types.AnswerPhoto{ID: id, Name: values[id], About: strings.TrimSpace(about), ImageURL: all[id][thingPictureLabel]}
			if all[id][thingPictureReadFromLabel] == p.ImageURL {
				var r types.PhotoReading
				if json.Unmarshal([]byte(all[id][thingPictureReadLabel]), &r) == nil {
					p.Read = &r
				}
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
