package server

import (
	"net/http"
	"strings"
	"time"

	mywant "mywant/engine/core"
)

// webInspectorUsesKey is the gui_state key holding where the bookmarklet has
// actually run: one entry per browser (User-Agent) and address (origin), the
// newest first.
//
// A web page cannot see another browser's bookmarks, so "is the bookmarklet
// installed on this device?" has no direct answer. "Has it run from this
// device?" does: the bookmarklet's first request is active-inspection, sent
// from that device's own browser, through the address it was made for. The
// GUI's setup panel reads this to mark a path as working — this browser, a
// phone, or from outside — rather than asking the user whether it is.
const webInspectorUsesKey = "web_inspector_uses"

// webInspectorUsesMax keeps the record small: it answers "has it worked
// here", not "every time it ran".
const webInspectorUsesMax = 20

// recordWebInspectorUse notes that the bookmarklet ran from this request's
// browser through this request's address. Kept in gui_state so it persists
// and every GUI reads the same answer.
func (s *Server) recordWebInspectorUse(r *http.Request) {
	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil {
		return
	}
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	origin := scheme + "://" + r.Host
	ua := r.UserAgent()
	entry := map[string]any{
		"ua":        ua,
		"origin":    origin,
		"character": r.URL.Query().Get("character"),
		"at":        time.Now().UnixMilli(),
	}

	next := []any{entry}
	if prev, ok := want.GetAllState()[webInspectorUsesKey].([]any); ok {
		for _, p := range prev {
			m, ok := p.(map[string]any)
			if !ok {
				continue
			}
			// The same browser through the same address is the same path;
			// only its latest run is worth keeping.
			if pua, _ := m["ua"].(string); strings.EqualFold(pua, ua) {
				if po, _ := m["origin"].(string); po == origin {
					continue
				}
			}
			if len(next) >= webInspectorUsesMax {
				break
			}
			next = append(next, m)
		}
	}
	want.StoreState(webInspectorUsesKey, next)
	// Explicit, or GET /gui/state leaves it out (see updateGUIState).
	if !mywant.Contains(want.ProvidedStateFields, webInspectorUsesKey) {
		want.ProvidedStateFields = append(want.ProvidedStateFields, webInspectorUsesKey)
	}
	resp := guiStateResponse{Seq: nextGUIStateSeq(), State: s.guiStateWithConfig(want)}
	go broadcastSSE("gui_state", resp)
}
