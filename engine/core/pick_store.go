package mywant

// pick_store.go — what a character typed on a keyboard and what they chose
// from what it offered for those letters: the Mac's input source and the
// iPhone keyboard (mywant-input, mywant-ios), whose suggestions are the
// board's wants, things and values. Kept per character, newest last, so the
// keyboard can offer the same choice for the same words next time and show
// Apple FM how this person chooses.
//
// Format: JSON Lines, one file per character, ~/.mywant/picks/<id>.jsonl,
// cut back to the newest pickKeep lines when it grows past pickKeep*2.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// PickChoice is what was chosen from the suggestions.
type PickChoice struct {
	Kind   string `json:"kind"`             // "want" | "thing"
	Name   string `json:"name"`             // the want's or thing's name
	WantID string `json:"wantId,omitempty"` // a want's id, when it has one
	Field  string `json:"field,omitempty"`  // the field whose value was typed; empty for the name itself
	Text   string `json:"text"`             // what went into the text
}

// Pick is one typed string and what was chosen for it.
type Pick struct {
	At      string     `json:"at"`      // RFC3339 UTC, set by the server
	Typed   string     `json:"typed"`   // the letters exactly as typed, or the text picked before the caret
	Chose   PickChoice `json:"chose"`
	Via     string     `json:"via"`     // "match" (the letters) | "fm" (Apple FM's choice) | "history"
	Rank    int        `json:"rank"`    // where it was among the suggestions, 0 the first
	Surface string     `json:"surface"` // "mac-input" | "ios-keyboard"
}

const pickKeep = 1000

// pickIDPattern keeps a character id a plain file name.
var pickIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type pickStore struct {
	mu  sync.Mutex
	dir string
}

var (
	globalPickStore     *pickStore
	globalPickStoreOnce sync.Once
)

func getPickStore() *pickStore {
	globalPickStoreOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		globalPickStore = &pickStore{dir: filepath.Join(home, ".mywant", "picks")}
	})
	return globalPickStore
}

func (s *pickStore) path(characterID string) (string, error) {
	if !pickIDPattern.MatchString(characterID) {
		return "", fmt.Errorf("bad character id %q", characterID)
	}
	return filepath.Join(s.dir, characterID+".jsonl"), nil
}

// add appends picks, stamped now, and trims the file when it has grown.
func (s *pickStore) add(characterID string, picks []Pick, now time.Time) error {
	path, err := s.path(characterID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, p := range picks {
		if p.At == "" {
			p.At = now.UTC().Format(time.RFC3339)
		}
		line, err := json.Marshal(p)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	all, err := s.readLocked(path)
	if err != nil || len(all) <= pickKeep*2 {
		return err
	}
	return s.writeLocked(path, all[len(all)-pickKeep:])
}

// list returns the newest `limit` picks, newest first.
func (s *pickStore) list(characterID string, limit int) ([]Pick, error) {
	path, err := s.path(characterID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	all, err := s.readLocked(path)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	out := make([]Pick, 0, limit)
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, all[i])
	}
	return out, nil
}

func (s *pickStore) clear(characterID string) error {
	path, err := s.path(characterID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *pickStore) readLocked(path string) ([]Pick, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Pick
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var p Pick
		// A line that does not read (cut short by a crash) is skipped.
		if json.Unmarshal(sc.Bytes(), &p) == nil {
			out = append(out, p)
		}
	}
	return out, sc.Err()
}

func (s *pickStore) writeLocked(path string, picks []Pick) error {
	var buf bytes.Buffer
	for _, p := range picks {
		line, err := json.Marshal(p)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// AddPicks records what a character chose from a keyboard's suggestions.
func AddPicks(characterID string, picks []Pick) error {
	return getPickStore().add(characterID, picks, time.Now())
}

// ListPicks returns a character's newest picks, newest first.
func ListPicks(characterID string, limit int) ([]Pick, error) {
	return getPickStore().list(characterID, limit)
}

// ClearPicks forgets everything a character chose.
func ClearPicks(characterID string) error {
	return getPickStore().clear(characterID)
}
