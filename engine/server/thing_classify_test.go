package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	mwlabels "mywant/engine/labels"
)

func TestClassifyLink(t *testing.T) {
	cases := []struct {
		link, subtype, value string
	}{
		{"https://photos.app.goo.gl/AbCdEf123", "image_url", ""},
		{"https://photos.google.com/share/AF1Qip?key=x", "image_url", ""},
		{"https://lh3.googleusercontent.com/pw/AP1Gcz=w600", "image_url", ""},
		{"https://example.com/a/b/photo.JPG", "image_url", ""},
		{"https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC", "song", ""},
		{"https://open.spotify.com/intl-ja/album/1DFixLWuPkv3KT3TnV35m3", "album", ""},
		{"https://open.spotify.com/artist/0OdUWJ0sBjDrqHygGUXeCF", "artist", ""},
		{"https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M", "music", ""},
		{"https://music.apple.com/jp/album/lemon/1374931396?i=1374931398", "song", ""},
		{"https://music.apple.com/jp/album/lemon/1374931396", "album", ""},
		{"https://music.youtube.com/watch?v=abc", "song", ""},
		{"https://soundcloud.com/someone", "artist", ""},
		{"https://soundcloud.com/someone/a-track", "song", ""},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "movie", ""},
		{"https://m.youtube.com/shorts/abc", "movie", ""},
		{"https://youtu.be/dQw4w9WgXcQ", "movie", ""},
		{"https://www.youtube.com/@channel", "url", ""},
		{"https://example.com/clip.mp4", "movie", ""},
		{"https://example.com/voice.m4a", "song", ""},
		{"https://booklog.jp/item/1/4101010013", "book", ""},
		{"https://play.google.com/store/books/details?id=x", "book", ""},
		{"https://www.booking.com/hotel/jp/park-hyatt-tokyo.html", "hotel", ""},
		{"https://www.airbnb.jp/rooms/123", "hotel", ""},
		{"https://www.amazon.co.jp/dp/B0ABCDEF12", "product", ""},
		{"https://amzn.asia/d/abc", "product", ""},
		{"https://jp.mercari.com/item/m123", "product", ""},
		{"https://peatix.com/event/123", "event", ""},
		{"https://www.linkedin.com/jobs/view/123", "job", ""},
		{"https://tabelog.com/tokyo/A1301/A130101/13000001/", "place", ""},
		{"https://maps.apple.com/?q=%E6%9D%B1%E4%BA%AC%E9%A7%85&ll=35.68,139.76", "place", "東京駅"},
		{"https://www.google.com/maps/place/Tokyo+Tower/@35.6,139.7,17z", "place", "Tokyo Tower"},
		{"https://maps.app.goo.gl/xyz", "place", ""},
		{"https://example.com/article", "url", ""},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.link)
		got := classifyLink(u)
		want := c.value
		if want == "" {
			want = u.String()
		}
		if got.Subtype != c.subtype || got.Value != want {
			t.Errorf("%s: got %s %q, want %s %q", c.link, got.Subtype, got.Value, c.subtype, want)
		}
	}
}

func TestClassifyText(t *testing.T) {
	cases := map[string]string{
		"someone@example.com":  "email",
		"090-1234-5678":        "phone",
		"+81 3 1234 5678":      "phone",
		"2026-10-08":           "date",
		"2026年10月8日":           "date",
		"2026/10/08 19:30":     "datetime",
		"東京都千代田区丸の内1-9-1":      "address",
		"〒100-0005 千代田区丸の内1丁目": "address",
		"今日の夕飯":                "string",
		"1234":                 "string",
	}
	for text, want := range cases {
		if got := classifyText(text).Subtype; got != want {
			t.Errorf("%q: got %s, want %s", text, got, want)
		}
	}
}

func TestClassifySharedTextWithLinks(t *testing.T) {
	// Safari: the page's title as text, the link as a URL. Google Photos: the
	// links in one text. The caption is not a thing; each link is.
	got := classifyShared(context.Background(), http.DefaultClient,
		[]string{"https://youtu.be/a"},
		[]string{"見てほしい写真\nhttps://photos.app.goo.gl/one\nhttps://photos.app.goo.gl/two。", "https://youtu.be/a"})
	want := []sharedThing{
		{"movie", "https://youtu.be/a"},
		{"image_url", "https://photos.app.goo.gl/one"},
		{"image_url", "https://photos.app.goo.gl/two"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestClassifySharedPeeksUnknownLinks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/song", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><meta content="music.song" property="og:type"></head><body></body></html>`))
	})
	mux.HandleFunc("/film", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<head><meta property="og:type" content="video.movie"></head>`))
	})
	mux.HandleFunc("/picture", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{0x89, 'P', 'N', 'G'})
	})
	mux.HandleFunc("/article", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<head><meta property="og:type" content="article"></head>`))
	})
	mux.HandleFunc("/short", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dp/B0ABCDEF12", http.StatusFound)
	})
	mux.HandleFunc("/dp/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<head><meta property="og:type" content="product"></head>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	links := []string{srv.URL + "/song", srv.URL + "/film", srv.URL + "/picture", srv.URL + "/article", srv.URL + "/short", srv.URL + "/gone"}
	got := classifyShared(context.Background(), srv.Client(), links, nil)
	want := []string{"song", "movie", "image_url", "url", "product", "url"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i, w := range want {
		if got[i].Subtype != w || got[i].Value != links[i] {
			t.Errorf("%s: got %v, want %s with the shared link", links[i], got[i], w)
		}
	}
}

func TestShareContentMakesAndPinsThings(t *testing.T) {
	dir := t.TempDir()
	s := &Server{
		thingStore:  &ThingStore{path: filepath.Join(dir, "memo.yaml")},
		thingLabels: &ThingLabelStore{mwlabels.NewFileStore(filepath.Join(dir, "memo-labels.yaml"))},
	}
	share := func() []sharedThingResult {
		body := `{"texts":["https://photos.app.goo.gl/one https://photos.app.goo.gl/two"],"urls":["https://open.spotify.com/track/x"]}`
		rec := httptest.NewRecorder()
		s.shareContent(rec, httptest.NewRequest(http.MethodPost, "/api/v1/things/share", strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("HTTP %d: %s", rec.Code, rec.Body)
		}
		var got struct{ Things []sharedThingResult }
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got.Things
	}

	first := share()
	want := []string{"song", "image_url", "image_url"}
	if len(first) != len(want) {
		t.Fatalf("got %+v", first)
	}
	for i, th := range first {
		if th.Subtype != want[i] || th.ID == "" || !th.Pinned || th.Error != "" {
			t.Errorf("[%d] got %+v, want a pinned %s", i, th, want[i])
		}
		if got := s.thingLabels.Get(th.ID)[thingCanvasPinLabel]; got != "true" {
			t.Errorf("[%d] pin label = %q", i, got)
		}
	}
	if first[1].Catalog != "image_urls" || first[0].Icon != "Music2" {
		t.Errorf("catalog/icon: %+v", first)
	}

	// Shared again: the same things, not new ones.
	again := share()
	for i := range again {
		if again[i].ID != first[i].ID {
			t.Errorf("[%d] shared again made a new thing: %s != %s", i, again[i].ID, first[i].ID)
		}
	}
}
