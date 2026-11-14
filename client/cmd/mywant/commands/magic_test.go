package commands

import (
	"strings"
	"testing"
)

func TestFoldReading(t *testing.T) {
	for _, c := range []struct{ reading, typed string }{
		{"ogikubo", "ogi"}, {"toukyou", "tokyo"}, {"ootemachieki", "otemachi"}, {"yuurakuchou", "yurakucho"},
		{"kichijouji", "kichijoji"}, {"shinbashi", "shimbashi"}, {"nakanosakaue", "nakanosaka"},
	} {
		if !strings.HasPrefix(foldReading(c.reading), foldReading(c.typed)) {
			t.Errorf("%q does not offer %q (%q, %q)", c.typed, c.reading, foldReading(c.typed), foldReading(c.reading))
		}
	}
	if strings.HasPrefix(foldReading("shinjuku"), foldReading("shibuya")) {
		t.Error("shibuya offers shinjuku")
	}
}

func TestPatchedScripts(t *testing.T) {
	if !strings.Contains(patchZsh(`        if eval _describe $keepOrder "completions" completions $flagPrefix $noSpace; then`), `$noSpace $asChosen`) {
		t.Error("zsh script not patched")
	}
	if !strings.Contains(patchBash("__mywant_handle_standard_completion_case() {\n    local tab=$'\\t'\n"), "(directive & 1024)") {
		t.Error("bash script not patched")
	}
}
