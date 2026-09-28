package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
	"mywant/engine/types"
)

// WebWantElement describes a single interactive element captured by the inspector.
type WebWantElement struct {
	Role     string `json:"role"`
	Name     string `json:"name"`
	Selector string `json:"selector,omitempty"`
	FieldKey string `json:"field_key,omitempty"` // ASCII param key for textbox inputs (empty for buttons)
	HtmlName string `json:"html_name,omitempty"` // HTML name attribute of the element (e.g. "q" for Google search)
	// Where the element was on the page when it was saved, and what it looked
	// like there: its box in document CSS pixels, and that box cut out of the
	// save-time screenshot as a small JPEG data URL. Kept so each object can be
	// drawn with its own picture (the sidebar's cards use it as a background).
	// Image is absent when the element was off screen at save time, or the
	// saver could not take a screenshot (the bookmarklet).
	Rect  *WebWantRect `json:"rect,omitempty"`
	Image string       `json:"image,omitempty"`
}

// WebWantRect is an element's box in document CSS pixels.
type WebWantRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// createWebWantRequest is the body for POST /api/v1/web-wants/create.
type createWebWantRequest struct {
	Name          string                      `json:"name"`
	Title         string                      `json:"title,omitempty"`
	URL           string                      `json:"url"`
	Hostname      string                      `json:"hostname,omitempty"`
	Elements      []WebWantElement            `json:"elements"`
	AllData       map[string][]WebWantElement `json:"all_data,omitempty"`
	URLTemplate   string                      `json:"url_template,omitempty"`
	ScreenshotURL string                      `json:"screenshot_url,omitempty"`
}

var validTypeName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var reName = regexp.MustCompile(`\[name=["']?([^"'\]]+)["']?\]`)
var reID = regexp.MustCompile(`\[id=["']?([^"'\]]+)["']?\]`)
var reNonASCII = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// isInputRole returns true for roles that accept text input.
func isInputRole(role string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	switch r {
	case "textbox", "searchbox", "combobox", "spinbutton", "slider", "input":
		return true
	}
	return false
}

// fieldKeyFromSelector derives an ASCII field key from a CSS selector.
func fieldKeyFromSelector(sel string) string {
	if m := reName.FindStringSubmatch(sel); len(m) > 1 {
		return sanitizeFieldKey(m[1])
	}
	if strings.HasPrefix(sel, "#") {
		return sanitizeFieldKey(strings.TrimPrefix(sel, "#"))
	}
	if m := reID.FindStringSubmatch(sel); len(m) > 1 {
		return sanitizeFieldKey(m[1])
	}
	return ""
}

// sanitizeFieldKey makes a name usable as a field key — which is also a
// parameter name, so it must match ^[a-z][a-z0-9_]*$. A name with nothing
// ASCII in it (ポスト本文) used to come out as "_____": a key that says nothing,
// and one no parameter may be called. It now comes out empty, so the caller
// falls back to the selector or a numbered key.
func sanitizeFieldKey(s string) string {
	r := strings.ToLower(reNonASCII.ReplaceAllString(s, "_"))
	r = reRepeatedUnderscore.ReplaceAllString(r, "_")
	r = strings.Trim(r, "_")
	if len(r) == 0 {
		return ""
	}
	if r[0] >= '0' && r[0] <= '9' {
		r = "f_" + r
	}
	return r
}

var reRepeatedUnderscore = regexp.MustCompile(`_+`)

// oneLine collapses every run of whitespace — newlines included — to a single
// space. An element's name is its visible text, which on a card or a button
// spanning two lines carries the line break and the indentation after it; the
// name is a title, not a paragraph, and written raw into the type's YAML
// comments a newline ends the comment and breaks the file.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// enrichElements assigns FieldKey to every captured element.
// Input roles  → field key derived from selector/name (e.g. "email").
// Button roles → "click_" + sanitized element name (e.g. "click_login").
func enrichElements(elements []WebWantElement) []WebWantElement {
	usedKeys := map[string]bool{}
	result := make([]WebWantElement, len(elements))
	autoInputIdx := 0
	autoButtonIdx := 0
	for i, el := range elements {
		el.Name = oneLine(el.Name)
		enriched := el
		if isInputRole(el.Role) {
			key := sanitizeFieldKey(el.Name)
			if key == "" {
				key = fieldKeyFromSelector(el.Selector)
			}
			if key == "" {
				autoInputIdx++
				key = fmt.Sprintf("field_%d", autoInputIdx)
			}
			base := key
			for n := 2; usedKeys[key]; n++ {
				key = fmt.Sprintf("%s_%d", base, n)
			}
			usedKeys[key] = true
			enriched.FieldKey = key
		} else {
			// Button / link / other clickable: derive key from element name
			key := ""
			if el.Name != "" {
				key = "click_" + sanitizeFieldKey(el.Name)
			}
			if key == "" || key == "click_" {
				autoButtonIdx++
				key = fmt.Sprintf("click_btn_%d", autoButtonIdx)
			}
			base := key
			for n := 2; usedKeys[key]; n++ {
				key = fmt.Sprintf("%s_%d", base, n)
			}
			usedKeys[key] = true
			enriched.FieldKey = key
		}
		result[i] = enriched
	}
	return result
}

