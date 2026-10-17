package types

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
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
		if looking != nil {
			looking(p.About)
		}
		// A score card's total is read off it, not asked for: the model,
		// handed the card's words, once answered 87（+15） and once 7 — a
		// stray cell — for the same photo and question.
		if total := scoreCardTotal(p.Question, p.Lines); total != "" {
			text := total
			if p.About != "" {
				text = fmt.Sprintf("%s（%s の写真より）", total, p.About)
			}
			return fmReply{Text: text}, true
		}
		r, err := server.askPictureOrNone(p.Question, root, timeout, p.Image, p.Lines)
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

// scoreWords are how a question asks for a score.
var scoreWords = []string{"スコア", "score", "打数", "成績"}

// scoreCardTotalRe is a score card's total as the recogniser reads it: the
// strokes and, in brackets, against par — 87（+15）, 71（-1）, 72（E）.
var scoreCardTotalRe = regexp.MustCompile(`(\d{2,3})\s*[（(]\s*([+＋\-－−]\s*\d{1,2}|E|±0)\s*[）)]`)

// scoreCardTotal is the total on a score card a question asks the score of,
// or "" when the question is not after a score or the words hold no total —
// then the model is asked, as for any other photo. The last total read wins:
// a card lists the player's line after the course's.
func scoreCardTotal(question string, lines []string) string {
	if !containsAny(strings.ToLower(question), scoreWords...) {
		return ""
	}
	total := ""
	for _, l := range lines {
		if m := scoreCardTotalRe.FindAllStringSubmatch(l, -1); len(m) > 0 {
			last := m[len(m)-1]
			sign := strings.NewReplacer("＋", "+", "－", "-", "−", "-", " ", "").Replace(last[2])
			total = last[1] + "（" + sign + "）"
		}
	}
	return total
}

// AnswerFromPhotos answers a question from a want's answers' photos — read
// and asked as the robot on this machine asks (askAnswerPhotos) — for an
// answerer that cannot be shown a photo itself: the model on a phone or in a
// browser, which reaches the board only through its tools (the server's
// look). ok=false when no reading has a score and this machine has no model
// to ask, or no photo answers.
func AnswerFromPhotos(ctx context.Context, robot *Want, question string, photos []AnswerPhoto) (string, bool) {
	// A score is read off a card already read — by this machine before, or by
	// the phone that shared it (its PhotoReader) — with no model at all, so a
	// server without one (Fly) answers it too.
	for _, p := range photos {
		if p.Read == nil {
			continue
		}
		if total := scoreCardTotal(question, readingLines(p)); total != "" {
			if p.About != "" {
				total = fmt.Sprintf("%s（%s の写真より）", total, p.About)
			}
			return total, true
		}
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
		out = append(out, &robotPicture{Name: p.Name, About: p.About, Question: question, Image: file, Lines: readingLines(p)})
	}
	return out
}

// closeAll removes every photo's file.
func closePictures(ps []*robotPicture) {
	for _, p := range ps {
		p.close()
	}
}
