package mywant

// OnCharacterSpeak, when set by the server, records that a character said
// something into the conversation every client reads (GET /api/v1/speech).
//
// The seam exists for the same reason OnCharacterEffectFire does: a want knows
// what was said and nothing about how the world keeps a record, and the types
// package cannot depend on the server package to find out. The server registers
// it on the way up; want code calls CharacterSpeaks and does not care whether
// anybody is listening.
var OnCharacterSpeak func(characterID, text, source string)

// CharacterSpeaks records an utterance if a handler is registered; a no-op
// otherwise (in tests, or before server startup).
//
// source says how it was said — a cursor message, a chat want, an agent's
// answer. They are different acts that belong in one column, and the column is
// the only thing that has to know they are different.
func CharacterSpeaks(characterID, text, source string) {
	if OnCharacterSpeak != nil && characterID != "" && text != "" {
		OnCharacterSpeak(characterID, text, source)
	}
}