// createWebWant handles POST /api/v1/web-wants/create
func (s *Server) createWebWant(w http.ResponseWriter, r *http.Request) {
	var req createWebWantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.ToLower(strings.ReplaceAll(req.Name, " ", "_"))
	name = strings.ReplaceAll(name, "-", "_")
	if !validTypeName.MatchString(name) {
		http.Error(w, "invalid name: must match [a-z][a-z0-9_-]{0,63}", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	title := req.Title
	if title == "" {
		title = req.Name
	}

	hostname := req.Hostname
	if hostname == "" && req.URL != "" {
		u := req.URL
		if i := strings.Index(u, "://"); i >= 0 {
			u = u[i+3:]
		}
		if i := strings.Index(u, "/"); i >= 0 {
			u = u[:i]
		}
		hostname = u
	}

	elements := req.Elements
	if elements == nil && req.AllData != nil {
		if v, ok := req.AllData[hostname]; ok {
			elements = v
		}
	}

	dir, loaded, warnings, err := s.writeWebWantType(name, title, req.URL, hostname, req.URLTemplate, req.ScreenshotURL, elements)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.globalBuilder.LogAPIOperation("POST", "/web-wants/create", name, "created", loaded, "", "")
	go broadcastSSE("want_type_changed", name)

	s.JSONResponse(w, http.StatusCreated, map[string]any{
		"name":     name,
		"dir":      dir,
		"loaded":   loaded,
		"warnings": warnings,
		"message":  fmt.Sprintf("web want type %q created successfully", name),
	})
}

// frameRewrites mirrors FRAME_REWRITES in the GUI's WebFrameCardPlugin: hosts
// that refuse to be framed as-is but have a way in. Probed through the same
// door the card will use, or Google would be reported as unframeable.
var frameRewrites = map[string]func(u *url.URL){
	"www.google.com": func(u *url.URL) { q := u.Query(); q.Set("igu", "1"); u.RawQuery = q.Encode() },
	"google.com":     func(u *url.URL) { q := u.Query(); q.Set("igu", "1"); u.RawQuery = q.Encode() },
}

// probeFrameBlocked reports whether pageURL tells browsers not to show it
// inside another site's iframe (X-Frame-Options, or a CSP frame-ancestors that
// does not allow everyone). A browser cannot find this out afterwards — a
// refused cross-origin frame looks the same as a slow one — so it is asked
// once, here, when the type is made. Anything that stops the question being
// answered (network, timeout) counts as "not blocked": the card then tries the
// frame, which is what it did before this existed.
func probeFrameBlocked(pageURL string) bool {
	u, err := url.Parse(pageURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if rw, ok := frameRewrites[u.Hostname()]; ok {
		rw(u)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return frameBlockedByHeaders(resp.Header)
}

func frameBlockedByHeaders(h http.Header) bool {
	switch strings.ToUpper(strings.TrimSpace(h.Get("X-Frame-Options"))) {
	case "DENY", "SAMEORIGIN":
		return true
	}
	for _, csp := range h.Values("Content-Security-Policy") {
		for _, directive := range strings.Split(csp, ";") {
			fields := strings.Fields(strings.ToLower(directive))
			if len(fields) == 0 || fields[0] != "frame-ancestors" {
				continue
			}
			for _, src := range fields[1:] {
				if src == "*" {
					return false
				}
			}
			return true
		}
	}
	return false
}

// writeWebWantType writes a web want type's on-disk artifacts (elements.json,
// <name>.yaml, main.py, SKILL.md) under UserCustomTypesDir()/<name>/ and reloads
// the type registry. name must already be validated against validTypeName.
// Shared by createWebWant (GUI-driven) and captureWebWant (bookmarklet-driven).

// persistWebScreenshot turns a captured page's screenshot into a file and
// returns the path that serves it.
//
// The extension sends the shot as a data: URI, and it used to be stored as one
// — in the want type's own YAML, where a few hundred kilobytes of base64 sat
// in a file otherwise made of field definitions, and where nothing else could
// point at it. A file in ~/.mywant/screenshots (already served, already how
// replay keeps its own shots) is both smaller in the type and referenceable
// from elsewhere, which is what lets the page's url THING wear it too.
//
// Anything unexpected returns the input unchanged: a capture whose shot cannot
// be decoded should still produce a want type, with the data URI it always had.
func (s *Server) persistWebScreenshot(name, dataURI string) string {
	// Named after the want type, so a re-capture replaces its own shot rather
	// than leaving the old one behind.
	return persistScreenshotFile("web-"+name, dataURI)
}

// persistScreenshotFile writes a data: URI image to ~/.mywant/screenshots as
// base.<ext> and returns the path that serves it; anything it cannot decode is
// returned unchanged. The serving route only accepts [A-Za-z0-9-_.], which a
// generated type name and field key already are.
func persistScreenshotFile(base, dataURI string) string {
	if !strings.HasPrefix(dataURI, "data:image/") {
		return dataURI // already a path — a re-capture, or a future sender
	}
	comma := strings.Index(dataURI, ",")
	if comma < 0 {
		return dataURI
	}
	meta := dataURI[len("data:"):comma]
	if !strings.Contains(meta, ";base64") {
		return dataURI
	}
	ext := "png"
	switch {
	case strings.HasPrefix(meta, "image/jpeg"), strings.HasPrefix(meta, "image/jpg"):
		ext = "jpg"
	case strings.HasPrefix(meta, "image/webp"):
		ext = "webp"
	}
	raw, err := base64.StdEncoding.DecodeString(dataURI[comma+1:])
	if err != nil || len(raw) == 0 {
		return dataURI
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return dataURI
	}
	dir := filepath.Join(home, ".mywant", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return dataURI
	}
	file := base + "." + ext
	if err := os.WriteFile(filepath.Join(dir, file), raw, 0o644); err != nil {
		return dataURI
	}
	return "/api/v1/screenshots/" + file
}

// labelThingWithScreenshot gives the page's url thing the picture the capture
// just took, as a label on the thing itself.
//
// That is what the Thing card reads (`background: "@screenshot-url"` on the url
// data type), and a label is where it belongs: the shot is this page's, not
// every page's. Only the path is stored, never the image — the picture stays
// the one file persistWebScreenshot wrote.
//
// Quietly does nothing when the page has never been named: there is no thing to
// label yet, and the next capture of that page will do it. Nothing is created
// here, because naming is the user's act and a screenshot is not a name.
func (s *Server) labelThingWithScreenshot(pageURL, servedPath string) {
	if pageURL == "" || servedPath == "" || strings.HasPrefix(servedPath, "data:") {
		return
	}
	for _, e := range s.thingStore.Entries() {
		if e.Value != pageURL || keyToSubtype(e.Catalog) != "url" {
			continue
		}
		if err := s.thingLabels.Set(e.ID, "screenshot-url", servedPath); err == nil {
			go broadcastSSE("thing_changed", e.ID)
		}
		return
	}
}

func (s *Server) writeWebWantType(name, title, pageURL, hostname, urlTemplate, screenshotURL string, elements []WebWantElement) (dir string, loaded int, warnings []string, err error) {
	// Enrich elements with field_key for textbox-like roles
	elements = enrichElements(elements)

	// The shot becomes a file, and the page's own thing learns where it is.
	if screenshotURL != "" {
		screenshotURL = s.persistWebScreenshot(name, screenshotURL)
		s.labelThingWithScreenshot(pageURL, screenshotURL)
	}

	dir = filepath.Join(mywant.UserCustomTypesDir(), name)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Write elements.json (includes field_key for textboxes)
	elemJSON, _ := json.MarshalIndent(map[string][]WebWantElement{hostname: elements}, "", "  ")
	if err = os.WriteFile(filepath.Join(dir, "elements.json"), elemJSON, 0o644); err != nil {
		return "", 0, nil, fmt.Errorf("failed to write elements.json: %w", err)
	}

	// Each object's picture as a file of its own, for its parameter card to
	// point at. elements.json keeps the data: URL too — the extension's sidebar
	// draws it inside the page, where a URL to this server may not load.
	objectImages := map[string]string{}
	for _, el := range elements {
		if el.FieldKey == "" || !strings.HasPrefix(el.Image, "data:image/") {
			continue
		}
		if u := persistScreenshotFile("web-"+name+"-obj-"+el.FieldKey, el.Image); strings.HasPrefix(u, "/api/") {
			objectImages[el.FieldKey] = u
		}
	}

	yamlContent := buildWebWantYAML(name, title, pageURL, hostname, urlTemplate, screenshotURL, probeFrameBlocked(pageURL), elements, objectImages)
	if err = os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(yamlContent), 0o644); err != nil {
		return "", 0, nil, fmt.Errorf("failed to write YAML: %w", err)
	}

	if err = os.WriteFile(filepath.Join(dir, "main.py"), []byte(buildWebWantMainPy(pageURL, elements)), 0o755); err != nil {
		return "", 0, nil, fmt.Errorf("failed to write main.py: %w", err)
	}

	if err = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(buildWebWantSkillMd(name, title, pageURL, elements)), 0o644); err != nil {
		return "", 0, nil, fmt.Errorf("failed to write SKILL.md: %w", err)
	}

	loaded, warnings = s.reloadUserCustomTypesAndSync()
	return dir, loaded, warnings, nil
}

// deriveWebWantName derives a want-type name from a page hostname, mirroring
// the GUI's suggestName (hostname dots→underscores + "_web"). Browsers
// serialize IDN hosts as punycode in location.href, so input is effectively
// ASCII already; the sanitizer squashes anything else to '_' regardless.
func deriveWebWantName(hostname string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(hostname) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	host := b.String()
	if host == "" {
		return "my_web_want"
	}
	if host[0] < 'a' || host[0] > 'z' {
		// validTypeName requires a leading letter (IP hosts start with a digit).
		host = "w" + host
	}
	// Leave room for the "-N" uniquifying suffix under validTypeName's 64-char cap.
	if len(host) > 56 {
		host = host[:56]
	}
	name := host + "_web"
	if !validTypeName.MatchString(name) {
		return "my_web_want"
	}
	return name
}

// reserveWebWantTypeDir atomically claims a unique type directory under
// UserCustomTypesDir() for base, trying base, base-2, base-3, …. os.Mkdir
// fails with EEXIST on collision, so concurrent captures can never claim the
// same name (no mutex needed). Repeat captures of the same site therefore
// always create a NEW numbered type, never merging into an existing one.
func reserveWebWantTypeDir(base string) (name, dir string, err error) {
	root := mywant.UserCustomTypesDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", "", err
	}
	for i := 1; i <= 100; i++ {
		name = base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		dir = filepath.Join(root, name)
		err = os.Mkdir(dir, 0o755)
		if err == nil {
			return name, dir, nil
		}
		if !os.IsExist(err) {
			return "", "", err
		}
	}
	return "", "", fmt.Errorf("no free type name for %q after 100 attempts", base)
}

// captureWebWant handles POST /api/v1/web-wants/capture — the always-on ingest
// for bookmarklet-driven captures. Unlike createWebWant it needs no
// user-entered metadata: the overlay
// sends its selected elements keyed by hostname plus __page_url / __page_title
// taken from the page itself, and the type name is derived from the hostname
// (uniquified via reserveWebWantTypeDir). Deliberately unauthenticated and
// CORS-open like the rest of /web-wants (local tool); mitigations are the
// 2 MB body cap, the element-count cap, the http(s) scheme whitelist and
// validTypeName on the derived name (which also blocks path traversal).
// parseWebWantElementsBody reads the shared capture/overwrite request body
// ({__page_url, __page_title, __url_template, [hostname]: []WebWantElement})
// into its parts. On any validation failure it writes the HTTP error itself and
// returns ok=false, so callers just `if !ok { return }`.
func (s *Server) parseWebWantElementsBody(w http.ResponseWriter, r *http.Request) (pageURL, pageTitle, urlTemplate, screenshotURL string, u *url.URL, elements []WebWantElement, constellations []WebWantConstellation, ok bool) {
	// 8MB: the page screenshot plus one small cut-out per element (Image).
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	str := func(key string) string {
		var v string
		if rawV, okk := raw[key]; okk {
			_ = json.Unmarshal(rawV, &v)
		}
		return v
	}
	pageURL = str("__page_url")
	pageTitle = str("__page_title")
	urlTemplate = str("__url_template")
	screenshotURL = str("__screenshot_url")
	if rawC, okk := raw["__constellations"]; okk {
		_ = json.Unmarshal(rawC, &constellations)
	}
	// Strip reserved/metadata keys — the same set the GUI's handleSave strips,
	// plus the page-context keys — so only hostname→elements entries remain.
	for _, k := range []string{"__page_url", "__page_title", "__url_template", "characterId", "color", "__screenshot_url", "__constellations"} {
		delete(raw, k)
	}

	var perr error
	u, perr = url.Parse(pageURL)
	if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		http.Error(w, "__page_url is required and must be an http(s) URL", http.StatusBadRequest)
		return
	}

	for _, v := range raw {
		var els []WebWantElement
		if err := json.Unmarshal(v, &els); err != nil {
			continue // tolerate non-array fields, like the GUI's Array.isArray filter
		}
		elements = append(elements, els...)
	}
	// No elements is a page saved as it is: a Web Want that opens it, with
	// nothing on it to operate yet. Marks can be added later by overwriting.
	if elements == nil {
		elements = []WebWantElement{}
	}
	if len(elements) > 500 {
		http.Error(w, "too many elements", http.StatusBadRequest)
		return
	}
	constellations = cleanWebWantConstellations(constellations, elements)
	ok = true
	return
}

