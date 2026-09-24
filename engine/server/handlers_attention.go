package server

import (
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	mywant "mywant/engine/core"
)

// What needs a person, across everything that is running — the list behind
// the control pill's attention button, which warps to where each thing is.
//
// Four kinds, each answering "go where?" differently:
//   - approval:    a want is waiting on a yes (pending_command, or the
//                  waiting_user_action status). Answered in the GUI.
//   - want_error:  a want failed (failed / config_error / module_error).
//                  Looked at in the GUI.
//   - web_failed:  an automated run in a browser tab (browser-run) came back
//                  with an error. Looked at in that tab — its URL.
//   - needs_human: such a run reached a page only a person can get past (a
//                  login, a CAPTCHA), as the extension judged it. That tab too.
//
// The first two are read off the wants every time; the last two are events
// the extension reports, kept here until a later run of the same URL
// succeeds — the tab working again is what makes them no longer news.

// AttentionItem is one thing needing a person.
type AttentionItem struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	WantID string `json:"want_id,omitempty"`
	// Where to go: a page URL to bring its tab forward (or open), or "" for
	// the GUI itself.
	URL string `json:"url,omitempty"`
	At  int64  `json:"at"`
}

var (
	webAttentionMu sync.Mutex
	webAttention   = map[string]AttentionItem{} // by URL
)

// recordWebRunOutcome keeps or clears the tab-side attention for a run's URL.
func recordWebRunOutcome(url string, res browserRunResult) {
	if url == "" {
		return
	}
	needsHuman := ""
	if res.Result != nil {
		if v, ok := res.Result["needs_human"].(string); ok {
			needsHuman = v
		}
	}
	webAttentionMu.Lock()
	defer webAttentionMu.Unlock()
	switch {
	case needsHuman != "":
		webAttention[url] = AttentionItem{
			ID: "web:" + url, Kind: "needs_human", Title: hostnameOfURL(url),
			Detail: needsHuman, URL: url, At: time.Now().UnixMilli(),
		}
	case res.Error != "":
		webAttention[url] = AttentionItem{
			ID: "web:" + url, Kind: "web_failed", Title: hostnameOfURL(url),
			Detail: res.Error, URL: url, At: time.Now().UnixMilli(),
		}
	default:
		delete(webAttention, url)
	}
}

// getAttention handles GET /api/v1/attention — newest first.
func (s *Server) getAttention(w http.ResponseWriter, r *http.Request) {
	items := []AttentionItem{}
	for _, want := range s.globalBuilder.GetAllWantStates() {
		if want.Metadata.IsSystemWant && want.Metadata.Name != "robot" {
			continue
		}
		id := want.Metadata.ID
		title := want.Metadata.Name
		status := want.GetStatus()
		pending := strings.TrimSpace(mywant.GetCurrent(want, "pending_command", ""))
		switch {
		case pending != "" || status == mywant.WantStatusWaitingUserAction:
			items = append(items, AttentionItem{
				ID: "approval:" + id, Kind: "approval", Title: title, Detail: pending, WantID: id,
				At: time.Now().UnixMilli(),
			})
		case status == mywant.WantStatusFailed || status == mywant.WantStatusConfigError || status == mywant.WantStatusModuleError:
			items = append(items, AttentionItem{
				ID: "error:" + id, Kind: "want_error", Title: title, Detail: string(status), WantID: id,
				At: time.Now().UnixMilli(),
			})
		}
	}
	webAttentionMu.Lock()
	for _, it := range webAttention {
		items = append(items, it)
	}
	webAttentionMu.Unlock()
	// Approvals first — someone is blocked on them — then newest.
	rank := map[string]int{"approval": 0, "needs_human": 1, "web_failed": 2, "want_error": 3}
	sort.SliceStable(items, func(i, j int) bool {
		if rank[items[i].Kind] != rank[items[j].Kind] {
			return rank[items[i].Kind] < rank[items[j].Kind]
		}
		return items[i].At > items[j].At
	})
	s.JSONResponse(w, http.StatusOK, map[string]any{"items": items})
}
