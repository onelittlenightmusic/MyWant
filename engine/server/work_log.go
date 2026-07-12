package server

// work_log.go — activity logger for Robot and CursorMan events.
//
// Format: JSON-Lines (one JSON object per line) in ~/.mywant/work.log.
//
// Each entry carries an "important" flag.  Rotation (every 5 min):
//   - Entries ≥ 1 hour old AND important  → appended to the daily gzip archive
//     (~/.mywant/work-YYYY-MM-DD.log.gz).
//   - Entries ≥ 1 hour old AND NOT important → discarded (plain movements).
//   - Entries < 1 hour old → kept in work.log unchanged.
//
// "Important" rules:
//   robot  — always important (robot has a non-empty action or target != "none")
//   cursor — important when effectType != "" (aura/effect applied to a want)
//   gui_state — important when sidebar_want_id changes (want selection)

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WorkLogEntry is one line in work.log.
type WorkLogEntry struct {
	Ts        string         `json:"ts"`        // RFC3339Nano UTC
	Type      string         `json:"type"`      // "robot" | "cursor" | "gui_state"
	Important bool           `json:"important"` // kept in archive after 1-hour rotation
	Data      map[string]any `json:"data"`
}

// workLogger is a singleton logger that writes to ~/.mywant/work.log.
type workLogger struct {
	mu   sync.Mutex
	path string // absolute path to work.log
}

var (
	globalWorkLogger *workLogger
	workLogOnce      sync.Once
)

// getWorkLogger returns (and lazily initialises) the global work logger.
// The rotation goroutine is started exactly once.
func getWorkLogger() *workLogger {
	workLogOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[worklog] cannot determine home dir: %v", err)
			return
		}
		dir := filepath.Join(home, ".mywant")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("[worklog] cannot create ~/.mywant: %v", err)
			return
		}
		globalWorkLogger = &workLogger{
			path: filepath.Join(dir, "work.log"),
		}
		go globalWorkLogger.runRotation()
	})
	return globalWorkLogger
}

// AppendWorkLog serialises entry to work.log (thread-safe).
func AppendWorkLog(entry WorkLogEntry) {
	wl := getWorkLogger()
	if wl == nil {
		return
	}
	if entry.Ts == "" {
		entry.Ts = time.Now().UTC().Format(time.RFC3339Nano)
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	wl.mu.Lock()
	defer wl.mu.Unlock()
	f, err := os.OpenFile(wl.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("[worklog] open error: %v", err)
		return
	}
	defer f.Close()
	_, _ = f.Write(b)
	_, _ = f.Write([]byte("\n"))
}

// runRotation calls rotate() every 5 minutes indefinitely.
func (wl *workLogger) runRotation() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		wl.rotate()
	}
}

// rotate separates entries into three buckets:
//   recent (< 1h)      → rewritten to work.log
//   old + important    → appended to the daily .log.gz archive
//   old + !important   → discarded
func (wl *workLogger) rotate() {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	data, err := os.ReadFile(wl.path)
	if err != nil {
		// File may not exist yet — nothing to do.
		return
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return
	}

	cutoff := time.Now().UTC().Add(-1 * time.Hour)

	var keep    [][]byte // stays in work.log
	var archive [][]byte // compressed into daily .gz

	for _, raw := range bytes.Split(data, []byte("\n")) {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			continue
		}
		var entry WorkLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Keep malformed lines so we don't silently drop data.
			keep = append(keep, line)
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, entry.Ts)
		if err != nil || t.After(cutoff) {
			// Recent (or unparseable timestamp): keep in work.log.
			keep = append(keep, line)
		} else if entry.Important {
			// Old but important: compress into daily archive.
			archive = append(archive, line)
		}
		// Old + unimportant: silently dropped.
	}

	// Rewrite work.log.
	var newData []byte
	if len(keep) > 0 {
		newData = append(bytes.Join(keep, []byte("\n")), '\n')
	}
	if err := os.WriteFile(wl.path, newData, 0o644); err != nil {
		log.Printf("[worklog] rotate rewrite error: %v", err)
	}

	// Append important old entries to the daily gzip archive.
	if len(archive) > 0 {
		archiveName := time.Now().UTC().Format("work-2006-01-02.log.gz")
		archivePath := filepath.Join(filepath.Dir(wl.path), archiveName)
		if err := appendToGzip(archivePath, archive); err != nil {
			log.Printf("[worklog] gzip archive error: %v", err)
		}
	}
}

// appendToGzip reads any existing gzip file at path, then rewrites it with
// the existing content followed by the new lines.
func appendToGzip(path string, lines [][]byte) error {
	// Read existing gzipped content (may not exist yet).
	var existing []byte
	if f, err := os.Open(path); err == nil {
		gr, gerr := gzip.NewReader(f)
		if gerr == nil {
			existing, _ = io.ReadAll(gr)
			_ = gr.Close()
		}
		_ = f.Close()
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	if len(existing) > 0 {
		if _, err := gw.Write(existing); err != nil {
			return err
		}
		// Ensure newline between old block and new lines.
		if existing[len(existing)-1] != '\n' {
			if _, err := gw.Write([]byte("\n")); err != nil {
				return err
			}
		}
	}
	for _, line := range lines {
		if _, err := gw.Write(line); err != nil {
			return err
		}
		if _, err := gw.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return nil
}