func (s *Server) captureWebWant(w http.ResponseWriter, r *http.Request) {
	pageURL, pageTitle, urlTemplate, screenshotURL, u, elements, constellations, ok := s.parseWebWantElementsBody(w, r)
	if !ok {
		return
	}

	hostname := u.Hostname()
	name, dir, err := reserveWebWantTypeDir(deriveWebWantName(hostname))
	if err != nil {
		http.Error(w, "failed to reserve type name: "+err.Error(), http.StatusInternalServerError)
		return
	}

	title := strings.TrimSpace(pageTitle)
	if title == "" {
		title = name
	}

	_, loaded, warnings, werr := s.writeWebWantType(name, title, pageURL, hostname, urlTemplate, screenshotURL, elements)
	if werr != nil {
		// Remove the reserved dir so the failed attempt doesn't poison future
		// uniquification with an empty husk.
		os.RemoveAll(dir)
		http.Error(w, werr.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeWebWantConstellations(name, constellations); err != nil {
		http.Error(w, "failed to save constellations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/web-wants/capture", name, "created", loaded, "", "")
	go broadcastSSE("want_type_changed", name)

	s.JSONResponse(w, http.StatusCreated, map[string]any{
		"name":     name,
		"dir":      dir,
		"loaded":   loaded,
		"warnings": warnings,
		"message":  fmt.Sprintf("web want type %q created from capture", name),
	})
}

// getWebWantElements handles GET /api/v1/web-wants/{name} — returns the stored
// elements.json ({hostname: []WebWantElement}) for an existing web want type so
// the Inspect overlay can preload the current marks before an overwrite (PUT).
func (s *Server) getWebWantElements(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if !validTypeName.MatchString(name) {
		s.JSONError(w, r, http.StatusBadRequest, "invalid type name", "")
		return
	}
	data, err := os.ReadFile(filepath.Join(mywant.UserCustomTypesDir(), name, "elements.json"))
	if err != nil {
		s.JSONError(w, r, http.StatusNotFound, "web want type not found", "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data) // already {hostname: [elements]}
}

// updateWebWant handles PUT /api/v1/web-wants/{name} — the "上書き保存" (overwrite)
// path used by the Inspect flow. Unlike captureWebWant it never reserves a new
// name: it replaces the named (existing) type's files in place with the full
// element set in the body (the overlay preloads via GET, edits, then PUTs the
// whole set — PUT semantics = replace).
func (s *Server) updateWebWant(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if !validTypeName.MatchString(name) {
		http.Error(w, "invalid type name", http.StatusBadRequest)
		return
	}
	dir := filepath.Join(mywant.UserCustomTypesDir(), name)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		http.Error(w, "web want type not found", http.StatusNotFound)
		return
	}

	pageURL, pageTitle, urlTemplate, screenshotURL, u, elements, constellations, ok := s.parseWebWantElementsBody(w, r)
	if !ok {
		return
	}

	title := strings.TrimSpace(pageTitle)
	if title == "" {
		title = name
	}

	_, loaded, warnings, werr := s.writeWebWantType(name, title, pageURL, u.Hostname(), urlTemplate, screenshotURL, elements)
	if werr != nil {
		http.Error(w, werr.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeWebWantConstellations(name, constellations); err != nil {
		http.Error(w, "failed to save constellations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.globalBuilder.LogAPIOperation("PUT", "/web-wants/"+name, name, "updated", loaded, "", "")
	go broadcastSSE("want_type_changed", name)

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"name":     name,
		"loaded":   loaded,
		"warnings": warnings,
		"message":  fmt.Sprintf("web want type %q overwritten", name),
	})
}

func buildWebWantYAML(name, title, url, hostname, urlTemplate, screenshotURL string, frameBlocked bool, elements []WebWantElement, objectImages map[string]string) string {
	var elemComments strings.Builder
	var inputStateFields strings.Builder
	var buttonStateFields strings.Builder
	var objectParams strings.Builder

	// Every saved object is also a parameter, so deploying the want shows it as
	// a card — titled with the object's own name, drawn over its picture from
	// the page. A field's parameter is the value to type into it (the web form
	// monitor copies it into the plan field of the same name when the want
	// starts); a button's is whether to press it after filling (default yes).
	for _, el := range elements {
		if el.FieldKey == "" {
			continue
		}
		bg := ""
		if u := objectImages[el.FieldKey]; u != "" {
			bg = fmt.Sprintf("\n      backgroundImage: %q", u)
		}
		if isInputRole(el.Role) {
			objectParams.WriteString(fmt.Sprintf(`
    - name: %s
      title: %q
      description: %q
      type: string
      required: false
      default: ""%s
`, el.FieldKey, el.Name, "Value to type into \""+el.Name+"\"", bg))
		} else {
			objectParams.WriteString(fmt.Sprintf(`
    - name: %s
      title: %q
      description: %q
      type: bool
      required: false
      default: true%s
`, el.FieldKey, el.Name, "Press \""+el.Name+"\" after filling in the fields", bg))
		}
	}

	for _, el := range elements {
		if isInputRole(el.Role) {
			elemComments.WriteString(fmt.Sprintf("    # - [input] %s  selector: %s\n", oneLine(el.Name), oneLine(el.Selector)))
			if el.FieldKey != "" {
				subType := "text"
				if el.Role == "combobox" {
					subType = "select"
				}
				inputStateFields.WriteString(fmt.Sprintf(`
    - name: %s
      description: %q
      type: string
      subType: %s
      label: plan
      persistent: true
      initialValue: ""
`, el.FieldKey, fmt.Sprintf("Value for %q (%s)", el.Name, el.Role), subType))
			}
		} else {
			elemComments.WriteString(fmt.Sprintf("    # - [button] %s  selector: %s\n", oneLine(el.Name), oneLine(el.Selector)))
			if el.FieldKey != "" {
				buttonStateFields.WriteString(fmt.Sprintf(`
    - name: %s
      description: %q
      type: bool
      label: current
      persistent: true
      initialValue: false
`, el.FieldKey, fmt.Sprintf("Click %q (%s) — set by the want after form submission", el.Name, el.Role)))
			}
		}
	}

	elementStateBlock := ""
	if inputStateFields.Len() > 0 {
		elementStateBlock += "\n    # — form field values —" + inputStateFields.String()
	}
	if buttonStateFields.Len() > 0 {
		elementStateBlock += "\n    # — buttons —" + buttonStateFields.String()
	}

	// Rewrite url-template placeholders from HTML element names to field_keys.
	// e.g. {{plan.q}} → {{plan.search}} when the user named the element "Search"
	rewrittenTemplate := urlTemplate
	for _, el := range elements {
		if !isInputRole(el.Role) || el.FieldKey == "" {
			continue
		}
		// Prefer the explicit html_name; fall back to extracting from selector.
		htmlName := el.HtmlName
		if htmlName == "" {
			if m := reName.FindStringSubmatch(el.Selector); len(m) > 1 {
				htmlName = m[1]
			}
		}
		if htmlName != "" && htmlName != el.FieldKey {
			rewrittenTemplate = strings.ReplaceAll(rewrittenTemplate,
				"{{plan."+htmlName+"}}", "{{plan."+el.FieldKey+"}}")
		}
	}

	urlTemplateLabel := ""
	if rewrittenTemplate != "" {
		urlTemplateLabel = fmt.Sprintf("\n      url-template: %q", rewrittenTemplate)
	}

	screenshotLabel := ""
	if frameBlocked {
		// Read by the card (WebFrameCardPlugin): show the shot, not a frame
		// the browser is going to refuse.
		screenshotLabel = "\n      frameable: \"false\""
	}
	if screenshotURL != "" {
		screenshotLabel += fmt.Sprintf("\n      screenshot-url: %q", screenshotURL)
	}

	return fmt.Sprintf(`wantType:
  metadata:
    name: %s
    title: %q
    description: |
      Opens %s via the MyWant Web Inspector Chrome extension.  Set plan state
      fields to auto-fill form elements.  Pre-configured elements for %s:
%s    version: '1.0'
    category: web
    pattern: independent
    labels:
      category-icon: "Globe"
      # A doorway you walk into, not a plate you press: a page somewhere else
      # is a way OUT of the board, and the board already has a shape for that.
      # The form opens when a character stands on it — see forms/types/web.tsx.
      form-type: web
      category-bg-light: "linear-gradient(160deg, #bfdbfe 0%%, #ddd6fe 100%%)"
      category-bg-dark:  "linear-gradient(160deg, #1e3a5f 0%%, #2d1b69 100%%)"
      source-url: %q%s%s

  parameters:
    - name: target_url
      description: URL to open (defaults to the captured site)
      type: string
      required: false
      default: %q
%s
  state:
    - name: status
      description: Current status (idle / active / done)
      type: string
      label: current
      persistent: true
      initialValue: "idle"

    - name: active_element
      description: Currently focused element name
      type: string
      label: current
      persistent: true
      initialValue: ""

    - name: phase
      description: "Submission phase: waiting / ready / done"
      type: string
      label: current
      persistent: true
      initialValue: "waiting"

    - name: reaction_queue_id
      description: Reaction queue ID for user approval
      type: string
      label: current
      persistent: true
      initialValue: ""

    - name: user_reaction
      description: User reaction result from approval UI
      type: object
      label: current
      persistent: true
      initialValue: {}

    - name: plan_snapshot
      description: Snapshot of plan field values at last submission
      type: string
      label: current
      persistent: true
      initialValue: ""

    - name: pending_device_action
      description: Open-URL action pushed to connected devices (cleared after use)
      type: object
      label: current
      persistent: false

    - name: embed_url
      description: Page the want card shows (set when an approved form resolves to a URL)
      type: string
      label: current
      persistent: true
      initialValue: ""
%s
  requires:
    - reminder_monitoring
    - web_form_monitoring

  finalResultField: status
`, name, title, url, hostname, elemComments.String(), url, urlTemplateLabel, screenshotLabel, url, objectParams.String(), elementStateBlock)
}

// launchWebWant handles POST /api/v1/web-wants/{name}/launch
func (s *Server) launchWebWant(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		if vars := mux.Vars(r); vars != nil {
			name = vars["name"]
		}
	}
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	dir := filepath.Join(mywant.UserCustomTypesDir(), name)
	elemFile := filepath.Join(dir, "elements.json")
	data, err := os.ReadFile(elemFile)
	if err != nil {
		http.Error(w, "elements.json not found for: "+name, http.StatusNotFound)
		return
	}

	var allElems map[string][]WebWantElement
	if err := json.Unmarshal(data, &allElems); err != nil {
		http.Error(w, "failed to parse elements.json: "+err.Error(), http.StatusBadRequest)
		return
	}

	var body struct {
		TargetURL   string            `json:"target_url"`
		FieldValues map[string]string `json:"field_values,omitempty"`
		// FillOnly types the values in and stops there: no button is pressed
		// and nothing is submitted. The want card's "open in a new tab" asks
		// for this, so the page comes up with the want's parameters already
		// entered and the person decides what to do with them.
		FillOnly bool `json:"fill_only,omitempty"`
		// Device is the browser that asked (see navLaunchClaim.Device).
		Device string `json:"device,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	targetURL := body.TargetURL
	if targetURL == "" {
		for hostname := range allElems {
			targetURL = "https://" + hostname
			break
		}
	}

	var elements []WebWantElement
	for _, elems := range allElems {
		elements = append(elements, elems...)
	}
	// No elements is a page saved as it is (see parseWebWantElementsBody):
	// launching it just opens the page.

	// Auto-fill mode when field_values are provided — this is what
	// agent_web_form_monitor.go's webFormMonitorSubmit calls when a web want
	// deployed as a want card has its plan fields filled in and approved.
	// Queued for the extension the same way as the default branch below
	// (see navLaunchClaim) rather than driving CDP directly — the extension
	// side (background.js's handleNavLaunch) branches on FieldValues being
	// present to run mywantFillAndSubmit instead of the read-only
	// mywantNavOverlay.
	if len(body.FieldValues) > 0 {
		enqueueNavLaunch(navLaunchClaim{TargetURL: targetURL, Elements: elements, FieldValues: body.FieldValues, FillOnly: body.FillOnly, Device: body.Device})
		mode := "fill"
		if body.FillOnly {
			mode = "prefill"
		}
		s.JSONResponse(w, http.StatusOK, map[string]any{
			"ok":      true,
			"url":     targetURL,
			"mode":    mode,
			"fields":  len(body.FieldValues),
			"message": fmt.Sprintf("queued %s (%d field(s)) for the Chrome extension", name, len(body.FieldValues)),
		})
		return
	}

	// Default: queue for the Chrome extension's own poll instead of driving
	// CDP directly (see handleNavLaunch in background.js). No CDP fallback —
	// if nothing is polling pending-action, this silently does nothing until
	// something does.
	enqueueNavLaunch(navLaunchClaim{TargetURL: targetURL, Elements: elements, Device: body.Device})

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"ok":       true,
		"url":      targetURL,
		"mode":     "nav",
		"elements": len(elements),
		"message":  fmt.Sprintf("queued %s (%d elements) for the Chrome extension", name, len(elements)),
	})
}

// openInBrowser handles POST /api/v1/web-wants/open with body {"url": "..."}.
//
// The bare form of launchWebWant above: it queues the same nav-launch claim, but
// with no elements and no field values, so the extension opens the tab and
// injects nothing. It exists so that a want which only needs a URL opened —
// gmail_memo's "show mail" — does not have to be backed by a web want type with
// an elements.json, and does not deploy a second want just to open a link.
func (s *Server) openInBrowser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
		// Device is the browser that asked (see navLaunchClaim.Device).
		Device string `json:"device,omitempty"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	// http(s) only: this hands a URL straight to chrome.tabs.create, and the
	// javascript:/data: schemes are code execution rather than navigation.
	if !strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://") {
		s.JSONError(w, r, http.StatusBadRequest, "url must be http(s)", body.URL)
		return
	}
	enqueueNavLaunch(navLaunchClaim{TargetURL: body.URL, Device: body.Device})
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"ok":      true,
		"url":     body.URL,
		"mode":    "open",
		"message": "queued for the Chrome extension",
	})
}

// navLaunchClaim is a pending "Inspect" request (POST /web-wants/{name}/launch
// with no navigate_only/field_values) waiting for the Chrome extension to
// open a tab for and inject the read-only nav-highlight overlay into —
// background.js's JS equivalent of BuildNavJS below (in mywant-gui's
// webext/), run via chrome.scripting.executeScript's func+args instead of a
// CDP-injected string, so it isn't subject to page CSP the way
// build-standalone-overlay.js injecting a <script src> would be.
type navLaunchClaim struct {
	TargetURL string           `json:"target_url"`
	Elements  []WebWantElement `json:"nav_elements"`
	// Present only for the auto-fill mode (webFormMonitorSubmit) — tells the
	// extension to run mywantFillAndSubmit instead of the read-only
	// mywantNavOverlay. Keyed by WebWantElement.FieldKey.
	FieldValues map[string]string `json:"field_values,omitempty"`
	// With FieldValues: fill them in but press nothing (see launchWebWant).
	FillOnly bool `json:"fill_only,omitempty"`
	// Device, when set, is the browser a person clicked in to ask for this
	// tab, and only that browser's extension is handed it — the home browser
	// (pollerIsHomeBrowser) is for work nobody is watching, and a tab asked
	// for from one Chrome opening in another is a click that did nothing.
	Device string `json:"-"`
}

// claimQueue is a generic mutex-guarded FIFO — the shared shape behind
// nav-launch and browser-run's "enqueue now, extension dequeues later via
// polling" queues (see pendingBrowserAction).
type claimQueue[T any] struct {
	mu    sync.Mutex
	items []T
}

func (q *claimQueue[T]) enqueue(item T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

func (q *claimQueue[T]) dequeue() (T, bool) {
	return q.dequeueWhere(func(T) bool { return true })
}

// dequeueWhere takes the oldest item that match accepts, leaving the rest in
// order.
func (q *claimQueue[T]) dequeueWhere(match func(T) bool) (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var zero T
	for i, item := range q.items {
		if match(item) {
			q.items = append(q.items[:i:i], q.items[i+1:]...)
			return item, true
		}
	}
	return zero, false
}

var navLaunchQueue = &claimQueue[navLaunchClaim]{}

func enqueueNavLaunch(claim navLaunchClaim) {
	navLaunchQueue.enqueue(claim)
	go broadcastSSE("pending_action", nil)
}

// navCallback is a no-op endpoint consumed by the navigation overlay's "Done" post.
func (s *Server) navCallback(w http.ResponseWriter, _ *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// suggestNameRequest is the body for POST /api/v1/web-wants/suggest-name.
type suggestNameRequest struct {
	HTML string `json:"html"`
}

// suggestNameResponse always returns HTTP 200 — this is a fail-open, best-effort
// suggestion; callers should just check Name for truthiness and never branch on
// HTTP status.
type suggestNameResponse struct {
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

// suggestElementName asks the lightweight one-shot `claude --print` helper for
// a short name describing an inspector element's surrounding HTML. Invoked by
// the Web Inspector overlay (injected into the page being inspected, hence a
// cross-origin fetch — see corsMiddleware) speculatively while its naming
// dialog is open and not yet focused by the user.
func (s *Server) suggestElementName(w http.ResponseWriter, r *http.Request) {
	var req suggestNameRequest
	if err := DecodeRequest(r, &req); err != nil || req.HTML == "" {
		s.JSONResponse(w, http.StatusOK, suggestNameResponse{Error: "html is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	name, err := types.SuggestElementName(ctx, req.HTML)
	if err != nil {
		s.JSONResponse(w, http.StatusOK, suggestNameResponse{Error: err.Error()})
		return
	}
	s.JSONResponse(w, http.StatusOK, suggestNameResponse{Name: name})
}

// activeInspectionResponse is the payload for GET /api/v1/web-wants/active-inspection —
// everything the standalone overlay loader (Chrome extension or bookmarklet)
// needs to capture the page it runs on.
type activeInspectionResponse struct {
	DoneWebhookURL string                  `json:"done_webhook_url"`
	SuggestNameURL string                  `json:"suggest_name_url"`
	CharacterID    string                  `json:"character_id"`
	Color          string                  `json:"color"`
	Avatar         string                  `json:"avatar"`
	ExistingMarks  []mywant.WebElementMark `json:"existing_marks"`
}

// hostnameOfURL returns targetURL's hostname, or "" if it doesn't parse.
func hostnameOfURL(targetURL string) string {
	u, err := url.Parse(targetURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// activeInspection tells a static, session-agnostic overlay loader — a
// bookmarklet or the extension, neither of which has anything baked in but
// this server's address — where to send what it captures on the page it runs
// on (passed as ?url=), and which marks are already there.
func (s *Server) activeInspection(w http.ResponseWriter, r *http.Request) {
	// The bookmarklet says so (?via=bookmarklet): this device can use it. See
	// recordWebInspectorUse.
	if r.URL.Query().Get("via") == "bookmarklet" {
		s.recordWebInspectorUse(r)
	}
	pageHost := hostnameOfURL(r.URL.Query().Get("url"))

	// Built from the request's own Host header, not a hardcoded "localhost":
	// the caller may be a phone on the LAN via mywant-gui's reverse proxy, and
	// r.Host is exactly the origin it used to reach us. Scheme comes from
	// X-Forwarded-Proto (set by Caddy, which terminates TLS), not r.TLS —
	// getting it wrong sends the overlay a plain-http URL that fails the
	// mixed-content block Caddy was added to fix.
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	origin := scheme + "://" + r.Host
	if r.Host == "" {
		origin = "http://localhost:8080"
	}
	resp := activeInspectionResponse{
		DoneWebhookURL: origin + "/api/v1/web-wants/capture",
		SuggestNameURL: origin + "/api/v1/web-wants/suggest-name",
		ExistingMarks:  mywant.GetWebMarks(pageHost),
	}
	// Who is capturing is otherwise unknown here: "my character" lives in the
	// GUI's own browser storage, which a bookmarklet on another site cannot
	// read. The bookmarklet carries it instead (?character=, baked in when it
	// was made), so the CursorMan it draws is the one the dashboard draws —
	// resolved on every launch, so a character's later colour or avatar
	// change still shows.
	if id := r.URL.Query().Get("character"); id != "" {
		if character, ok := mywant.GetCharacter(id); ok {
			resp.CharacterID = character.ID
			resp.Color = character.Color
			resp.Avatar = character.Avatar
		}
	}
	s.JSONResponse(w, http.StatusOK, resp)
}

// browserRunClaim is a pending request queued for the Chrome extension to
// open a tab for and run via @puppeteer/replay's Step/UserFlow schema (see
// mywant-gui's webext/webext-src/browser-run-interpreter.ts) — the
// CDP-free replacement for the various ~/.mywant/custom-types plugins
// (gmail, smartgolf, ...) that used to Playwright-connect_over_cdp to an
// existing --remote-debugging-port Chrome. Steps is left as raw JSON on
// purpose: Go never interprets it, only the extension's TypeScript does, so
// there's no schema to keep in sync on this side.
type browserRunClaim struct {
	RequestID  string          `json:"request_id"`
	URL        string          `json:"url"`
	Steps      json.RawMessage `json:"steps"`
	KeepOpen   bool            `json:"keep_open,omitempty"`
	Background bool            `json:"background,omitempty"` // open the tab non-active (see handleBrowserRun) — for callers polled often enough that stealing focus every run would be disruptive (e.g. claude_info's 60s gauge poll)
	// Quiet: this run must never get in its person's way. A login or a
	// CAPTCHA it runs into is not filed as needing them, a failure is not
	// filed either, and its tab closes as asked instead of being kept open
	// for them. The caller hears the outcome; the person is not called.
	Quiet bool `json:"quiet,omitempty"`
}

// browserRunResult is both what the extension POSTs back (browserRunResult
// handler) and what browserRun ultimately responds to the Python caller
// with — same shape on both ends of the relay.
type browserRunResult struct {
	RequestID string         `json:"request_id"`
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
}

var browserRunQueue = &claimQueue[browserRunClaim]{}

var (
	browserRunPendingMu sync.Mutex
	browserRunPending   = map[string]chan browserRunResult{}
	// The URL each pending run is for, so its outcome can be filed against
	// the tab it happened in (see recordWebRunOutcome).
	browserRunPendingURL = map[string]string{}
	// Runs asked to be quiet (browserRunClaim.Quiet), by request ID.
	browserRunPendingQuiet = map[string]bool{}
)

const defaultBrowserRunTimeoutMs = 90000

// browserRun is POST /api/v1/web-wants/browser-run — the synchronous,
// Python-facing entry point (see the generated browser_run() helper in
// buildWebWantMainPy-style skill scripts, and the hand-written equivalent in
// ~/.mywant/custom-types/*/main.py for gmail/smartgolf). Queues the request
// for the extension's pollForPendingAction (same 1-minute alarm tick as
// nav-launch) and blocks until browserRunResult delivers a
// result or the timeout elapses — the caller gets a single ordinary HTTP
// response either way, no separate polling needed on the Python side.
func (s *Server) browserRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL        string          `json:"url"`
		Steps      json.RawMessage `json:"steps"`
		KeepOpen   bool            `json:"keep_open,omitempty"`
		Background bool            `json:"background,omitempty"`
		Quiet      bool            `json:"quiet,omitempty"`
		TimeoutMs  int             `json:"timeout_ms,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultBrowserRunTimeoutMs
	}

	requestID := generateWantID()
	resultCh := make(chan browserRunResult, 1)

	browserRunPendingMu.Lock()
	browserRunPending[requestID] = resultCh
	browserRunPendingURL[requestID] = req.URL
	if req.Quiet {
		browserRunPendingQuiet[requestID] = true
	}
	browserRunPendingMu.Unlock()
	browserRunQueue.enqueue(browserRunClaim{
		RequestID:  requestID,
		URL:        req.URL,
		Steps:      req.Steps,
		KeepOpen:   req.KeepOpen,
		Background: req.Background,
		Quiet:      req.Quiet,
	})
	go broadcastSSE("pending_action", nil)

	select {
	case res := <-resultCh:
		s.JSONResponse(w, http.StatusOK, res)
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		browserRunPendingMu.Lock()
		delete(browserRunPending, requestID)
		// No browser took it, so there is no tab to send anyone to.
		delete(browserRunPendingURL, requestID)
		delete(browserRunPendingQuiet, requestID)
		browserRunPendingMu.Unlock()
		s.JSONResponse(w, http.StatusGatewayTimeout, browserRunResult{
			RequestID: requestID,
			Error:     "timed out waiting for the browser extension to run this request",
		})
	}
}

// pendingActionResponse is the single envelope GET
// /api/v1/web-wants/pending-action returns — replaces the formerly separate
// pending-nav-launch/pending-browser-run endpoints (and background.js's
// separate poll functions) with one polled endpoint,
// dispatched by Kind.
type pendingActionResponse struct {
	Kind       string           `json:"kind"` // "nav_launch" | "browser_run" | "" when nothing is pending
	NavLaunch  *navLaunchClaim  `json:"nav_launch,omitempty"`
	BrowserRun *browserRunClaim `json:"browser_run,omitempty"`
}

// pendingBrowserAction is GET /api/v1/web-wants/pending-action — polled by
// the extension's background service worker on a single alarm tick (see
// pollForPendingAction in background.js) in place of the three separate
// polls this used to require. Checked in priority order — nav-launch, then
// browser-run — and returns the first one found; an idle
// poll (nothing pending anywhere) gets back {kind: ""}.
// pollerIsHomeBrowser reports whether this poller may claim work.
//
// Every extension polling this server used to be interchangeable, so whichever
// asked first got the job — with two browsers signed in, a tab could open on
// the wrong machine entirely. A device can now be named home, after which only
// it is served; work then waits for that browser rather than going somewhere
// the user cannot see. With no home named, any poller is served, which is the
// behaviour every existing install has.
// Read from devices.yaml rather than from the gui_state want: the want is only
// a mirror, and the mirror is what a world switch overwrites (device_store.go).
// A poller asking for work while the board was being changed under it used to
// be able to get an answer meant for a different browser.
func (s *Server) pollerIsHomeBrowser(r *http.Request) bool {
	home := mywant.GetDeviceStore().Home()
	if home == "" {
		return true
	}
	return r.URL.Query().Get("device") == home
}

func (s *Server) pendingBrowserAction(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, nextPendingAction(r.URL.Query().Get("device"), s.pollerIsHomeBrowser(r)))
}

// nextPendingAction picks what a poller gets: first a tab its own browser
// asked for (navLaunchClaim.Device), home or not; then, only for the home
// browser, the unaddressed work — nav-launch before browser-run.
func nextPendingAction(device string, isHome bool) pendingActionResponse {
	if device != "" {
		if claim, ok := navLaunchQueue.dequeueWhere(func(c navLaunchClaim) bool { return c.Device == device }); ok {
			return pendingActionResponse{Kind: "nav_launch", NavLaunch: &claim}
		}
	}
	if !isHome {
		return pendingActionResponse{}
	}
	if claim, ok := navLaunchQueue.dequeueWhere(func(c navLaunchClaim) bool { return c.Device == "" }); ok {
		return pendingActionResponse{Kind: "nav_launch", NavLaunch: &claim}
	}
	if claim, ok := browserRunQueue.dequeue(); ok {
		return pendingActionResponse{Kind: "browser_run", BrowserRun: &claim}
	}
	return pendingActionResponse{}
}

// browserRunResultHandler is POST /api/v1/web-wants/browser-run-result — the
// extension posts here once it's run the steps (or failed to). Wakes up the
// matching browserRun call, if it's still waiting; a late/duplicate post
// (e.g. after the requester already timed out) is just dropped.
func (s *Server) browserRunResultHandler(w http.ResponseWriter, r *http.Request) {
	var res browserRunResult
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	browserRunPendingMu.Lock()
	ch, ok := browserRunPending[res.RequestID]
	if ok {
		delete(browserRunPending, res.RequestID)
	}
	runURL := browserRunPendingURL[res.RequestID]
	delete(browserRunPendingURL, res.RequestID)
	quiet := browserRunPendingQuiet[res.RequestID]
	delete(browserRunPendingQuiet, res.RequestID)
	browserRunPendingMu.Unlock()
	if quiet {
		clearWebAttention(runURL)
	} else {
		recordWebRunOutcome(runURL, res)
	}

	if ok {
		ch <- res
	}
	s.JSONResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// serveCACert serves the CA root certificate configured via
// web_inspector_ca_cert_path (see mywant-gui/docs/WebInspectorIPhone.md) —
// lets a phone download and trust the Caddy internal CA straight from its
// own Safari instead of needing the cert AirDropped from the Mac.
// application/x-x509-ca-cert is the MIME type iOS recognizes to offer
// "Install Profile" on download, rather than treating it as an opaque file.
func (s *Server) serveCACert(w http.ResponseWriter, r *http.Request) {
	p := s.config.WebInspectorCACertPath
	if p == "" {
		http.Error(w, "web_inspector_ca_cert_path not configured — set it from the Web Inspector modal's iPhone tab", http.StatusNotFound)
		return
	}
	if _, err := os.Stat(p); err != nil {
		http.Error(w, "configured CA cert not found: "+err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="mywant-ca.crt"`)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, p)
}

func buildWebWantMainPy(url string, elements []WebWantElement) string {
	elemJSON, _ := json.Marshal(elements)

	return fmt.Sprintf(`#!/usr/bin/env python3
"""Web want: auto-fills form fields from parameters, or opens a navigation overlay."""
import json, os, sys, urllib.request, urllib.error

MYWANT_API = os.environ.get("MYWANT_URL", "http://localhost:8080")
TARGET_URL = %q
ELEMENTS   = %s
WANT_NAME  = os.path.basename(os.path.dirname(os.path.abspath(__file__)))

_SYSTEM_PARAMS = {"target_url"}


def report(p, m=""):
    print(json.dumps({"_progress": p, "_message": m}), flush=True)


def call_launch(payload_dict):
    payload = json.dumps(payload_dict).encode()
    req = urllib.request.Request(
        f"{MYWANT_API}/api/v1/web-wants/{WANT_NAME}/launch",
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read())


def main():
    raw = sys.argv[1] if len(sys.argv) > 1 else "{}"
    arg = json.loads(raw or "{}")
    target_url = arg.get("target_url", TARGET_URL)

    # Collect field values for input elements that have a field_key in params
    field_values = {}
    for el in ELEMENTS:
        fk = el.get("field_key") or ""
        if fk and fk in arg and str(arg[fk]).strip():
            field_values[fk] = str(arg[fk])

    launch_payload = {
        "target_url": target_url,
    }
    if field_values:
        launch_payload["field_values"] = field_values
        report(20, f"auto-filling {len(field_values)} field(s) on {target_url}")
    else:
        report(20, "launching browser navigation overlay")

    try:
        result = call_launch(launch_payload)
        report(100, result.get("message", "done"))
        print(json.dumps({"status": "active", "url": target_url,
                          "mode": result.get("mode", "nav")}), flush=True)
    except Exception as e:
        print(json.dumps({"status": "error", "error": str(e)}), flush=True)
        sys.exit(1)


if __name__ == "__main__":
    main()
`, url, string(elemJSON))
}

func buildWebWantSkillMd(name, title, url string, elements []WebWantElement) string {
	var elemList strings.Builder
	for _, el := range elements {
		fkNote := ""
		if el.FieldKey != "" {
			fkNote = fmt.Sprintf(" → state field `%s` (plan)", el.FieldKey)
		}
		elemList.WriteString(fmt.Sprintf("- **%s** (%s)%s\n", el.Name, el.Role, fkNote))
	}

	return fmt.Sprintf(`# %s

Web want type for **%s**.

Set plan state fields to auto-fill form elements on launch.
Without plan state values, opens an interactive navigation overlay.

## Elements

%s
## State Fields (plan)

Set these plan state fields to auto-fill the form:

| Field | Element | Role |
|-------|---------|------|
`, title, url, elemList.String()) + buildParamTable(elements) + "\n"
}

func buildParamTable(elements []WebWantElement) string {
	var sb strings.Builder
	for _, el := range elements {
		if el.FieldKey != "" {
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", el.FieldKey, el.Name, el.Role))
		}
	}
	return sb.String()
}
