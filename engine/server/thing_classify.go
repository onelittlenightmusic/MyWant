package server

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Reading what was shared as things: which subtype of the data type catalog
// (datatypes.yaml) each link or text is. Clients hand over what another app
// shared — the iPhone's share sheet, anything else — and get the same answer,
// because the rules live here, next to the catalog they answer in.
//
// A link is filed as what it links to: a photo, a song, a film, a book, a
// hotel, a product … First by its site and path, without a request; a link
// that says nothing that way is looked at — the head of its page only: an
// image, a sound or a film served as such, else the page's og:type, else the
// same rules on where its redirects end, which places short links
// (spotify.link, amzn.to, maps.app.goo.gl). Whatever is still unknown is a url.
//
// A text that came with links is their caption (the title Safari sends with
// its URL), not a thing. A text alone is one thing, of the subtype it reads
// as whole — an e-mail address, a phone number, a date, an address — else a
// plain string.

// sharedThing is one thing read from what was shared: its subtype and value.
type sharedThing struct {
	Subtype string `json:"subtype"`
	Value   string `json:"value"`
}

// classifyShared reads links and texts as things, looking at the pages of
// links the rules alone cannot place. The order is the order they came in,
// links first; duplicates are dropped.
func classifyShared(ctx context.Context, client *http.Client, links, texts []string) []sharedThing {
	var urls []*url.URL
	var leftover []string
	for _, l := range links {
		if u := webURL(l); u != nil {
			urls = append(urls, u)
		}
	}
	for _, t := range texts {
		found := linksIn(t)
		urls = append(urls, found...)
		if len(found) == 0 {
			leftover = append(leftover, t)
		}
	}

	var out []sharedThing
	if len(urls) > 0 {
		out = make([]sharedThing, len(urls))
		var wg sync.WaitGroup
		sem := make(chan struct{}, peekParallel)
		for i, u := range urls {
			out[i] = classifyLink(u)
			if !needsPeek(out[i]) {
				continue
			}
			wg.Add(1)
			go func(i int, u *url.URL) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				out[i] = refineLink(ctx, client, u, out[i])
			}(i, u)
		}
		wg.Wait()
	} else {
		for _, t := range leftover {
			if t = strings.TrimSpace(t); t != "" {
				out = append(out, classifyText(t))
			}
		}
	}

	seen := map[sharedThing]bool{}
	uniq := out[:0]
	for _, t := range out {
		if !seen[t] {
			seen[t] = true
			uniq = append(uniq, t)
		}
	}
	return uniq
}

const (
	peekParallel = 8
	peekTimeout  = 6 * time.Second
	peekLimit    = 512 << 10
	// Sites serve their og: tags to a browser; this reads as Safari on an iPhone.
	peekUserAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"
)

func webURL(s string) *url.URL {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return nil
	}
	if scheme := strings.ToLower(u.Scheme); scheme != "http" && scheme != "https" {
		return nil
	}
	return u
}

var linkRe = regexp.MustCompile(`https?://[^\s<>"'「」『』（）、。]+`)

// linksIn finds the web links in a text. Trailing punctuation that closes a
// sentence rather than the link is left off.
func linksIn(text string) []*url.URL {
	var out []*url.URL
	for _, m := range linkRe.FindAllString(text, -1) {
		m = strings.TrimRight(m, ".,;:!?)]}")
		if u := webURL(m); u != nil {
			out = append(out, u)
		}
	}
	return out
}

// ── links ────────────────────────────────────────────────────────────────────

// classifyLink files a link by its site and path alone. A maps link that
// names its place is that place, by name.
func classifyLink(u *url.URL) sharedThing {
	if name := mapPlaceName(u); name != "" {
		return sharedThing{Subtype: "place", Value: name}
	}
	subtype := linkSubtype(u)
	if subtype == "" {
		subtype = "url"
	}
	return sharedThing{Subtype: subtype, Value: u.String()}
}

// needsPeek: a link the rules could not place, or a maps link that named no
// place — where it ends up may.
func needsPeek(t sharedThing) bool {
	return t.Subtype == "url" || (t.Subtype == "place" && webURL(t.Value) != nil)
}

