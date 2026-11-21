package server

// The conversations on the board: every pair of wants that has talked (one
// heard the other's murmur, hear.go), the latest of each pair, kept in
// ~/.mywant/converse.json so they outlive a restart. The board draws them all
// as it opens (GET /api/v1/converse), and each new one as it comes (the
// converse event) — a page opened later, or by another character, sees the
// same talk as one open when it happened.
//
//	GET /api/v1/converse → [{from, to, murmurId, at}]

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	conversesMu sync.Mutex
	converses   map[string]wantConverse // by pair, either way round
)

func conversesFile() string { return initiativeHomeFile("converse.json") }

func conversePair(c wantConverse) string {
	a, b := c.From, c.To
	if b < a {
		a, b = b, a
	}
	return a + "|" + b
}

// loadConversesLocked reads the kept conversations, once.
func loadConversesLocked() {
	if converses != nil {
		return
	}
	converses = map[string]wantConverse{}
	data, err := os.ReadFile(conversesFile())
	if err != nil {
		return
	}
	var list []wantConverse
	if json.Unmarshal(data, &list) == nil {
		for _, c := range list {
			converses[conversePair(c)] = c
		}
	}
}

// noteConverse keeps a conversation: the pair's latest.
func noteConverse(c wantConverse) {
	if c.At == "" {
		c.At = time.Now().Format(time.RFC3339)
	}
	conversesMu.Lock()
	defer conversesMu.Unlock()
	loadConversesLocked()
	converses[conversePair(c)] = c
	list := make([]wantConverse, 0, len(converses))
	for _, v := range converses {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].At < list[j].At })
	if data, err := json.MarshalIndent(list, "", "  "); err == nil {
		tmp := conversesFile() + ".tmp"
		if os.WriteFile(tmp, data, 0o644) == nil {
			_ = os.Rename(tmp, conversesFile())
		}
	}
}

// listConverses handles GET /api/v1/converse: every pair that has talked,
// both of them still there, oldest first.
func (s *Server) listConverses(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.handleOptions(w, r)
		return
	}
	present := map[string]bool{}
	if s.globalBuilder != nil {
		for _, want := range s.globalBuilder.GetWants() {
			present[want.Metadata.ID] = true
		}
	}
	conversesMu.Lock()
	loadConversesLocked()
	list := make([]wantConverse, 0, len(converses))
	for _, c := range converses {
		if present[c.From] && present[c.To] {
			list = append(list, c)
		}
	}
	conversesMu.Unlock()
	sort.Slice(list, func(i, j int) bool {
		if list[i].At != list[j].At {
			return list[i].At < list[j].At
		}
		return strings.Compare(conversePair(list[i]), conversePair(list[j])) < 0
	})
	s.JSONResponse(w, http.StatusOK, list)
}
