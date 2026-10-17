package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	mywant "mywant/engine/core"
	types "mywant/engine/types"
)

// What a question means by "this morning's golf" — the first stage of
// answering it, before what is asked about it is worked out.
//
// "今朝のゴルフのスコアは？" is two questions in one: WHICH of the golf want's
// answers is meant (this morning's — the 09:00 reservation, not the 13:00 one
// it answers now), and WHAT about it is asked (the score). The second already
// has its own handling — a score is read from the photos joined to the answer
// (answer_photos.go), anything else from the answer itself. The first was
// nobody's: asked about this morning, the robot read the want as it stands
// now and answered about the afternoon.
//
// So the first is settled here, by the server and not by the model: a time
// the question names (今朝, 昨日の夜, yesterday) is a window, the want it
// names (by its name, its type or one of the type's aliases, as
// findAnswerPhotos finds it) has a history of answers, and the answers that
// fall in the window are what the question is about. 「今朝の乗り換え」 is the
// same: this morning's answers of the transit wants.
//
// When an answer "is" is the time it is about — a reservation's own time
// when it says one — else when it was answered (answerTime).

// answerReferent is what a question picked out: the wants it named, the time
// it named, and their answers in that time, newest first.
type answerReferent struct {
	// The words that named the time, e.g. "今朝", and the window they mean.
	Phrase   string
	From, To time.Time
	// The wants the question named; Title is what it called them.
	Wants   []*mywant.Want
	Title   string
	Answers []referentAnswer
}

type referentAnswer struct {
	Want  *mywant.Want
	Entry mywant.ResultHistoryEntry
	// The answer's id as the board names it: `<want id>/result/<key>`.
	ID   string
	When time.Time
}

// covers says whether the referent is about this want.
func (r *answerReferent) covers(wantID string) bool {
	for _, w := range r.Wants {
		if w.Metadata.ID == wantID {
			return true
		}
	}
	return false
}

// answerIDs is the set of answers picked out, by their board ids.
func (r *answerReferent) answerIDs() map[string]bool {
	ids := map[string]bool{}
	for _, a := range r.Answers {
		ids[a.ID] = true
	}
	return ids
}

// window says the time span in words a person reads: "10/17 04:00–12:00".
func (r *answerReferent) window() string {
	to := r.To.Add(-time.Minute)
	if r.From.Format("1/2") == to.Format("1/2") {
		end := r.To.Format("15:04")
		if end == "00:00" {
			end = "24:00"
		}
		return r.From.Format("1/2 15:04") + "–" + end
	}
	return r.From.Format("1/2 15:04") + "–" + to.Format("1/2 15:04")
}

// A day a question names, as days back from today, and a part of a day as the
// hours it spans (past 24 runs into the next morning).
type dayWord struct {
	word string
	back int
}
type partWord struct {
	word     string
	from, to int
}

var (
	// Longest first: 一昨日 before 昨日.
	referentDays = []dayWord{
		{"一昨日", 2}, {"おととい", 2}, {"day before yesterday", 2},
		{"昨日", 1}, {"きのう", 1}, {"yesterday", 1},
		{"今日", 0}, {"きょう", 0}, {"today", 0},
	}
	referentParts = []partWord{
		{"午前", 0, 12}, {"朝", 4, 12}, {"morning", 4, 12},
		{"昼", 11, 14}, {"noon", 11, 14},
		{"午後", 12, 18}, {"afternoon", 12, 18},
		{"夕方", 16, 19}, {"evening", 16, 22},
		{"夜", 18, 28}, {"晩", 18, 28}, {"night", 18, 28},
	}
	// Words that are a day and a part in one.
	referentCompounds = []struct {
		word string
		back int
		part partWord
	}{
		{"今朝", 0, partWord{"朝", 4, 12}}, {"けさ", 0, partWord{"朝", 4, 12}}, {"this morning", 0, partWord{"morning", 4, 12}},
		{"昨夜", 1, partWord{"夜", 18, 28}}, {"ゆうべ", 1, partWord{"夜", 18, 28}}, {"昨晩", 1, partWord{"夜", 18, 28}}, {"last night", 1, partWord{"night", 18, 28}},
		{"今夜", 0, partWord{"夜", 18, 28}}, {"今晩", 0, partWord{"夜", 18, 28}}, {"tonight", 0, partWord{"night", 18, 28}},
		{"this afternoon", 0, partWord{"afternoon", 12, 18}},
	}
)

