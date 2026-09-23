package types

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[PictureWant, PictureLocals]("picture")
	})
}

type PictureLocals struct{}

// maxPictureReach caps how far from its tile a picture can be pinned, in grid
// cells — the radius of the pin picker on the canvas.
const maxPictureReach = 5

// PictureWant keeps one photograph and where on the board it is pinned.
//
// The photo is drawn by the canvas on the ground beside the want's tile; this
// want only owns the facts: the link it was given (url), the image that link
// resolves to (image_url), and the pin — a grid offset from the tile
// (pin_dx, pin_dy). The pin is moved via POST /api/v1/webhooks/{id} with
// {"action":"pin","dx":..,"dy":..}, and the link can be replaced with
// {"action":"set_url","url":".."}.
//
// It also reads the words in the photo once (text), and remembers the
// questions it has been asked and whether each answer was right (answers) —
// see readText and recordAnswer.
type PictureWant struct{ Want }

func (p *PictureWant) GetLocals() *PictureLocals {
	return CheckLocalsInitialized[PictureLocals](&p.Want)
}

func (p *PictureWant) Initialize() {
	// First init only: the pin and the link are live-controlled by the webhook,
	// and re-applying the deploy-time params on every restart would throw away
	// where the user last put the photo. Same guard as direction.
	if _, ok := p.GetCurrent("pin_dx"); !ok {
		dx, dy := clampPicturePin(p.GetFloatParam("pin_dx", 2), p.GetFloatParam("pin_dy", 0))
		p.SetCurrent("pin_dx", dx)
		p.SetCurrent("pin_dy", dy)
	}
	// The link has two authors — the url param (the settings form, the YAML)
	// and the set_url webhook (the card) — and the latest one wins. A param
	// only speaks when it has changed since it last spoke, so a restart does
	// not undo a link given through the card, and editing the param still
	// replaces it.
	param := strings.TrimSpace(p.GetStringParam("url", ""))
	if seen, _ := p.GetInternal("param_url"); seen != param {
		p.SetInternal("param_url", param)
		p.SetCurrent("url", param)
	}
	p.SetCurrent("size", clampPictureSize(p.GetFloatParam("size", 2)))
	p.StoreState("last_action_at", "")
}

func (p *PictureWant) IsAchieved() bool { return false }

func (p *PictureWant) Progress() {
	ConsumeWebhookAction(&p.Want, "last_action_at", func(action string, pm map[string]any) bool {
		switch action {
		case "pin":
			dx, ok1 := pm["dx"].(float64)
			dy, ok2 := pm["dy"].(float64)
			if !ok1 || !ok2 {
				return false
			}
			dx, dy = clampPicturePin(dx, dy)
			p.SetCurrent("pin_dx", dx)
			p.SetCurrent("pin_dy", dy)
			return true
		case "set_url":
			u, ok := pm["url"].(string)
			if !ok {
				return false
			}
			p.SetCurrent("url", strings.TrimSpace(u))
			return true
		case "record_answer":
			return p.recordAnswer(pm)
		case "feedback":
			return p.recordFeedback(pm)
		}
		return false
	})

	p.resolve()
	p.readText()
}

// maxPictureAnswers is how many questions and answers a picture remembers.
const maxPictureAnswers = 50

// recordAnswer keeps one question somebody asked about this picture and the
// answer they were given, so what was asked of it — and, once somebody says,
// whether the answer was right — stays with the picture.
//
// {"action":"record_answer","id":"..","question":"..","answer":"..","by":".."}
func (p *PictureWant) recordAnswer(pm map[string]any) bool {
	id, _ := pm["id"].(string)
	question, _ := pm["question"].(string)
	answer, _ := pm["answer"].(string)
	if id == "" || answer == "" {
		return false
	}
	entry := map[string]any{
		"id":       id,
		"question": question,
		"answer":   answer,
		"at":       time.Now().Format(time.RFC3339),
		"feedback": "",
	}
	if by, _ := pm["by"].(string); by != "" {
		entry["by"] = by
	}
	answers := append(pictureAnswers(&p.Want), entry)
	if len(answers) > maxPictureAnswers {
		answers = answers[len(answers)-maxPictureAnswers:]
	}
	p.SetCurrent("answers", answers)
	return true
}

