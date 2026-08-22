package server

import (
	"log"

	mywant "mywant/engine/core"
)

// Every character gets a motion want, and never more than one.
//
// This is what replaced the old shared ticker (see drive_engine.go):
// movement is now something each character's own want does for itself, on
// its own schedule, calling back into driveOneCharacterTick — not something
// a goroutine outside the engine does to every character at once. A
// character with nothing to move it can still be posed by hand, but nothing
// resolves going/gear/direction votes for them until this exists.
//
// Named after the character it belongs to, matching characterChatWantType's
// own naming: motion-<characterId>.
const characterMotionWantType = "character_motion"

func characterMotionWantName(characterID string) string {
	return "motion-" + characterID
}

// ensureCharacterMotionWants creates the missing motion wants for every
// character that has none.
//
// Idempotent and cheap, same as ensureCharacterChatWants: looks for the want
// by name first, so a character that already has one costs a map lookup.
// Called on the way up and whenever a character is created, which between
// them covers characters that predate this and characters that arrive after
// it.
func (s *Server) ensureCharacterMotionWants() {
	for _, c := range mywant.ListCharacters() {
		s.ensureCharacterMotionWant(c.ID)
	}
}

func (s *Server) ensureCharacterMotionWant(characterID string) {
	name := characterMotionWantName(characterID)
	if existing := s.findWantByIDOrName(name); existing != nil {
		// Re-assert the mark. A want that predates this being a system want
		// is still on disk without it, and would be swept away by the next
		// world switch exactly as before (see clearNonSystemWants).
		existing.Metadata.IsSystemWant = true
		return
	}

	want := &mywant.Want{
		Metadata: mywant.Metadata{
			ID:   name,
			Name: name,
			Type: characterMotionWantType,
			// A character's legs belong to the person, not to the board —
			// same reasoning as the chat want. Switching worlds tears down
			// every want that is not a system want, which is right for the
			// things a world is made of and wrong for this: the person is
			// the same person in every world, and without the mark they
			// would stop being able to move the first time anybody
			// travelled.
			IsSystemWant: true,
			Labels: map[string]string{
				canvasLabelNear: characterID,
			},
		},
		Spec: mywant.WantSpec{
			Params: map[string]any{"character_id": characterID},
		},
	}

	if _, err := s.globalBuilder.AddWantsAsyncWithTracking([]*mywant.Want{want}); err != nil {
		log.Printf("[CharacterMotion] could not create motion want for %s: %v", characterID, err)
		return
	}
	log.Printf("[CharacterMotion] created %s", name)
}
