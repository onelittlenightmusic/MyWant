package types

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "mywant/engine/core"
)

// A want's answers, asked about through their photos.
//
// A photo taken at the practice a reservation was for is joined to that
// answer on the board (a constellation holding the answer and the photo — the
// capture skill makes one). "ゴルフの最新スコアは？" is then a question about
// those photos: the reservation said where and when, the score card says the
// score. Which photos, newest answer first, is the server's to find — it keeps
// the constellations and the things (AnswerPhotoFinder); reading and asking is
// done here, the way a picture want's photo is.
//
// Each photo is READ first, on this machine, before anybody is asked about it:
// the words in it (Vision's text recogniser) and what the file says about
// itself (when it was taken, with what, where — ImageIO), by `fmtool --read`.
// Only then is the model shown the photo with all of that, and asked whether
// this one answers — and if it does, what the answer is. A photo that does not
// is passed over for the next.

// AnswerPhoto is one photo joined to one of a want's answers.
type AnswerPhoto struct {
	// The thing's id, and what it is called.
	ID   string
	Name string
	// Which answer it is joined to, for a person: "10/11 09:00 北新宿店/右打席予約(Room01)".
	About string
	// The image itself (the thing's picture-url).
	ImageURL string
	// What was read from it before, when it was read from this same image.
	Read *PhotoReading
	// How to read it: the want type's photo-reading rules — what in such a
	// photo answers what ("the score is the 87（+15） at the end of the Player
	// row"). Handed to the model with the photo and its words.
	Rules string
	// The fields the photo was read into by its want's type (photo_fields.go)
	// — score, course — and the type's spec of them, so a question is
	// answered from the field it asks for.
	Fields    map[string]string
	FieldSpec []PhotoField
}

// PhotoReading is what `fmtool --read` found in a photo.
type PhotoReading struct {
	Lines []string          `json:"lines"`
	Meta  map[string]string `json:"meta,omitempty"`
}

// AnswerPhotoFinder finds the photos a request is about, newest answer first,
// or none when it is not about a want's answers' photos. Set by the server.
var AnswerPhotoFinder func(question string) []AnswerPhoto

// AnswerPhotoReadSaver keeps what was read from a photo on its thing, so it is
// read once. Set by the server.
var AnswerPhotoReadSaver func(id, imageURL string, read PhotoReading)

// maxAnswerPhotos is how many photos one question looks through.
const maxAnswerPhotos = 6

// readPhoto runs `fmtool --read` on an image file: the words in it and the
// file's own metadata. An older fmtool without --read still reads the words
// (--ocr).
func readPhoto(ctx context.Context, fmtool, file string) (PhotoReading, error) {
	var r PhotoReading
	out, err := exec.CommandContext(ctx, fmtool, "--read", file).Output()
	if err != nil {
		out, err = exec.CommandContext(ctx, fmtool, "--ocr", file).Output()
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return r, fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return r, err
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return r, fmt.Errorf("fmtool --read: %v", err)
	}
	return r, nil
}

// CanReadPhotos says whether this machine can read photos itself — it has the
// recogniser (fmtool, on a Mac).
func CanReadPhotos() bool {
	_, ok := fmToolPath()
	return ok
}

// ReadPhoto reads a photo at a URL on this machine: downloaded, then the
// words in it and its file's own facts (readPhoto).
func ReadPhoto(ctx context.Context, imageURL string) (PhotoReading, error) {
	fmtool, ok := fmToolPath()
	if !ok {
		return PhotoReading{}, fmt.Errorf("this machine cannot read photos")
	}
	file, err := downloadPicture(ctx, imageURL)
	if err != nil {
		return PhotoReading{}, err
	}
	defer os.Remove(file)
	return readPhoto(ctx, fmtool, file)
}

// readingLines is a reading as the lines handed to the answerer: what the
// file says about itself first (so "when was it taken" has its answer among
// them), then the words in the picture.
func readingLines(p AnswerPhoto) []string {
	var lines []string
	if p.About != "" {
		lines = append(lines, "joined to: "+p.About)
	}
	if p.Read != nil {
		for _, k := range []string{"taken_at", "camera", "place", "size"} {
			if v := p.Read.Meta[k]; v != "" {
				lines = append(lines, k+": "+v)
			}
		}
		lines = append(lines, p.Read.Lines...)
	}
	return lines
}

// prepareAnswerPhotos finds the photos a request is about and gets each ready
// to be shown — downloaded, and read (once; kept on the thing after). Nil when
// the request is not about any.
func prepareAnswerPhotos(ctx context.Context, robot *Want, request string) []*robotPicture {
	if AnswerPhotoFinder == nil {
		return nil
	}
	question, _ := splitRobotRequest(request)
	photos := AnswerPhotoFinder(question)
	if len(photos) == 0 {
		return nil
	}
	// Read into fields when it was joined: nothing to download or ask.
	if v, id, ok := AnswerFromPhotoFieldsOf(question, photos); ok {
		robot.StoreLog("[ROBOT] answered from a photo's fields: %s", v)
		showAnswerEvidence(id)
		return []*robotPicture{{Question: question, Answer: v}}
	}
	out := prepareFoundPhotos(ctx, robot, question, photos)
	if len(out) > 0 {
		robot.StoreLog("[ROBOT] the question is about %d photo(s) joined to answers", len(out))
		RecordCCActivity(robot, CCActivityNote, fmt.Sprintf("写真 %d 枚を見ます", len(out)))
	}
	return out
}

