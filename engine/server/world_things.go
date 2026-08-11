package server

import (
	"log"
	"os"
	"path/filepath"
)

// ── Things belong to a world ─────────────────────────────────────────────────
//
// Wants are saved twice: continuously into state.yaml (the running truth) and
// as a snapshot into <worlds>/<name>.yaml when you leave a world or save it by
// hand. Things need no such pair. A thing is an id, a catalog, a value and some
// labels — nothing running, nothing to reconcile — and the stores already write
// their whole file on every change. So there is one copy, and the world is
// where it lives: point a store's path at the open world's file and every save
// it already performs is a save into that world.
//
// That is also why these are files of their own rather than a section inside
// <name>.yaml. A thing is edited far more often than a world is switched, and
// sharing the file would mean rewriting every want in the world each time
// somebody renames a place — with two writers on one file whose cadences have
// nothing to do with each other.
//
// They live in a subdirectory for the reason the thumbnails do: listWorlds
// enumerates worlds by scanning *.yaml in the worlds directory, and a sibling
// file would list itself as a world.

// worldThingsDir returns <worldsDir>/things, where every world's things live.
func worldThingsDir(dir string) string {
	return filepath.Join(dir, "things")
}

// worldThingsPath returns <worldsDir>/things/<name>.yaml — the world's things.
func worldThingsPath(dir, name string) string {
	return filepath.Join(worldThingsDir(dir), name+".yaml")
}

// worldThingLabelsPath returns <worldsDir>/things/<name>-labels.yaml — where
// each of those things sits on the canvas and which groups it is in.
//
// Beside the things and named after the same world, because they are one fact
// in two files: a thing restored without its labels arrives with no position
// and no group, which on the canvas is indistinguishable from not arriving.
func worldThingLabelsPath(dir, name string) string {
	return filepath.Join(worldThingsDir(dir), name+"-labels.yaml")
}

// useWorldThings points both thing stores at the given world's files, so every
// read and write from here on is that world's.
//
// Nothing is flushed on the way out: the stores have already written each
// change as it happened, so the world being left is up to date by the time
// anyone asks to leave it.
func (s *Server) useWorldThings(name string) {
	dir, err := s.worldsDir()
	if err != nil {
		log.Printf("[WARN] worlds: cannot access worlds directory for things: %v", err)
		return
	}
	if err := os.MkdirAll(worldThingsDir(dir), 0o755); err != nil {
		log.Printf("[WARN] worlds: cannot create things directory: %v", err)
		return
	}
	s.thingStore.SetPath(worldThingsPath(dir, name))
	s.thingLabels.SetPath(worldThingLabelsPath(dir, name))
}

// adoptLegacyThingsInto moves the pre-world thing files into a world, once.
//
// Everything anyone has entered so far was collected before things had a world
// to belong to, and it belongs to the world that was open while it was being
// collected — which is the one this runs for. Copied rather than moved: the
// originals stay put as they are, so a downgrade to a build without worlds
// still finds its data where it left it.
//
// A no-op once the world has files of its own, so it cannot overwrite a world
// that has since diverged.
func (s *Server) adoptLegacyThingsInto(name string) {
	dir, err := s.worldsDir()
	if err != nil {
		return
	}
	pairs := [2][2]string{
		{thingPath("thing.yaml"), worldThingsPath(dir, name)},
		{thingPath("thing-labels.yaml"), worldThingLabelsPath(dir, name)},
	}
	for _, p := range pairs {
		from, to := p[0], p[1]
		if _, err := os.Stat(to); err == nil {
			continue // this world already has its own
		}
		data, err := os.ReadFile(from)
		if err != nil {
			continue // nothing collected under the old name
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			log.Printf("[WARN] worlds: cannot create things directory: %v", err)
			continue
		}
		if err := os.WriteFile(to, data, 0o644); err != nil {
			log.Printf("[WARN] worlds: could not adopt %s into world %q: %v", from, name, err)
			continue
		}
		log.Printf("[Worlds] Adopted %s into world %q", filepath.Base(from), name)
	}
}

// copyWorldThings duplicates one world's things onto another name, for the
// paths that snapshot the running state under a name of the caller's choosing
// (POST /worlds/{name}/save). Missing source files are not an error: a world
// with no things is a world with no things.
func copyWorldThings(dir, from, to string) {
	if from == to {
		return
	}
	if err := os.MkdirAll(worldThingsDir(dir), 0o755); err != nil {
		log.Printf("[WARN] worlds: cannot create things directory: %v", err)
		return
	}
	pairs := [2][2]string{
		{worldThingsPath(dir, from), worldThingsPath(dir, to)},
		{worldThingLabelsPath(dir, from), worldThingLabelsPath(dir, to)},
	}
	for _, p := range pairs {
		data, err := os.ReadFile(p[0])
		if err != nil {
			continue
		}
		if err := os.WriteFile(p[1], data, 0o644); err != nil {
			log.Printf("[WARN] worlds: could not copy things to %q: %v", to, err)
		}
	}
}
