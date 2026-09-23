package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mywant "mywant/engine/core"
)

// webWantLookupResponse is the payload of GET /api/v1/web-wants/lookup.
type webWantLookupResponse struct {
	Found     bool             `json:"found"`
	Name      string           `json:"name,omitempty"`
	Title     string           `json:"title,omitempty"`
	SourceURL string           `json:"source_url,omitempty"`
	Elements  []WebWantElement `json:"elements,omitempty"`
	// How the page matched: "url" (the page it was captured from), "path"
	// (same host and path) or "host".
	Match string `json:"match,omitempty"`
	// Other saved types that also match this page, best first — a host can
	// have several (Google's search page and its images page, say).
	Others []string `json:"others,omitempty"`
}

// webWantCandidate is one saved web want type as the lookup sees it.
type webWantCandidate struct {
	name      string
	title     string
	sourceURL string
	hosts     []string // hostnames elements.json is keyed by
	modified  time.Time
}

// Match strength, strongest first.
const (
	webWantMatchNone = iota
	webWantMatchHost
	webWantMatchPath
	webWantMatchURL
)

var webWantMatchNames = map[int]string{
	webWantMatchHost: "host",
	webWantMatchPath: "path",
	webWantMatchURL:  "url",
}

// normalizePageURL reduces a URL to what makes it the same page for this
// purpose: scheme and host lower-cased, no fragment, no trailing slash. The
// query stays — for most captured pages it is the page (a search, an album).
func normalizePageURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path != "/" {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	u.RawPath = ""
	return u, true
}

// matchWebWant says how strongly a saved type matches the page at page.
func matchWebWant(page *url.URL, c webWantCandidate) int {
	if src, ok := normalizePageURL(c.sourceURL); ok {
		if src.String() == page.String() {
			return webWantMatchURL
		}
		if src.Hostname() == page.Hostname() && src.Path == page.Path {
			return webWantMatchPath
		}
		if src.Hostname() == page.Hostname() {
			return webWantMatchHost
		}
	}
	for _, h := range c.hosts {
		if strings.EqualFold(h, page.Hostname()) {
			return webWantMatchHost
		}
	}
	return webWantMatchNone
}

// rankWebWants returns the candidates that match page, strongest match first
// and, within one strength, the most recently saved first.
func rankWebWants(page *url.URL, cands []webWantCandidate) (ranked []webWantCandidate, strengths []int) {
	type scored struct {
		c webWantCandidate
		s int
	}
	var hits []scored
	for _, c := range cands {
		if s := matchWebWant(page, c); s != webWantMatchNone {
			hits = append(hits, scored{c, s})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].s != hits[j].s {
			return hits[i].s > hits[j].s
		}
		return hits[i].c.modified.After(hits[j].c.modified)
	})
	for _, h := range hits {
		ranked = append(ranked, h.c)
		strengths = append(strengths, h.s)
	}
	return ranked, strengths
}

// webWantCandidates reads every saved web want type: a user custom type with an
// elements.json beside it, which is what makes it a web want rather than any
// other custom type.
func (s *Server) webWantCandidates() []webWantCandidate {
	root := mywant.UserCustomTypesDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []webWantCandidate
	for _, e := range entries {
		if !e.IsDir() || !validTypeName.MatchString(e.Name()) {
			continue
		}
		elemPath := filepath.Join(root, e.Name(), "elements.json")
		info, err := os.Stat(elemPath)
		if err != nil {
			continue
		}
		c := webWantCandidate{name: e.Name(), title: e.Name(), modified: info.ModTime()}
		if data, err := os.ReadFile(elemPath); err == nil {
			var byHost map[string]json.RawMessage
			if json.Unmarshal(data, &byHost) == nil {
				for h := range byHost {
					c.hosts = append(c.hosts, h)
				}
			}
		}
		if s.wantTypeLoader != nil {
			if def := s.wantTypeLoader.GetDefinition(e.Name()); def != nil {
				if def.Metadata.Title != "" {
					c.title = def.Metadata.Title
				}
				c.sourceURL = def.Metadata.Labels["source-url"]
			}
		}
		out = append(out, c)
	}
	return out
}

// lookupWebWant handles GET /api/v1/web-wants/lookup?url=<page> — whether the
// page a browser is on has already been saved as a web want, and if so which
// one and with what elements, so the extension's sidebar can open on it
// (and save over it) instead of starting empty and making a duplicate.
//
// Always 200: "not saved" is an answer ({found:false}), not an error.
func (s *Server) lookupWebWant(w http.ResponseWriter, r *http.Request) {
	page, ok := normalizePageURL(r.URL.Query().Get("url"))
	if !ok {
		s.JSONResponse(w, http.StatusOK, webWantLookupResponse{Found: false})
		return
	}
	ranked, strengths := rankWebWants(page, s.webWantCandidates())
	if len(ranked) == 0 {
		s.JSONResponse(w, http.StatusOK, webWantLookupResponse{Found: false})
		return
	}
	best := ranked[0]
	resp := webWantLookupResponse{
		Found:     true,
		Name:      best.name,
		Title:     best.title,
		SourceURL: best.sourceURL,
		Match:     webWantMatchNames[strengths[0]],
	}
	if data, err := os.ReadFile(filepath.Join(mywant.UserCustomTypesDir(), best.name, "elements.json")); err == nil {
		var byHost map[string][]WebWantElement
		if json.Unmarshal(data, &byHost) == nil {
			for _, elems := range byHost {
				resp.Elements = append(resp.Elements, elems...)
			}
		}
	}
	for _, c := range ranked[1:] {
		resp.Others = append(resp.Others, c.name)
	}
	s.JSONResponse(w, http.StatusOK, resp)
}