// referentTime is the time window a question names, or ok=false when it names
// none. A part of a day alone (「朝のゴルフ」) is today's; a part counts only
// when it is plainly about the time — followed by の/に or after a day — so
// 朝日 or 夜景 in a name is not taken for one.
func referentTime(q string, now time.Time) (phrase string, from, to time.Time, ok bool) {
	q = strings.ToLower(q)
	day := func(back int) time.Time {
		y, m, d := now.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -back)
	}
	span := func(back int, p partWord) (time.Time, time.Time) {
		d := day(back)
		return d.Add(time.Duration(p.from) * time.Hour), d.Add(time.Duration(p.to) * time.Hour)
	}
	for _, c := range referentCompounds {
		if strings.Contains(q, c.word) {
			f, t := span(c.back, c.part)
			return c.word, f, t, true
		}
	}
	back, dayAt, dayLen := 0, -1, 0
	dayPhrase := ""
	for _, d := range referentDays {
		if i := strings.Index(q, d.word); i >= 0 {
			back, dayAt, dayLen, dayPhrase = d.back, i, len(d.word), d.word
			break
		}
	}
	for _, p := range referentParts {
		i := strings.Index(q, p.word)
		if i < 0 {
			continue
		}
		after := q[i+len(p.word):]
		afterDay := dayAt >= 0 && i >= dayAt+dayLen && i-(dayAt+dayLen) <= len("の ")
		if !afterDay && !strings.HasPrefix(after, "の") && !strings.HasPrefix(after, "に") && !strings.HasPrefix(after, " ") {
			continue
		}
		f, t := span(back, p)
		if dayPhrase != "" {
			return dayPhrase + "の" + p.word, f, t, true
		}
		return p.word, f, t, true
	}
	if dayPhrase != "" {
		d := day(back)
		return dayPhrase, d, d.AddDate(0, 0, 1), true
	}
	return "", time.Time{}, time.Time{}, false
}

// wantsNamedIn are the wants a question names — by name, type, or one of the
// type's aliases; the longest term found wins, so "スマートゴルフ" is not taken
// for some want merely called "ゴルフ", and every want that term names is
// returned (several transit wants share 乗り換え). title is what the question
// called them, for saying so.
func (s *Server) wantsNamedIn(q string) (wants []*mywant.Want, title string) {
	if s.globalBuilder == nil {
		return nil, ""
	}
	q = strings.ToLower(q)
	aliases := s.fmTypeAliases()
	best := 0
	for _, w := range s.globalBuilder.GetWants() {
		terms := []string{w.Metadata.Name, w.Metadata.Type}
		terms = append(terms, aliases[w.Metadata.Type]...)
		hit := 0
		for _, t := range terms {
			if t = strings.ToLower(strings.TrimSpace(t)); t != "" && len([]rune(t)) > hit && strings.Contains(q, t) {
				hit = len([]rune(t))
			}
		}
		switch {
		case hit == 0 || hit < best:
		case hit > best:
			wants, best = []*mywant.Want{w}, hit
		default:
			wants = append(wants, w)
		}
	}
	if len(wants) == 0 {
		return nil, ""
	}
	title = wants[0].Metadata.Name
	if a := aliases[wants[0].Metadata.Type]; len(a) > 0 {
		title = a[0]
	}
	return wants, title
}

// resolveAnswerReferent is what a question picks out, or nil when it names no
// time or no want. A referent with no answers is still returned: "nothing this
// morning" is the answer then, not the want as it stands now.
func (s *Server) resolveAnswerReferent(q string, now time.Time) *answerReferent {
	phrase, from, to, ok := referentTime(q, now)
	if !ok {
		return nil
	}
	wants, title := s.wantsNamedIn(q)
	if len(wants) == 0 {
		return nil
	}
	r := &answerReferent{Phrase: phrase, From: from, To: to, Wants: wants, Title: title}
	for _, w := range wants {
		for _, e := range w.BuildHistory().ResultHistory {
			at := answerTime(e)
			if at.Before(from) || !at.Before(to) {
				continue
			}
			r.Answers = append(r.Answers, referentAnswer{Want: w, Entry: e, ID: w.Metadata.ID + answerSep + answerKey(e), When: at})
		}
	}
	// Newest first; of two copies of one answer (the engine keeps a new entry
	// when its supporting fields move) the newer, better-described one only.
	sort.SliceStable(r.Answers, func(i, j int) bool { return r.Answers[i].Entry.Timestamp.After(r.Answers[j].Entry.Timestamp) })
	var kept []referentAnswer
	seen := map[string]bool{}
	for _, a := range r.Answers {
		raw, _ := json.Marshal(a.Entry.Result)
		k := a.Want.Metadata.ID + string(raw)
		if seen[k] {
			continue
		}
		seen[k] = true
		kept = append(kept, a)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].When.After(kept[j].When) })
	r.Answers = kept
	return r
}

// answerKey is how an answer is named: its id, or its timestamp before
// answers had ids (outputEntryKey on the board).
func answerKey(e mywant.ResultHistoryEntry) string {
	if e.ID != "" {
		return e.ID
	}
	return e.Timestamp.Format(time.RFC3339Nano)
}

