package server

import (
	"os"
	"path/filepath"
	"testing"

	mwlabels "mywant/engine/labels"
)

// A kept picture worn by two — a picture chosen from another's kept file is
// the same file — stays while either wears it (removeArtFile / artInUse).
// Taking a skill's test picture away once deleted 国分寺's floor (2026-11-03).
func TestArtFileKeptWhileWorn(t *testing.T) {
	dir := t.TempDir()
	s := &Server{
		config:      Config{ConfigPath: filepath.Join(dir, "config.yaml")},
		thingLabels: &ThingLabelStore{mwlabels.NewFileStore(filepath.Join(dir, "labels.yaml"))},
	}
	artDir, err := s.artDir()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(artDir, "a-0123456789abcdef.png")
	if err := os.WriteFile(file, []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}
	url := artURLPrefix + "a-0123456789abcdef.png"
	_ = s.thingLabels.Set("thing-a", artLabel, url)
	_ = s.thingLabels.Set("thing-b", constellationArtPrefix+"国分寺", url+"#scale=1.5")

	_ = s.thingLabels.Remove("thing-a", artLabel)
	s.removeArtFile(url)
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("removed while another still wears it: %v", err)
	}

	_ = s.thingLabels.Remove("thing-b", constellationArtPrefix+"国分寺")
	s.removeArtFile(url)
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("kept once nothing wears it: %v", err)
	}
}