// refineLink looks at the page behind a link the rules could not place. The
// value stays the link that was shared.
func refineLink(ctx context.Context, client *http.Client, u *url.URL, t sharedThing) sharedThing {
	final, subtype, ok := peekLink(ctx, client, u)
	if !ok {
		return t
	}
	if name := mapPlaceName(final); name != "" {
		return sharedThing{Subtype: "place", Value: name}
	}
	if t.Subtype == "place" {
		return t
	}
	if subtype == "" {
		subtype = linkSubtype(final)
	}
	if subtype != "" {
		t.Subtype = subtype
	}
	return t
}

// peekLink fetches the page behind a link, only as far as its head: where it
// ended up and, when it says, what it is.
func peekLink(ctx context.Context, client *http.Client, u *url.URL) (final *url.URL, subtype string, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, peekTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", false
	}
	req.Header.Set("User-Agent", peekUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", false
	}
	defer resp.Body.Close()
	final = resp.Request.URL
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	switch {
	case strings.HasPrefix(ct, "image/"):
		return final, "image_url", true
	case strings.HasPrefix(ct, "audio/"):
		return final, "song", true
	case strings.HasPrefix(ct, "video/"):
		return final, "movie", true
	case resp.StatusCode >= 300 || !strings.Contains(ct, "html"):
		return final, "", true
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, peekLimit))
	page := string(body)
	if i := strings.Index(strings.ToLower(page), "</head>"); i >= 0 {
		page = page[:i]
	}
	return final, ogTypeSubtype(ogType(page)), true
}

