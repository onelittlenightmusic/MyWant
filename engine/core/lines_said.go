package mywant

import "strings"

// lines_said.go — what a want is about to say, heard as it is decided.
//
// Every line a want says is written in Japanese. A person who reads the board
// in another language (CharacterDisplay.Language) reads its translation, and
// the translation is made ahead, not when they stand on the tile: a model
// asked then would keep them waiting, and a phone or a Mac may not be there
// to ask. So the engine says each line here as it is decided — a voice's
// lines (WantVoice, WantNewsVoice), a `say` written into a want's state — and
// whoever listens (the server's translator, server/translate.go) gathers them
// and has them translated in batches, offline.

// OnLinesSaid, when set, hears the lines: as they will be shown (spaced
// between sentences, trimmed). Run on a goroutine of its own, so a slow
// listener holds nothing up. Set once, by the server, before any want runs.
var OnLinesSaid func(lines []string)

// linesSaid hands lines to OnLinesSaid, if anyone listens.
func linesSaid(lines []string) {
	f := OnLinesSaid
	if f == nil {
		return
	}
	var out []string
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	if len(out) > 0 {
		go f(out)
	}
}

// voiceLinesSaid says a voice's lines as pickVoice will show them.
func voiceLinesSaid(lines []string) {
	if OnLinesSaid == nil || len(lines) == 0 {
		return
	}
	spaced := make([]string, len(lines))
	for i, l := range lines {
		spaced[i] = SpaceSentences(l)
	}
	linesSaid(spaced)
}
