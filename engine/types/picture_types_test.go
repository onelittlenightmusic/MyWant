package types

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "mywant/engine/core"
)

func TestOgImage(t *testing.T) {
	page := `<html><head>
<meta property="og:title" content="New photo">
<meta property="og:image" content="https://lh3.googleusercontent.com/pw/AbC-123=w600-h315-p-k">
</head></html>`
	if got := ogImage(page); got != "https://lh3.googleusercontent.com/pw/AbC-123=w600-h315-p-k" {
		t.Fatalf("ogImage = %q", got)
	}
	// Attribute order reversed, and an escaped ampersand.
	page2 := `<meta content="https://example.com/a.jpg?x=1&amp;y=2" property="og:image" />`
	if got := ogImage(page2); got != "https://example.com/a.jpg?x=1&y=2" {
		t.Fatalf("ogImage reversed = %q", got)
	}
	if got := ogImage(`<meta property="og:title" content="x">`); got != "" {
		t.Fatalf("ogImage without image = %q", got)
	}
}

func TestSizeGooglePhoto(t *testing.T) {
	cases := map[string]string{
		"https://lh3.googleusercontent.com/pw/AbC-123=w600-h315-p-k": "https://lh3.googleusercontent.com/pw/AbC-123=w1600",
		"https://lh3.googleusercontent.com/pw/AbC-123":               "https://lh3.googleusercontent.com/pw/AbC-123=w1600",
		"https://example.com/photo.jpg":                              "https://example.com/photo.jpg",
	}
	for in, want := range cases {
		if got := sizeGooglePhoto(in, 1600); got != want {
			t.Errorf("sizeGooglePhoto(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolvePictureURLDirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/p.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("\x89PNG"))
		case "/login":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte("<html>sign in</html>"))
		case "/moved":
			http.Redirect(w, r, "/login", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	if got, err := resolvePictureURL(ctx, srv.Client(), srv.URL+"/p.png"); err != nil || got != srv.URL+"/p.png" {
		t.Fatalf("image = %q, %v", got, err)
	}
	// A page, a redirect to a sign-in page, a dead link: none is a picture.
	for _, path := range []string{"/login", "/moved", "/gone"} {
		if got, err := resolvePictureURL(ctx, srv.Client(), srv.URL+path); err == nil {
			t.Errorf("%s resolved to %q, want an error", path, got)
		}
	}
	if _, err := resolvePictureURL(ctx, srv.Client(), "not a url"); err == nil {
		t.Fatal("expected an error for a non-URL")
	}
}

func TestResolvePictureURLLibraryLink(t *testing.T) {
	// Refused before anything is fetched: a nil client would panic if it were.
	var noClient *http.Client
	_, err := resolvePictureURL(context.Background(), noClient, "https://photos.google.com/photo/AF1QipExample")
	if err == nil || !strings.Contains(err.Error(), "Create link") {
		t.Fatalf("library link err = %v", err)
	}
}

func TestClampPicturePin(t *testing.T) {
	cases := []struct{ dx, dy, wx, wy float64 }{
		{0, 0, 1, 0},   // never under its own tile
		{2, -1, 2, -1}, // within reach
		{9, 0, 5, 0},   // pulled back to the edge
		{1.4, 2.6, 1, 3},
	}
	for _, c := range cases {
		x, y := clampPicturePin(c.dx, c.dy)
		if x != c.wx || y != c.wy {
			t.Errorf("clampPicturePin(%v,%v) = (%v,%v), want (%v,%v)", c.dx, c.dy, x, y, c.wx, c.wy)
		}
	}
}

func TestPictureAnswersAndFeedback(t *testing.T) {
	p := &PictureWant{Want: Want{
		Metadata:    Metadata{ID: "pic-id", Name: "score-photo", Type: "picture"},
		Spec:        WantSpec{Params: map[string]any{}},
		StateLabels: map[string]StateLabel{"answers": LabelCurrent},
	}}
	// Each webhook lands in a cycle of its own, as it does on a running want.
	cycle := func(f func()) { p.BeginProgressCycle(); f(); p.EndProgressCycle() }

	cycle(func() {
		if !p.recordAnswer(map[string]any{"id": "a1", "question": "スコアは？", "answer": "60（E）", "by": "robot"}) {
			t.Fatal("record_answer refused")
		}
		if p.recordAnswer(map[string]any{"id": "a2", "question": "?"}) {
			t.Fatal("an answer with nothing in it was kept")
		}
	})
	cycle(func() {
		if !p.recordFeedback(map[string]any{"id": "a1", "value": "good"}) {
			t.Fatal("feedback refused")
		}
		if p.recordFeedback(map[string]any{"id": "a1", "value": "maybe"}) {
			t.Fatal("feedback other than good/wrong/empty was kept")
		}
		if p.recordFeedback(map[string]any{"id": "nope", "value": "wrong"}) {
			t.Fatal("feedback on an answer that does not exist was kept")
		}
	})

	answers := pictureAnswers(&p.Want)
	if len(answers) != 1 {
		t.Fatalf("answers = %v", answers)
	}
	a := answers[0]
	if a["answer"] != "60（E）" || a["question"] != "スコアは？" || a["by"] != "robot" || a["feedback"] != "good" {
		t.Fatalf("answer = %v", a)
	}

	// Taken back.
	cycle(func() { p.recordFeedback(map[string]any{"id": "a1", "value": ""}) })
	if pictureAnswers(&p.Want)[0]["feedback"] != "" {
		t.Fatal("feedback was not cleared")
	}
}
