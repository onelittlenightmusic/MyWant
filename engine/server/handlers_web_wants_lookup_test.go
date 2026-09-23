package server

import (
	"testing"
	"time"
)

func TestRankWebWants(t *testing.T) {
	now := time.Now()
	cands := []webWantCandidate{
		{name: "google_home", sourceURL: "https://www.google.com/webhp", modified: now.Add(-3 * time.Hour)},
		{name: "google_images", sourceURL: "https://www.google.com/imghp", modified: now},
		{name: "yahoo_radar", sourceURL: "https://weather.yahoo.co.jp/weather/zoomradar/", modified: now},
		{name: "legacy_host_only", hosts: []string{"www.google.com"}, modified: now.Add(-1 * time.Hour)},
	}
	cases := []struct {
		page      string
		wantFirst string
		wantMatch int
		wantCount int
	}{
		// Trailing slash and fragment do not make it another page.
		{"https://weather.yahoo.co.jp/weather/zoomradar#tokyo", "yahoo_radar", webWantMatchURL, 1},
		// Exact source URL beats the same host.
		{"https://www.google.com/webhp", "google_home", webWantMatchURL, 3},
		// Same host and path, different query.
		{"https://www.google.com/webhp?hl=ja", "google_home", webWantMatchPath, 3},
		// Same host only: most recently saved first.
		{"https://www.google.com/search?q=x", "google_images", webWantMatchHost, 3},
		{"https://example.com/", "", webWantMatchNone, 0},
	}
	for _, c := range cases {
		page, ok := normalizePageURL(c.page)
		if !ok {
			t.Fatalf("%s: did not parse", c.page)
		}
		ranked, strengths := rankWebWants(page, cands)
		if len(ranked) != c.wantCount {
			t.Errorf("%s: %d matches, want %d", c.page, len(ranked), c.wantCount)
		}
		if c.wantCount == 0 {
			continue
		}
		if ranked[0].name != c.wantFirst || strengths[0] != c.wantMatch {
			t.Errorf("%s: first %s (%d), want %s (%d)", c.page, ranked[0].name, strengths[0], c.wantFirst, c.wantMatch)
		}
	}
}
