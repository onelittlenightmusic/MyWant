package server

import "testing"

func TestPhotoReadingOf(t *testing.T) {
	// From fields.
	r := photoReadingOf(map[string]string{
		thingPictureTextLabel: "SCORE CARD\nPlayer 1 | 87（+15）",
		"picture-taken-at":    "2026-10-17 09:39",
	})
	if r == nil || len(r.Lines) != 2 || r.Lines[1] != "Player 1 | 87（+15）" || r.Meta["taken_at"] != "2026-10-17 09:39" {
		t.Fatalf("fields: got %+v", r)
	}
	// Read, and nothing written in it: still read.
	if r := photoReadingOf(map[string]string{thingPictureTextLabel: ""}); r == nil || len(r.Lines) != 0 {
		t.Fatalf("empty text: got %+v", r)
	}
	// From the one blob older readings were kept as.
	r = photoReadingOf(map[string]string{thingPictureReadLabel: `{"lines":["a","b"],"meta":{"size":"1200x900"}}`})
	if r == nil || len(r.Lines) != 2 || r.Meta["size"] != "1200x900" {
		t.Fatalf("blob: got %+v", r)
	}
	if photoReadingOf(map[string]string{thingPictureLabel: "https://x/y.jpg"}) != nil {
		t.Fatal("never read: want nil")
	}
}
