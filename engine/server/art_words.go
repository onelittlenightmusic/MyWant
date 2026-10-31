package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	types "mywant/engine/types"
)

// The words a picture is drawn from — 作る in the GUI's ArtMaker (lib/art).
//
// Image Playground reads English best, and "東京タワー" drawn well is a red
// lattice tower, not the words; so before anything is drawn a model says, in
// a few English words, what the picture should show. What it is asked, and
// what becomes of its answer, are here, once. The model is wherever there is
// one:
//
//   - this server's own (fmtool, on a Mac): it answers here, and the page has
//     the words (prompt) with nothing more to do;
//   - otherwise (Fly: Linux) the page is handed the request (ask, warm) and has
//     whatever frames it — the iPhone app, Chrome on a Mac through the
//     extension's fmtool — run it on its own model (their `generate`), then
//     puts the answer together as it is told (suffix, limit), or falls back to
//     the value itself (fallback).
//
// So the iPhone and the Mac carry no knowledge of what a picture's words should
// be, only a model to ask: the same picture is asked for the same way
// wherever 作る is pressed, and changing how is a change here.
//
//	POST /api/v1/art/describe  {value, subtype, instead?, context?}
//	  → {ask, warm, suffix, limit, fallback, prompt?}

// artIconStyle is said after every description: a picture laid on the floor
// under a ball is small, and reads as an icon or not at all.
const artIconStyle = "simple flat icon, plain background"

// artWordsLimit is how much of the model's description is kept, in characters
// — 12 words asked for, and a model running on past them is cut here.
const artWordsLimit = 140

// artContextLimit is how much of what a want holds the request carries.
const artContextLimit = 400

type artWordsRequest struct {
	Value   string `json:"value"`
	Subtype string `json:"subtype"`
	// Instead: a description already offered, for another one. It is not
	// shown to the model — shown the last one, the model kept close to it, and
	// "another" came out as the same words shuffled — the model is let range
	// wider instead (warm).
	Instead string `json:"instead,omitempty"`
	// Context: a little of what it holds (a note's text, say), for the
	// picture to be about that and not only its name.
	Context string `json:"context,omitempty"`
}

type artWordsRecipe struct {
	Ask      string `json:"ask"`
	Warm     bool   `json:"warm"`
	Suffix   string `json:"suffix"`
	Limit    int    `json:"limit"`
	Fallback string `json:"fallback"`
	// Prompt: the words, already — this server had a model to ask.
	Prompt string `json:"prompt,omitempty"`
}

// artWordsRecipeFor is what a model is asked for value's picture, and how its
// answer is made into the words it is drawn from.
func artWordsRecipeFor(req artWordsRequest) artWordsRecipe {
	what := req.Subtype
	if what == "" {
		what = "thing"
	}
	ask := fmt.Sprintf("Describe in English, in at most 12 words, what a small icon-like picture of "+
		"this %s should show: %q. If it is a known landmark, station or place, name it and its most "+
		"recognisable look. Answer with the description only.", what, req.Value)
	if c := strings.TrimSpace(req.Context); c != "" {
		if r := []rune(c); len(r) > artContextLimit {
			c = string(r[:artContextLimit])
		}
		ask += fmt.Sprintf("\nWhat it holds, to draw from (it may be in Japanese): %q", c)
	}
	return artWordsRecipe{
		Ask:      ask,
		Warm:     strings.TrimSpace(req.Instead) != "",
		Suffix:   ", " + artIconStyle,
		Limit:    artWordsLimit,
		Fallback: req.Value + ", " + artIconStyle,
	}
}

// compose makes a model's answer into the words a picture is drawn from: cut
// to the limit, the icon style after it — or the fallback, for no answer.
func (r artWordsRecipe) compose(said string) string {
	said = strings.TrimSpace(said)
	if said == "" {
		return r.Fallback
	}
	if rs := []rune(said); len(rs) > r.Limit {
		said = string(rs[:r.Limit])
	}
	return said + r.Suffix
}

// artAskModel is this server's own model — a variable so tests need none.
var artAskModel = types.AskPlain

func (s *Server) describeArt(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	var req artWordsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "bad request", err.Error())
		return
	}
	req.Value = strings.TrimSpace(req.Value)
	if req.Value == "" {
		s.JSONError(w, r, http.StatusBadRequest, "value is required", "")
		return
	}
	recipe := artWordsRecipeFor(req)
	if said, ok := artAskModel(recipe.Ask, recipe.Warm); ok && strings.TrimSpace(said) != "" {
		recipe.Prompt = recipe.compose(said)
	}
	s.JSONResponse(w, http.StatusOK, recipe)
}
