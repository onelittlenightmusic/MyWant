package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// A way to one thing on a page (what POST /web-wants/open takes), kept a
// short while under an id so a link can carry it: the app's own browser,
// asked to open its page in the person's browser instead, cannot address that
// browser's extension — it does not know it — so it keeps the way here and
// opens the page with "#mywant-guide=<id>" on it. The extension, finding that
// on a tab it did not open, fetches the way and shows it in that tab.
//
//	POST /api/v1/web-wants/guides        {url, title, steps, elements, then} → {id}
//	GET  /api/v1/web-wants/guides/{id}   → the way

// webGuideTTL is how long a kept way waits to be opened.
const webGuideTTL = 30 * time.Minute

type keptWebGuide struct {
	way json.RawMessage
	at  time.Time
}

var (
	webGuidesMu sync.Mutex
	webGuides   = map[string]keptWebGuide{}
)

func (s *Server) keepWebGuide(w http.ResponseWriter, r *http.Request) {
	var way json.RawMessage
	if err := DecodeRequest(r, &way); err != nil || len(way) == 0 {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", "")
		return
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	now := time.Now()
	webGuidesMu.Lock()
	for k, g := range webGuides {
		if now.Sub(g.at) > webGuideTTL {
			delete(webGuides, k)
		}
	}
	webGuides[id] = keptWebGuide{way: way, at: now}
	webGuidesMu.Unlock()
	s.JSONResponse(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) getWebGuide(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	webGuidesMu.Lock()
	g, ok := webGuides[id]
	webGuidesMu.Unlock()
	if !ok || time.Since(g.at) > webGuideTTL {
		s.JSONError(w, r, http.StatusNotFound, "no such way", id)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(g.way)
}
