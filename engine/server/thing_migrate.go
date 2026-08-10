package server

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	mywant "mywant/engine/core"
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

// ── Definitions move out of characters.yaml and into the ledger ──────────────
//
// A name given to a value used to live in two places: the catalog kept the
// name, and the character who signed it kept the name→value pair. That made the
// character file the only place a thing's value existed, which is backwards —
// the thing is the record. The ledger now carries name, value, author, want and
// time on one line, so the copy on the character is redundant.
//
// This moves each definition mark into the ledger (if it is not already there)
// and then clears it from the character, leaving characters.yaml holding only
// want-type bindings, which really are the character's own.
func (s *Server) migrateAuraDefinitionsToLedger() {
	if s.thingEvents == nil {
		return
	}

	// What the ledger already knows — and knows the VALUE of. An entry recorded
	// before the ledger carried NamedValue holds only a name, so the character's
	// copy is still the only place that value exists: it has to be written
	// across before the character is cleared, or clearing loses it. Treating
	// those as "already known" is exactly the mistake that dropped five values
	// the first time this ran.
	known := map[string]bool{}
	for _, d := range s.thingEvents.Definitions() {
		if d.Value == nil {
			continue
		}
		known[d.CharacterID+"\x00"+d.Subtype+"\x00"+d.Name] = true
	}

	moved, dropped := 0, 0
	for _, c := range mywant.ListCharacters() {
		for _, mark := range c.AuraDefaults {
			if mark.Target.IsBinding() || mark.Target.Name == "" {
				continue
			}
			// A URL is not a name. These come from naming a field whose value
			// was already a URL and accepting the suggested name unchanged, so
			// the "name" repeats the value and points at nothing a person would
			// look up. Dropped rather than carried forward.
			if strings.HasPrefix(mark.Target.Name, "http://") || strings.HasPrefix(mark.Target.Name, "https://") {
				clearAuraDefinition(c.ID, mark.Target)
				dropped++
				continue
			}
			if !known[c.ID+"\x00"+mark.Target.Kind+"\x00"+mark.Target.Name] {
				s.recordThingEvent(mark.Target.Kind, mark.Target.Name, ThingSourceAuraDefinition, "", &c, mark.Value)
			}
			clearAuraDefinition(c.ID, mark.Target)
			moved++
		}
	}
	if moved > 0 || dropped > 0 {
		log.Printf("[thing] moved %d aura definitions into the ledger, dropped %d URL-named ones\n", moved, dropped)
	}
}

// clearAuraDefinition removes one mark from a character: an empty value is how
// SetCharacterAuraDefault spells deletion.
func clearAuraDefinition(characterID string, target mywant.AuraTarget) {
	mywant.SetCharacterAuraDefault(characterID, mywant.AuraMark{Target: target})
}

// ── identity migration (schema 1 → 2) ────────────────────────────────────────

// migrateThingStore gives every thing a UUID and moves the references onto it.
//
// Until now a thing WAS its catalog and its value: the id was the two joined,
// and the labels that say where a thing sits on the board and which group it
// belongs to were keyed by that. So changing a thing's category did not edit
// it — it destroyed one thing and created another, and left every reference
// pointing at an id nothing answered to. On the board the tile just vanished.
//
// Both halves have to happen together. A store rewritten without the labels
// remapped is exactly the failure this exists to prevent, so the labels are
// moved first: a label under an id whose thing has not been written yet is
// harmless, while a thing whose labels never followed is the bug.
func migrateThingStore(store *ThingStore, labels *ThingLabelStore) {
	if store == nil {
		return
	}
	raw, err := os.ReadFile(store.path)
	if os.IsNotExist(err) {
		return // nothing stored yet; the first write uses the new schema
	}
	if err != nil {
		log.Printf("[SERVER] thing store not readable (%v) — leaving it alone", err)
		return
	}
	var probe thingFile
	if err := yaml.Unmarshal(raw, &probe); err == nil && probe.Version >= thingSchemaVersion {
		return // already migrated
	}

	var legacy thingData
	if err := yaml.Unmarshal(raw, &legacy); err != nil {
		log.Printf("[SERVER] thing store unreadable as either schema (%v) — leaving it alone", err)
		return
	}
	if len(legacy) == 0 {
		return
	}

	entries := entriesFromCatalogs(legacy)
	mapping := make(map[string]string, len(entries))
	for _, e := range entries {
		mapping[e.Catalog+"::"+e.Value] = e.ID
	}

	if labels != nil {
		if err := labels.Rekey(mapping); err != nil {
			log.Printf("[SERVER] thing labels could not be re-keyed (%v) — thing store left on the old schema", err)
			return // do NOT rewrite the store; the two must move together
		}
	}
	if err := store.saveEntries(entries); err != nil {
		log.Printf("[SERVER] thing store could not be rewritten (%v)", err)
		return
	}
	log.Printf("[SERVER] thing store migrated to schema %d — %d things given stable ids", thingSchemaVersion, len(entries))
}
