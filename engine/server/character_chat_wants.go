package server

import (
	"log"

	mywant "mywant/engine/core"
)

// Every character gets a chat want, and never more than one.
//
// The robot has had one since the beginning — the "robot" want is its mouth and
// its chat window at once — and the arrangement is not special to the robot. A
// character with nobody able to send it a message is a character that can only
// be moved, so the want is made with the character rather than arranged for
// afterwards: a character always has a mouth, and nobody has to know that is
// what the want is for.
//
// Named after the character it belongs to, so the webhook URL reads as the
// thing it does: POST /api/v1/webhooks/chat-<characterId>.
const characterChatWantType = "character_chat"

// The one character whose want is not one of these — its coding agent is the
// whole difference. Matches design/robotVoice.ts on the frontend.
const robotCharacterID = "robot"

func characterChatWantName(characterID string) string {
	return "chat-" + characterID
}

// ensureCharacterChatWants creates the missing chat wants for every character
// that has none, and binds each character to its own.
//
// Idempotent and cheap: it looks for the want by name, and a character that
// already has one costs a map lookup. Called on the way up and whenever a
// character is created, which between them covers characters that predate this
// and characters that arrive after it.
func (s *Server) ensureCharacterChatWants() {
	for _, c := range mywant.ListCharacters() {
		// The robot is the one character whose want is not one of these: it has
		// a coding agent inside it, which is the whole difference between them.
		if c.ID == robotCharacterID {
			continue
		}
		s.ensureCharacterChatWant(c.ID)
	}
}

func (s *Server) ensureCharacterChatWant(characterID string) {
	name := characterChatWantName(characterID)
	if existing := s.findWantByIDOrName(name); existing != nil {
		// Re-assert the mark as well as the binding. A want that predates this
		// being a system want is still on disk without it, and would be swept
		// away by the next world switch exactly as before.
		existing.Metadata.IsSystemWant = true
		bindCharacterToChatWant(characterID, existing.Metadata.ID)
		return
	}

	want := &mywant.Want{
		Metadata: mywant.Metadata{
			ID:   name,
			Name: name,
			Type: characterChatWantType,
			// A character's mouth belongs to the person, not to the board.
			//
			// Switching worlds tears down every want that is not a system want
			// (see clearNonSystemWants), which is right for the things a world
			// is made of and wrong for this: the person is the same person in
			// every world, and without the mark their chat want was swept away
			// the first time anybody travelled — after which they could not say
			// anything, with nothing on screen to explain why.
			//
			// The same reasoning the robot's own want is held under, and it
			// survives startup for a different reason: the startup filter only
			// reconsiders types listed in system_wants.yaml, and these are not
			// there. They cannot be — there is one per character, and who the
			// characters are is not known until the store is read.
			IsSystemWant: true,
			Labels: map[string]string{
				// Off the board. The character is already on it as a cursor;
				// this want is the window you talk to them through, and drawing
				// it as well would put the same person in two places.
				"mywant.io/canvas-hidden": "true",
			},
		},
		Spec: mywant.WantSpec{
			Params: map[string]any{"character_id": characterID},
		},
	}

	if _, err := s.globalBuilder.AddWantsAsyncWithTracking([]*mywant.Want{want}); err != nil {
		log.Printf("[CharacterChat] could not create chat want for %s: %v", characterID, err)
		return
	}
	bindCharacterToChatWant(characterID, name)
	log.Printf("[CharacterChat] created %s", name)
}

// bindCharacterToChatWant points a character at its chat want, unless the
// character already points somewhere. A person who has starred a want as their
// aura card said where they wanted to point, and this is not more important
// than that.
func bindCharacterToChatWant(characterID, wantID string) {
	c, ok := mywant.GetCharacter(characterID)
	if !ok || c.AuraCardWantID != "" {
		return
	}
	mywant.SetCharacterAuraCardWant(characterID, wantID)
}

// removeCharacterChatWant tears down the chat want of a character who no longer
// exists.
//
// Deleted through the builder rather than the API path, which refuses on
// purpose: these are protected system wants so that nobody loses their voice by
// accident, and "the person is gone" is the one case where removing it is
// right.
func (s *Server) removeCharacterChatWant(characterID string) {
	name := characterChatWantName(characterID)
	want := s.findWantByIDOrName(name)
	if want == nil {
		return
	}
	if err := s.globalBuilder.QueueWantDelete([]string{want.Metadata.ID}); err != nil {
		log.Printf("[CharacterChat] could not remove %s: %v", name, err)
		return
	}
	log.Printf("[CharacterChat] removed %s", name)
}
