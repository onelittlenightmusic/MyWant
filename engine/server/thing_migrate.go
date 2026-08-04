package server

import (
	"log"
	"os"
	"path/filepath"
)

// What the app calls a remembered value is a Thing. It was called a Memo, and
// three files on disk still carried that name.
//
// The rename happens once, on the way to the new path: if the new file is not
// there and the old one is, the old one is moved. Nothing is copied and nothing
// is merged — a half-written pair of files is worse than either name.
//
// Kept as its own file so the whole of the compatibility story is in one place
// and can be deleted in one piece once no installation has the old names.
var thingFileRenames = map[string]string{
	"thing.yaml":        "memo.yaml",
	"thing-labels.yaml": "memo-labels.yaml",
	"thing-events.yaml": "memo-events.yaml",
}

// thingPath returns the home-relative path for one of the Thing stores, moving
// the pre-rename file into place the first time it is asked for.
func thingPath(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return name
	}
	current := filepath.Join(home, ".mywant", name)

	former, renamed := thingFileRenames[name]
	if !renamed {
		return current
	}
	if _, err := os.Stat(current); err == nil {
		return current // already on the new name
	}
	old := filepath.Join(home, ".mywant", former)
	if _, err := os.Stat(old); err != nil {
		return current // nothing to move
	}
	if err := os.Rename(old, current); err != nil {
		// Renaming failed (a read-only home, say). Keep reading the old file
		// rather than starting empty and looking like the data was lost.
		log.Printf("[SERVER] could not rename %s to %s (%v) — still reading the old name", former, name, err)
		return old
	}
	log.Printf("[SERVER] renamed %s to %s", former, name)
	return current
}