// recordFeedback marks one remembered answer as right ("good") or wrong
// ("wrong"), or clears the mark (""). Pressing the same button twice is the
// person taking it back, which the caller says by sending "".
//
// {"action":"feedback","id":"..","value":"good"|"wrong"|""}
func (p *PictureWant) recordFeedback(pm map[string]any) bool {
	id, _ := pm["id"].(string)
	value, _ := pm["value"].(string)
	if id == "" || (value != "" && value != "good" && value != "wrong") {
		return false
	}
	answers := pictureAnswers(&p.Want)
	for _, a := range answers {
		if a["id"] == id {
			a["feedback"] = value
			a["feedback_at"] = time.Now().Format(time.RFC3339)
			p.SetCurrent("answers", answers)
			return true
		}
	}
	return false
}

// pictureAnswers is a copy of the answers list, whatever shape it was stored
// in. A copy, map by map, because the caller edits it: the maps in state are
// the ones a reader may be serialising at the same moment.
func pictureAnswers(w *Want) []map[string]any {
	raw, _ := w.GetCurrent("answers")
	var items []map[string]any
	switch list := raw.(type) {
	case []any:
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				items = append(items, m)
			}
		}
	case []map[string]any:
		items = list
	}
	out := make([]map[string]any, 0, len(items)+1)
	for _, m := range items {
		c := make(map[string]any, len(m)+1)
		for k, v := range m {
			c[k] = v
		}
		out = append(out, c)
	}
	return out
}

// readText writes down the words in the picture (`text`), once per image.
//
// Read here, when the picture arrives, and never by a language model: the
// on-device text recogniser copies what is written and makes up nothing, which
// is the part a model gets wrong. Whoever is asked about the picture later —
// the robot, reading this want — gets the words already read, and does the
// understanding.
//
// The recogniser is Apple's, so this happens only on a Mac with fmtool beside
// the server; anywhere else the picture simply has no text, and says why.
func (p *PictureWant) readText() {
	img := GetCurrent(&p.Want, "image_url", "")
	from, _ := p.GetInternal("text_from")
	if img == "" || from == img {
		return
	}
	p.SetInternal("text_from", img)

	binary, ok := fmToolPath()
	if !ok {
		p.SetCurrent("text", "")
		p.SetCurrent("text_error", "reading text needs fmtool on macOS")
		return
	}
	// The first run loads the recogniser, which takes several seconds; after
	// that a picture reads in well under one.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	lines, err := ocrPicture(ctx, binary, img)
	if err != nil {
		p.SetCurrent("text", "")
		p.SetCurrent("text_error", err.Error())
		p.StoreLog("[PICTURE] could not read the text in %s: %v", img, err)
		return
	}
	p.SetCurrent("text", strings.Join(lines, "\n"))
	p.SetCurrent("text_error", "")
	p.StoreLog("[PICTURE] read %d lines of text", len(lines))
}

// downloadPicture saves an image to a temporary file and returns its path;
// the caller removes it.
func downloadPicture(ctx context.Context, imageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; mywant-picture/1.0)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("image HTTP %d", resp.StatusCode)
	}
	f, err := os.CreateTemp("", "mywant-picture-*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 30<<20)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// ocrPicture downloads the image and asks fmtool what is written in it.
func ocrPicture(ctx context.Context, fmtool, imageURL string) ([]string, error) {
	file, err := downloadPicture(ctx, imageURL)
	if err != nil {
		return nil, err
	}
	defer os.Remove(file)

	out, err := exec.CommandContext(ctx, fmtool, "--ocr", file).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	var result struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("fmtool --ocr: %v", err)
	}
	return result.Lines, nil
}

