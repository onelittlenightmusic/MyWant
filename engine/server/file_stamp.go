package server

import (
	"os"
	"time"
)

// fileStamp is what a file looked like when a store last read or wrote it:
// its path, size and modification time. A store keeps what it parsed beside
// the stamp and parses again only when the file no longer matches — so a
// request that reads the store once per want, or a poll every few seconds,
// costs a stat instead of a YAML parse of the whole file. A file written by
// someone else (another process, a restore) changes its stamp, so it is read
// again as before.
type fileStamp struct {
	path string
	size int64
	mod  time.Time
	ok   bool
}

// stampOf is the stamp of path as it is now; ok is false when it cannot be
// read (missing included — a missing file is never "unchanged").
func stampOf(path string) fileStamp {
	fi, err := os.Stat(path)
	if err != nil {
		return fileStamp{path: path}
	}
	return fileStamp{path: path, size: fi.Size(), mod: fi.ModTime(), ok: true}
}

// matches is whether the file at path is still the one this stamp was taken of.
func (s fileStamp) matches(path string) bool {
	if !s.ok || s.path != path {
		return false
	}
	now := stampOf(path)
	return now.ok && now.size == s.size && now.mod.Equal(s.mod)
}