var ogTypeRe = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<meta[^>]+property=["']og:type["'][^>]*content=["']([^"']+)["']`),
	regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]*property=["']og:type["']`),
}

func ogType(page string) string {
	for _, re := range ogTypeRe {
		if m := re.FindStringSubmatch(page); m != nil {
			return strings.TrimSpace(html.UnescapeString(m[1]))
		}
	}
	return ""
}

// ogTypeSubtype is the subtype for what a page says it is in its og:type.
// "website" and "article" say nothing about the thing, and are not answered.
func ogTypeSubtype(t string) string {
	t = strings.ToLower(t)
	switch t {
	case "music.song":
		return "song"
	case "music.album":
		return "album"
	case "music.musician":
		return "artist"
	case "music.playlist", "music.radio_station":
		return "music"
	case "book", "books.book":
		return "book"
	case "product", "product.item", "product.group", "og:product":
		return "product"
	case "restaurant.restaurant", "place", "business.business":
		return "place"
	case "hotel", "hotel.hotel":
		return "hotel"
	case "event", "events.event":
		return "event"
	}
	if strings.HasPrefix(t, "video.") {
		return "movie"
	}
	return ""
}

var (
	imageExts = set("jpg", "jpeg", "png", "gif", "webp", "heic", "heif", "avif", "bmp", "svg")
	audioExts = set("mp3", "m4a", "aac", "wav", "flac", "ogg", "opus", "aiff")
	videoExts = set("mp4", "mov", "m4v", "webm", "mkv", "avi")
)

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

var hostPrefixRe = regexp.MustCompile(`^(www|m)\.`)

// linkSubtype is what a link's site and path say it links to, or "". Sites
// by what they hold: music services by track / album / artist, video
// services by the watching page, bookshops, lodgings, shops, ticketing, job
// boards, maps and restaurant guides.
func linkSubtype(u *url.URL) string {
	if isGooglePhotosShareLink(u) || isImageLink(u) {
		return "image_url"
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(u.Path)), ".")
	switch {
	case audioExts[ext]:
		return "song"
	case videoExts[ext]:
		return "movie"
	case ext == "epub":
		return "book"
	}

	host := hostPrefixRe.ReplaceAllString(strings.ToLower(u.Hostname()), "")
	var parts []string
	for _, p := range strings.Split(strings.ToLower(u.Path), "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	p := "/" + strings.Join(parts, "/")
	on := func(domains ...string) bool {
		for _, d := range domains {
			if host == d || strings.HasSuffix(host, "."+d) {
				return true
			}
		}
		return false
	}
	has := func(segments ...string) bool {
		for _, part := range parts {
			for _, s := range segments {
				if part == s {
					return true
				}
			}
		}
		return false
	}

	// Music: one track, the record, who made it, else something musical.
	switch {
	case on("music.youtube.com"):
		switch {
		case has("watch"):
			return "song"
		case has("channel"):
			return "artist"
		}
		return "music"
	case on("soundcloud.com"):
		switch {
		case has("sets"):
			return "music"
		case len(parts) == 1:
			return "artist"
		case len(parts) == 2:
			return "song"
		}
		return "music"
	case on("open.spotify.com", "music.apple.com", "music.amazon.com", "music.amazon.co.jp",
		"deezer.com", "tidal.com", "music.line.me", "bandcamp.com", "awa.fm"):
		switch {
		case has("track", "tracks", "song", "songs"), has("album") && u.Query().Has("i"):
			return "song"
		case has("album", "albums"):
			return "album"
		case has("artist", "artists"):
			return "artist"
		case on("bandcamp.com") && len(parts) == 0:
			return "artist"
		}
		return "music"
	}

	// Video: the page a film or a clip is watched on.
	switch {
	case on("youtube.com"):
		for _, prefix := range []string{"/watch", "/shorts", "/live", "/embed"} {
			if strings.HasPrefix(p, prefix) {
				return "movie"
			}
		}
		return ""
	case on("youtu.be", "vimeo.com", "nicovideo.jp", "nico.ms", "netflix.com", "primevideo.com", "tver.jp",
		"abema.tv", "disneyplus.com", "hulu.jp", "unext.jp", "dailymotion.com", "twitch.tv"):
		return "movie"
	case on("tiktok.com") && has("video"),
		on("amazon.com", "amazon.co.jp") && has("video"),
		on("imdb.com", "filmarks.com", "eiga.com") && has("title", "movie"):
		return "movie"
	}

	switch {
	// Books.
	case on("books.google.com", "books.google.co.jp", "books.apple.com", "booklog.jp", "bookmeter.com",
		"honto.jp", "goodreads.com", "openlibrary.org", "read.amazon.com", "read.amazon.co.jp", "bookwalker.jp"),
		on("play.google.com") && strings.HasPrefix(p, "/store/books"):
		return "book"
	// Lodging, before shops: a travel site sells rooms.
	case on("booking.com") && has("hotel"),
		on("airbnb.com", "airbnb.jp") && has("rooms"),
		on("jalan.net", "ikyu.com", "hotels.com", "agoda.com", "relux.jp"),
		on("travel.rakuten.co.jp", "expedia.com", "expedia.co.jp") && strings.Contains(p, "hotel"):
		return "hotel"
	// Products.
	case on("amzn.to", "amzn.asia", "a.co"),
		on("amazon.com", "amazon.co.jp") && (has("dp") || strings.Contains(p, "/gp/product")),
		on("item.rakuten.co.jp", "store.shopping.yahoo.co.jp", "zozo.jp", "apps.apple.com"),
		on("jp.mercari.com", "mercari.com") && has("item"),
		on("ebay.com") && has("itm"):
		return "product"
	// Events and tickets.
	case on("eventbrite.com", "eventbrite.jp", "peatix.com", "connpass.com", "doorkeeper.jp", "eplus.jp",
		"t.pia.jp", "l-tike.com", "livepocket.jp"),
		on("meetup.com") && has("events"):
		return "event"
	// Jobs.
	case on("linkedin.com") && has("jobs"),
		on("indeed.com", "green-japan.com", "doda.jp", "bizreach.jp"),
		on("wantedly.com") && has("projects"):
		return "job"
	// Places: a map, or a restaurant's page.
	case on("maps.app.goo.gl", "maps.apple.com", "maps.google.com"),
		on("goo.gl") && has("maps"),
		isGoogleHost(host) && strings.HasPrefix(p, "/maps"),
		on("tabelog.com", "hotpepper.jp", "retty.me", "yelp.com", "tripadvisor.com", "tripadvisor.jp"):
		return "place"
	}
	return ""
}

// isGooglePhotosShareLink is a Google Photos share link, as the picture want
// takes them (engine/types/picture_types.go).
func isGooglePhotosShareLink(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	return host == "photos.app.goo.gl" ||
		(host == "photos.google.com" && strings.HasPrefix(u.Path, "/share/")) ||
		(host == "goo.gl" && strings.HasPrefix(u.Path, "/photos/"))
}

// isImageLink: a link to an image itself — its file says so, or it is served
// from Google's photo hosts (lh3.googleusercontent.com …), which have none.
func isImageLink(u *url.URL) bool {
	if imageExts[strings.TrimPrefix(strings.ToLower(path.Ext(u.Path)), ".")] {
		return true
	}
	host := strings.ToLower(u.Hostname())
	return strings.HasPrefix(host, "lh") && strings.HasSuffix(host, ".googleusercontent.com")
}

func isGoogleHost(host string) bool {
	return host == "google.com" || strings.HasPrefix(host, "google.") || strings.Contains(host, ".google.")
}

var coordinateRe = regexp.MustCompile(`^-?[0-9.]+,\s*-?[0-9.]+$`)

// mapPlaceName is the place a maps link names, when the link says it: Apple
// Maps' q, Google Maps' /maps/place/<name>/. A short link names nothing until
// it is followed.
func mapPlaceName(u *url.URL) string {
	host := hostPrefixRe.ReplaceAllString(strings.ToLower(u.Hostname()), "")
	q := u.Query()
	if host == "maps.apple.com" || host == "maps.apple" {
		for _, k := range []string{"q", "name"} {
			if v := strings.TrimSpace(q.Get(k)); v != "" {
				return v
			}
		}
		return ""
	}
	if host != "maps.google.com" && !(isGoogleHost(host) && strings.HasPrefix(u.Path, "/maps")) {
		return ""
	}
	parts := strings.Split(u.Path, "/")
	for i, part := range parts {
		if part == "place" && i+1 < len(parts) {
			if name := strings.TrimSpace(strings.ReplaceAll(parts[i+1], "+", " ")); name != "" {
				return name
			}
		}
	}
	// A coordinate is not a name; only a query that is words is.
	if v := strings.TrimSpace(q.Get("q")); v != "" && !coordinateRe.MatchString(v) {
		return v
	}
	return ""
}

// ── text ─────────────────────────────────────────────────────────────────────

var (
	emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)
	phoneRe = regexp.MustCompile(`^\+?[0-9][0-9\- ()]{7,}[0-9]$`)
	// 2026-10-08, 2026/10/8, 2026年10月8日, optionally with a time.
	dateRe     = regexp.MustCompile(`^\d{4}([-/.])\d{1,2}([-/.])\d{1,2}$|^\d{4}年\d{1,2}月\d{1,2}日$`)
	dateTimeRe = regexp.MustCompile(`^(\d{4}[-/.]\d{1,2}[-/.]\d{1,2}|\d{4}年\d{1,2}月\d{1,2}日)(\s*|T)\(?[月火水木金土日]?\)?\s*\d{1,2}[:時]\d{2}`)
	// A Japanese address: a postal code, or a prefecture followed by more.
	addressRe = regexp.MustCompile(`^(〒\s*\d{3}-?\d{4}|(東京都|北海道|(京都|大阪)府|\p{Han}{2,3}県)\p{Han}+[市区町村郡])`)
)

// classifyText files a text by what it is as a whole.
func classifyText(t string) sharedThing {
	single := !strings.ContainsAny(t, "\n\r")
	digits := 0
	for _, r := range t {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	switch {
	case single && emailRe.MatchString(t):
		return sharedThing{Subtype: "email", Value: t}
	case single && phoneRe.MatchString(t) && digits >= 9 && digits <= 15:
		return sharedThing{Subtype: "phone", Value: t}
	case single && dateTimeRe.MatchString(t):
		return sharedThing{Subtype: "datetime", Value: t}
	case single && dateRe.MatchString(t):
		return sharedThing{Subtype: "date", Value: t}
	case addressRe.MatchString(t):
		return sharedThing{Subtype: "address", Value: t}
	}
	return sharedThing{Subtype: "string", Value: t}
}
