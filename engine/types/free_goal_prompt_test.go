package types

import (
	"strings"
	"testing"

	. "mywant/engine/core"
)

// What the on-device model is actually asked.
//
// The prompts are built from pieces — the CLI's own command list, the steps so
// far, a handful of rules — and what they add up to is not obvious from any one
// of them. This renders them, so that reading the test output answers "what are
// we sending?" without anybody having to assemble it in their head.
//
// It also holds two rules that were paid for in wrong answers:
//
//   - no example sentence in quotes. Given one, this model says it: a prompt
//     containing "it is not there" produced that as the answer to a question
//     about constellations, to a request to move a tile, and to a want whose
//     value had already been read.
//   - the first turn cannot answer, only run. Asked "荻窪はどの星座？" with
//     nothing read, the model invents a constellation.
func TestFreeGoalPrompts(t *testing.T) {
	catalogue := map[string]freeGoalCommand{
		"board":     {Path: "board", Use: "board", Short: "Everything standing on the canvas", Risk: "read"},
		"thing get": {Path: "thing get", Use: "get <catalog|subtype>", Short: "Show the values recorded under one catalog key", Risk: "read"},
		"thing pin": {Path: "thing pin", Use: "pin <thing> <x> <y>", Short: "Put a thing on the canvas at a cell", Risk: "change"},
		"wants get": {Path: "wants get", Use: "get [name-or-id]", Short: "Show one want: the answer it holds", Risk: "read"},
		"wants create": {Path: "wants create", Use: "create", Short: "Create a new want", Risk: "change", Flags: []freeGoalFlag{
			{Name: "type", Type: "string"}, {Name: "param", Type: "stringArray"}, {Name: "at", Type: "string"}, {Name: "name", Type: "string"},
		}},
	}
	want := &Want{}

	first := freeGoalPrompt(want, "荻窪のWeatherを知りたい", catalogue, nil)
	t.Log("\n─── first turn ───\n" + first)

	later := freeGoalPrompt(want, "荻窪のWeatherを知りたい", catalogue, []freeGoalStep{{
		Command: "wants get", Args: "荻窪のWeather", Failed: true,
		Output: `Error: want not found: "荻窪のWeather"`,
	}})
	t.Log("\n─── a later turn ───\n" + later)

	if !strings.Contains(first, "RUN <command path>") {
		t.Error("the first turn does not say how to answer")
	}
	if strings.Contains(first, "ANSWER") {
		t.Error("the first turn offers an answer before anything has been read")
	}
	if !strings.Contains(later, "ANSWER") {
		t.Error("a later turn never offers to finish")
	}
	// A quoted sentence is a sentence this model will repeat back as its
	// answer. Rules may be quoted; answers may not.
	for _, phrase := range []string{`"it is not there"`, `'it is not there'`, `"not found"`} {
		if strings.Contains(first, phrase) || strings.Contains(later, phrase) {
			t.Errorf("prompt contains an example answer in quotes: %s", phrase)
		}
	}
}