// resolve turns url into image_url, once per url.
//
// A share link is a page, not a picture, so it is fetched and the image it
// shares is read out of it. That is a network round trip, so it happens only
// when the link has changed since the last one resolved (resolved_from) — not
// on every tick. A failure is remembered the same way, so a dead link is not
// fetched again and again; giving the want a new link is what retries it.
func (p *PictureWant) resolve() {
	raw, _ := p.GetCurrent("url")
	link, _ := raw.(string)
	from, _ := p.GetInternal("resolved_from")
	if from == link {
		return
	}
	p.SetInternal("resolved_from", link)
	if link == "" {
		p.SetCurrent("image_url", "")
		p.SetCurrent("status", "waiting: give this picture a url")
		p.SetCurrent("last_error", "")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	img, err := resolvePictureURL(ctx, http.DefaultClient, link)
	if err != nil {
		p.SetCurrent("status", "error")
		p.SetCurrent("last_error", err.Error())
		p.StoreLog("[PICTURE] could not resolve %s: %v", link, err)
		return
	}
	p.SetCurrent("image_url", img)
	p.SetCurrent("status", "resolved")
	p.SetCurrent("last_error", "")
	p.StoreLog("[PICTURE] %s → %s", link, img)
}

// resolvePictureURL returns the image a link points at.
//
// A Google Photos share link (photos.app.goo.gl/…, photos.google.com/share/…)
// is an HTML page whose og:image is the shared photo, served from
// lh3.googleusercontent.com with a size suffix sized for a link preview; the
// suffix is replaced with one sized for the board.
//
// A photos.google.com address that is not a share link is the photo's place in
// somebody's own library, which only they can open signed in — it is refused
// with a message that says what to paste instead. Anything else must actually
// be an image: it is fetched and its Content-Type checked, so a page, a login
// redirect or a dead link is reported rather than recorded as resolved and
// left to fail silently in an <img>.
func resolvePictureURL(ctx context.Context, client *http.Client, link string) (string, error) {
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("not a web address: %q", link)
	}
	if isGooglePhotosLibrary(u) {
		return "", fmt.Errorf("this is a photo in a Google Photos library, which only its owner can open — use Share → Create link and paste the photos.app.goo.gl link")
	}
	if !isGooglePhotosShare(u) {
		if err := checkIsImage(ctx, client, link); err != nil {
			return "", err
		}
		return link, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	// Google serves the og: tags to link-preview fetchers; a browser-ish agent
	// gets the same page.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; mywant-picture/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("share page HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	img := ogImage(string(body))
	if img == "" {
		return "", fmt.Errorf("share page has no image — is the link shared publicly?")
	}
	return sizeGooglePhoto(img, 1600), nil
}

// isGooglePhotosLibrary is a photos.google.com address that is not a share
// link — /photo/…, /album/…, the library itself.
func isGooglePhotosLibrary(u *url.URL) bool {
	return strings.ToLower(u.Host) == "photos.google.com" && !strings.HasPrefix(u.Path, "/share/")
}

// checkIsImage fetches link and fails unless what comes back is an image.
// Redirects are followed, so a link that ends at a sign-in page fails here on
// its text/html.
func checkIsImage(ctx context.Context, client *http.Client, link string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; mywant-picture/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("image HTTP %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(ct), "image/") {
		if ct == "" {
			ct = "no content type"
		}
		return fmt.Errorf("not an image (%s) — paste a share link or a direct image URL", ct)
	}
	return nil
}

func isGooglePhotosShare(u *url.URL) bool {
	host := strings.ToLower(u.Host)
	switch {
	case host == "photos.app.goo.gl":
		return true
	case host == "photos.google.com" && strings.HasPrefix(u.Path, "/share/"):
		return true
	case host == "goo.gl" && strings.HasPrefix(u.Path, "/photos/"):
		return true
	}
	return false
}

var ogImageRe = []*regexp.Regexp{
	regexp.MustCompile(`<meta[^>]+property=["']og:image["'][^>]*content=["']([^"']+)["']`),
	regexp.MustCompile(`<meta[^>]+content=["']([^"']+)["'][^>]*property=["']og:image["']`),
}

func ogImage(page string) string {
	for _, re := range ogImageRe {
		if m := re.FindStringSubmatch(page); m != nil {
			return html.UnescapeString(m[1])
		}
	}
	return ""
}

// googleSizeSuffix is the "=w600-h315-p-k" tail lh3 URLs carry: the size and
// crop the image is served at.
var googleSizeSuffix = regexp.MustCompile(`=[a-z0-9-]+$`)

// sizeGooglePhoto asks lh3 for the photo at the given width, uncropped. The
// og:image Google hands out is cropped to a link-preview rectangle, which cuts
// the top and bottom off a portrait photo.
func sizeGooglePhoto(img string, width int) string {
	if !strings.Contains(img, "googleusercontent.com") {
		return img
	}
	return googleSizeSuffix.ReplaceAllString(img, "") + fmt.Sprintf("=w%d", width)
}

// clampPicturePin keeps the photo within reach of its tile. Unlike a
// direction, (0, 0) is refused: a photo pinned under its own tile is hidden by
// it, so it is pushed out one cell east.
func clampPicturePin(dx, dy float64) (float64, float64) {
	dx, dy = math.Round(dx), math.Round(dy)
	if dx == 0 && dy == 0 {
		return 1, 0
	}
	if mag := math.Hypot(dx, dy); mag > maxPictureReach {
		s := maxPictureReach / mag
		return math.Trunc(dx * s), math.Trunc(dy * s)
	}
	return dx, dy
}

func clampPictureSize(s float64) float64 {
	return math.Max(1, math.Min(4, s))
}
