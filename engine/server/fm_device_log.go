package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// What a device running the robot's model recorded about itself.
//
// The turns (fm_turns.go) say what was asked, which tools ran and what was
// answered; they end where the device takes the answer back. What the device
// did after that — the window it showed, the voice it read out, the intent
// returning — and why it stopped, if it stopped, happens where this server
// cannot see. An iPhone app crashing on the action button left a turn here
// that answered fine and nothing at all about the crash a second later.
//
// So the device keeps its own record and sends it here: every step of a
// question as it happens, and — on the next launch, since a crashing process
// sends nothing — what its previous run wrote to stderr, which is where Swift
// puts "Fatal error: …".

type fmDeviceLogEntry struct {
	At     string `json:"at"`
	Device string `json:"device,omitempty"`
	Event  string `json:"event"`
	Detail string `json:"detail,omitempty"`
	// Received: when this server got it — a previous run's lines arrive late.
	Received string `json:"received,omitempty"`
}

var fmDeviceLogMu sync.Mutex

// fmDeviceLogMax: past this the log starts over (the old one kept as .1).
const fmDeviceLogMax = 5 << 20

func fmDeviceLogPath() string { return thingPath("fm-device-log.jsonl") }

// POST /api/v1/fm/device-log  {"device": "...", "entries": [{at, event, detail}]}
func (s *Server) handleFMDeviceLogPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device  string             `json:"device"`
		Entries []fmDeviceLogEntry `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now().Format(time.RFC3339)
	path := fmDeviceLogPath()

	fmDeviceLogMu.Lock()
	defer fmDeviceLogMu.Unlock()
	if st, err := os.Stat(path); err == nil && st.Size() > fmDeviceLogMax {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	for _, e := range req.Entries {
		if e.Device == "" {
			e.Device = req.Device
		}
		e.Received = now
		line, _ := json.Marshal(e)
		_, _ = f.Write(append(line, '\n'))
	}
	fmWriteJSON(w, map[string]any{"stored": len(req.Entries)})
}

// GET /api/v1/fm/device-log?tail=200 — the last entries, oldest first.
func (s *Server) handleFMDeviceLogGet(w http.ResponseWriter, r *http.Request) {
	tail := 200
	if n, err := strconv.Atoi(r.URL.Query().Get("tail")); err == nil && n > 0 {
		tail = n
	}
	fmDeviceLogMu.Lock()
	defer fmDeviceLogMu.Unlock()
	entries := []fmDeviceLogEntry{}
	f, err := os.Open(fmDeviceLogPath())
	if err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
		for sc.Scan() {
			var e fmDeviceLogEntry
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				entries = append(entries, e)
				if len(entries) > tail {
					entries = entries[1:]
				}
			}
		}
	}
	fmWriteJSON(w, map[string]any{"entries": entries})
}
