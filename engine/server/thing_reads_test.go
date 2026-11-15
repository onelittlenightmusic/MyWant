package server

import (
	"path/filepath"
	"testing"
	"time"
)

func TestThingReadStoreMarksAndKeepsTheNewest(t *testing.T) {
	dir := t.TempDir()
	s := &ThingReadStore{path: filepath.Join(dir, "r.yaml"), reads: map[string]*ThingRead{}}
	t1 := time.Date(2026, 11, 15, 10, 0, 30, 0, time.Local)
	t2 := t1.Add(5 * time.Minute)
	s.Mark("荻窪", "transit_search.from", t2)
	s.Mark("荻窪", "transit_search.to", t1)
	s.Mark("荻窪", "transit_search.from", t1) // older: not taken
	r, ok := s.Get("荻窪")
	if !ok || r.LastRead != t2.Truncate(time.Minute).Format(time.RFC3339) {
		t.Fatalf("lastRead = %+v", r)
	}
	if r.ReadAs["transit_search.from"] != r.LastRead || r.ReadAs["transit_search.to"] != t1.Truncate(time.Minute).Format(time.RFC3339) {
		t.Fatalf("readAs = %v", r.ReadAs)
	}
	s.Flush()
	// Another world, then back: read from its file.
	s.SetPath(filepath.Join(dir, "other.yaml"))
	if _, ok := s.Get("荻窪"); ok {
		t.Fatal("another world has its own reads")
	}
	s.SetPath(filepath.Join(dir, "r.yaml"))
	if got, _ := s.Get("荻窪"); got.LastRead != r.LastRead {
		t.Fatalf("back: %+v", got)
	}
}