// askAnswerPhotos asks about each photo in turn, newest answer first, until
// one answers; the answer says which answer's photo it came from. ok=false
// when none did — the question may not have been about the photos at all.
func askAnswerPhotos(server *fmServer, root string, timeout time.Duration, photos []*robotPicture, looking func(about string)) (fmReply, bool) {
	for _, p := range photos {
		if p.Answer != "" {
			return fmReply{Text: p.Answer}, true
		}
		if looking != nil {
			looking(p.About)
		}
		// A type that says how its photos are read: the model reads the words
		// by those rules (readByRules). Otherwise it looks at the photo and
		// picks one of the words in it.
		if p.Rules != "" {
			if answer, ok := readByRules(server, root, timeout, p); ok {
				r := fmReply{Text: answer}
				if p.About != "" {
					r.Text = fmt.Sprintf("%s（%s の写真より）", answer, p.About)
				}
				return r, true
			}
			continue
		}
		r, err := server.askPictureOrNone(p.prompt(), root, timeout, p.Image, p.Lines)
		if err != nil || r.None || strings.TrimSpace(r.Text) == "" {
			continue
		}
		r.Text = strings.TrimSpace(r.Text)
		if p.About != "" {
			r.Text = fmt.Sprintf("%s（%s の写真より）", r.Text, p.About)
		}
		return r, true
	}
	return fmReply{}, false
}

// photoNotHere is how the model says a photo does not answer the question.
const photoNotHere = "（この写真にはない）"

// readByRules asks the model a question of a photo's words, read by its
// type's rules — the words and the rules alone, no photo: asked with the photo
// and choosing among its cells, the model took a hole number (7) for a score
// card's total (87（+15）) every time, rules or not, where the rules read in
// words were followed. The answer is kept only if it is in the words — what
// was read, not what the model made up.
func readByRules(server *fmServer, root string, timeout time.Duration, p *robotPicture) (string, bool) {
	text := strings.Join(p.Lines, "\n")
	prompt := "Text read from a photo, line by line; cells on a line are separated by |:\n" + text +
		"\n\nHow to read this photo: " + p.Rules +
		"\n\nQuestion: " + p.Question +
		"\n\nAnswer with the value only, copied exactly as it is written in the text above, nothing else. If the text does not answer the question, answer " + photoNotHere + "."
	r, err := server.askPlain(prompt, root, timeout)
	if err != nil {
		return "", false
	}
	answer := strings.Trim(strings.TrimSpace(r.Text), "「」\"'`。. ")
	if answer == "" || answer == photoNotHere || !strings.Contains(text, answer) {
		return "", false
	}
	return answer, true
}

// AnswerFromPhotos answers a question from a want's answers' photos — read
// and asked as the robot on this machine asks (askAnswerPhotos) — for an
// answerer that cannot be shown a photo itself: the model on a phone or in a
// browser, which reaches the board only through its tools (the server's
// look). ok=false when this machine has no model to ask, or no photo answers.
func AnswerFromPhotos(ctx context.Context, robot *Want, question string, photos []AnswerPhoto) (string, bool) {
	// Read into fields when it was joined: answered from them, no model.
	if v, id, ok := AnswerFromPhotoFieldsOf(question, photos); ok {
		showAnswerEvidence(id)
		return v, true
	}
	binary, ok := fmToolPath()
	if !ok || len(photos) == 0 {
		return "", false
	}
	ready := prepareFoundPhotos(ctx, robot, question, photos)
	defer closePictures(ready)
	if len(ready) == 0 {
		return "", false
	}
	root, _ := os.UserHomeDir()
	reply, answered := askAnswerPhotos(fmServerFor(binary), root, fmDefaultTimeoutSeconds*time.Second, ready, nil)
	return reply.Text, answered
}

// AnswerReferentNote says, for a question that names a time of a want's
// answers (「今朝のゴルフ」), which answers it means — or "" when it names
// none. Set by the server (answer_referent.go).
var AnswerReferentNote func(question string) string

// withAnswerReferent is a request with what it refers to written after it,
// so the answer is about this morning's answer and not the want as it stands.
func withAnswerReferent(robot *Want, request string) string {
	if AnswerReferentNote == nil {
		return request
	}
	question, _ := splitRobotRequest(request)
	note := AnswerReferentNote(question)
	if note == "" {
		return request
	}
	robot.StoreLog("[ROBOT] the question refers to answers from a time: %s", firstLineOf(note))
	return request + "\n\n(" + strings.TrimSpace(note) + ")"
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// prepareFoundPhotos downloads each photo and reads it (once; kept on the
// thing after).
func prepareFoundPhotos(ctx context.Context, robot *Want, question string, photos []AnswerPhoto) []*robotPicture {
	if len(photos) > maxAnswerPhotos {
		photos = photos[:maxAnswerPhotos]
	}
	fmtool, haveTool := fmToolPath()
	var out []*robotPicture
	for _, p := range photos {
		dctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		file, err := downloadPicture(dctx, p.ImageURL)
		cancel()
		if err != nil {
			robot.StoreLog("[ROBOT] could not fetch the photo %s: %v", p.Name, err)
			continue
		}
		if p.Read == nil && haveTool {
			// The first run loads the recogniser, which takes several seconds;
			// after that a picture reads in well under one.
			rctx, cancel := context.WithTimeout(ctx, 45*time.Second)
			read, err := readPhoto(rctx, fmtool, file)
			cancel()
			if err != nil {
				robot.StoreLog("[ROBOT] could not read the photo %s: %v", p.Name, err)
			} else {
				p.Read = &read
				if AnswerPhotoReadSaver != nil {
					AnswerPhotoReadSaver(p.ID, p.ImageURL, read)
				}
			}
		}
		out = append(out, &robotPicture{Name: p.Name, About: p.About, Question: question, Rules: p.Rules, Image: file, Lines: readingLines(p)})
	}
	return out
}

// closeAll removes every photo's file.
func closePictures(ps []*robotPicture) {
	for _, p := range ps {
		p.close()
	}
}