// answerTime is the time an answer is about: a reservation's own time when it
// says one, else when it was answered.
func answerTime(e mywant.ResultHistoryEntry) time.Time {
	if r, ok := e.Result.(map[string]any); ok {
		if v, ok := r["datetime_rfc3339"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t.In(time.Local)
			}
		}
	}
	return e.Timestamp.In(time.Local)
}

// text is the referent as the robot is told it: which answers the
// question means, each with what it said — said plainly to be the ones meant,
// not the want's answer now.
func (r *answerReferent) text() string {
	var b strings.Builder
	if len(r.Answers) == 0 {
		fmt.Fprintf(&b, "The question asks about %s's answers from %s (%s). There are none in that time — say so; do not answer with what it says now.\n", r.Title, r.Phrase, r.window())
		return b.String()
	}
	fmt.Fprintf(&b, "The question asks about %s's answers from %s (%s) — these, not what it says now:\n", r.Title, r.Phrase, r.window())
	for i, a := range r.Answers {
		if i == 3 {
			fmt.Fprintf(&b, "(and %d more)\n", len(r.Answers)-i)
			break
		}
		raw, _ := json.Marshal(a.Entry.Result)
		fmt.Fprintf(&b, "- %s %s: %s\n", a.When.Format("1/2 15:04"), a.Want.Metadata.Name, firstLine(string(raw), 300))
	}
	return b.String()
}

// fmJapanese says whether a question is written in Japanese (any kana or kanji).
func fmJapanese(q string) bool {
	for _, r := range q {
		if (r >= 0x3040 && r <= 0x30ff) || (r >= 0x4e00 && r <= 0x9fff) {
			return true
		}
	}
	return false
}

// fmLookReferent is look's answer when the question named a time of the
// want's answers: those answers, and — asked what a photo of them shows (a
// score) — the photos joined to them, answered here as the robot on this
// machine answers them (types.AnswerFromPhotos), else the words read from
// them for the asker's own model to read.
func (s *Server) fmLookReferent(ref *answerReferent, question string, args map[string]string) string {
	if len(ref.Answers) > 0 {
		a := ref.Answers[0]
		args[fmCardArg] = fmCardValue("want", a.Want.Metadata.ID, a.Want.Metadata.Name)
	} else if len(ref.Wants) > 0 {
		args[fmCardArg] = fmCardValue("want", ref.Wants[0].Metadata.ID, ref.Wants[0].Metadata.Name)
	}
	var b strings.Builder
	b.WriteString(ref.text())
	if len(ref.Answers) == 0 || !containsAnyWord(strings.ToLower(question), photoQuestionWords) {
		return b.String()
	}
	photos := s.findAnswerPhotos(question)
	if len(photos) == 0 {
		b.WriteString("No photo is joined to these answers, so what a photo would show is not known — say so.\n")
		return b.String()
	}
	robot, _ := s.fmRobot()
	if answer, ok := types.AnswerFromPhotos(context.Background(), robot, question, photos); ok {
		fmt.Fprintf(&b, "Answer, read from the photo joined to it: %s\n", answer)
		if fmJapanese(question) {
			fmt.Fprintf(&b, "Reply in Japanese, with exactly this: 「%sの%s：%s」\n", ref.Phrase, ref.Title, answer)
		} else {
			b.WriteString("Tell the person this answer, in the language they asked in.\n")
		}
		return b.String()
	}
	// No model here to ask (a server without one): what was read from the
	// photos, for the asker's model to find the answer in — and the photos
	// nobody has read yet, for a device that can read them (the iPhone app's
	// PhotoReader) to read and ask again.
	var unread []fmUnreadPhoto
	for _, p := range photos {
		if p.Read == nil && p.ImageURL != "" {
			unread = append(unread, fmUnreadPhoto{ID: p.ID, Image: p.ImageURL})
		}
	}
	if len(unread) > 0 {
		raw, _ := json.Marshal(unread)
		args[fmUnreadArg] = string(raw)
	}
	words := false
	for i, p := range photos {
		if i == 2 {
			break
		}
		lines := []string{}
		if p.Read != nil {
			lines = p.Read.Lines
		}
		if len(lines) == 0 {
			fmt.Fprintf(&b, "Photo %s (joined to %s): not read yet.\n", p.Name, p.About)
			continue
		}
		text := strings.Join(lines, "\n")
		if len([]rune(text)) > 600 {
			text = string([]rune(text)[:600]) + "…"
		}
		fmt.Fprintf(&b, "Photo %s (joined to %s), the words read from it:\n%s\n", p.Name, p.About, text)
		words = true
	}
	if words {
		b.WriteString("Answer the question from these words only.\n")
	} else {
		b.WriteString("No photo of these has been read, so what it shows is not known yet — say so.\n")
	}
	return b.String()
}
