package types

import (
	"context"
	"encoding/json"
	"fmt"
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
	if len(out) > 0 {
		robot.StoreLog("[ROBOT] the question is about %d photo(s) joined to answers", len(out))
		RecordCCActivity(robot, CCActivityNote, fmt.Sprintf("写真 %d 枚を見ます", len(out)))
	}
	return out
}

// closeAll removes every photo's file.
func closePictures(ps []*robotPicture) {
	for _, p := range ps {
		p.close()
	}
}
