package server

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	types "mywant/engine/types"
)

// A photo thing's words, read when it is saved.
//
// What is written in a photo — a score card's rows, a receipt — and what its
// file says about itself (when it was taken, with what, where) is read once,
// as the photo becomes a thing, and kept on the thing as fields of their own:
// picture-text, one line per row, and picture-taken-at and the rest. A person
// sees them in the thing's panel; the robot answers from them without reading
// the photo again (answer_photos.go).
//
// Whoever can read does: this server when it has a recogniser (fmtool on a
// Mac), else the phone that shared the photo (the iPhone app's PhotoReader,
// which writes the same fields). picture-read-from says which image the words
// came from — a thing given a new picture is read again — and picture-read-by
// who read it.

const (
	thingPictureTextLabel   = "picture-text"
	thingPictureReadByLabel = "picture-read-by"
)

// thingPictureMetaLabels are the file's own facts, each as a field: the
// reading's meta key → the label.
var thingPictureMetaLabels = []struct{ meta, label string }{
	{"taken_at", "picture-taken-at"},
	{"camera", "picture-camera"},
	{"place", "picture-place"},
	{"size", "picture-size"},
}

// keepPhotoReading writes a reading onto its thing as fields, in place of
// whatever was read from it before.
func (s *Server) keepPhotoReading(id, imageURL, by string, r types.PhotoReading) {
	if s.thingLabels == nil {
		return
	}
	set := func(k, v string) {
		if v == "" {
			_ = s.thingLabels.Remove(id, k)
			return
		}
		_ = s.thingLabels.Set(id, k, v)
	}
	set(thingPictureTextLabel, strings.Join(r.Lines, "\n"))
	for _, m := range thingPictureMetaLabels {
		set(m.label, r.Meta[m.meta])
	}
	set(thingPictureReadFromLabel, imageURL)
	set(thingPictureReadByLabel, by)
	// The reading as one blob, from before it had fields.
	_ = s.thingLabels.Remove(id, thingPictureReadLabel)
	boardThingsChanged(id)
}

// photoReadingOf is what was read from a thing's photo, or nil when it was
// not read — from its fields, or the one blob older readings were kept as.
func photoReadingOf(labels map[string]string) *types.PhotoReading {
	r := types.PhotoReading{Meta: map[string]string{}}
	read := false
	if text, ok := labels[thingPictureTextLabel]; ok {
		read = true
		if text != "" {
			r.Lines = strings.Split(text, "\n")
		}
	}
	for _, m := range thingPictureMetaLabels {
		if v := labels[m.label]; v != "" {
			r.Meta[m.meta] = v
			read = true
		}
	}
	if read {
		return &r
	}
	if blob := labels[thingPictureReadLabel]; blob != "" {
		var old types.PhotoReading
		if json.Unmarshal([]byte(blob), &old) == nil {
			return &old
		}
	}
	return nil
}

// pictureReadCurrent says whether a thing's photo was read from the image it
// shows now.
func pictureReadCurrent(labels map[string]string) bool {
	return labels[thingPictureLabel] != "" && labels[thingPictureReadFromLabel] == labels[thingPictureLabel] && photoReadingOf(labels) != nil
}

// pictureReading keeps one read of each image at a time.
var pictureReading sync.Map

// readThingPictureSoon reads a thing's photo in the background and keeps the
// words on it — when this server can read (types.ReadPhoto); otherwise it is
// left to the phone that shared it.
func (s *Server) readThingPictureSoon(id, imageURL string) {
	if imageURL == "" || !types.CanReadPhotos() {
		return
	}
	if _, busy := pictureReading.LoadOrStore(id+"\x00"+imageURL, true); busy {
		return
	}
	go func() {
		defer pictureReading.Delete(id + "\x00" + imageURL)
		s.readThingPicture(id, imageURL)
	}()
}

func (s *Server) readThingPicture(id, imageURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r, err := types.ReadPhoto(ctx, imageURL)
	if err != nil {
		log.Printf("[things] could not read the photo of %s: %v", id, err)
		return
	}
	// Still the same picture: one given a new image meanwhile is read anew.
	if s.thingLabels.All()[id][thingPictureLabel] != imageURL {
		return
	}
	s.keepPhotoReading(id, imageURL, "mywant", r)
	log.Printf("[things] read the photo of %s: %d lines", id, len(r.Lines))
}

// readUnreadPictures brings every photo thing of the world just opened to
// fields (useWorldThings — at start, and on a switch of world): a
// reading kept as one blob is written out as fields; a photo never read is
// read now, one after another, when this server can read.
func (s *Server) readUnreadPictures() {
	if s.thingLabels == nil {
		return
	}
	go func() {
		for id, labels := range s.thingLabels.All() {
			url := labels[thingPictureLabel]
			if url == "" {
				continue
			}
			if labels[thingPictureReadLabel] != "" && labels[thingPictureReadFromLabel] == url {
				if r := photoReadingOf(map[string]string{thingPictureReadLabel: labels[thingPictureReadLabel]}); r != nil {
					by := labels[thingPictureReadByLabel]
					if by == "" {
						by = "mywant"
					}
					s.keepPhotoReading(id, url, by, *r)
					continue
				}
			}
			if pictureReadCurrent(labels) || !types.CanReadPhotos() {
				continue
			}
			s.readThingPicture(id, url)
		}
		// The photos just given their words, read into fields when joined.
		s.readPhotoFieldsSoon()
	}()
}
