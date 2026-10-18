package types

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// A photo's words read into fields, by the rules of the want it is joined to.
//
// A photo is read once when it is saved (its words — picture-text). What the
// words MEAN depends on what the photo is of, which is known only once it is
// joined to something: a score card joined to a SmartGolf reservation has a
// score, a course and when it was played. The want's type says which fields
// its photos have and how each is read (PhotoField, from the type's
// photo-fields label); a model reads the words by those rules once, when the
// photo is joined, and the values are kept on the photo as fields. A question
// is then answered from the fields, with no model at all — the same answer
// every time, on a server with no model too.

// PhotoField is one field a type's photos are read into.
type PhotoField struct {
	// The field's name, e.g. "score"; kept on the photo as photo-<name>.
	Name string
	// The words a question asks for it by — スコア, 打数, score.
	Words []string
	// How it is read from the words, for the model.
	Rule string
}

// ParsePhotoFields reads a type's photo-fields label: one field a line,
// "name | word, word | rule". Lines that do not have all three are skipped.
func ParsePhotoFields(spec string) []PhotoField {
	var out []PhotoField
	for _, line := range strings.Split(spec, "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		f := PhotoField{Name: strings.TrimSpace(parts[0]), Rule: strings.Join(strings.Fields(parts[2]), " ")}
		for _, w := range strings.Split(parts[1], ",") {
			if w = strings.TrimSpace(w); w != "" {
				f.Words = append(f.Words, w)
			}
		}
		if f.Name != "" && f.Rule != "" {
			out = append(out, f)
		}
	}
	return out
}

// PhotoFieldsPrompt is what the model is asked to read a photo's words into
// fields by: the words, what the photo is (the type's photo-reading), and each
// field's rule — answered one field a line, "name: value". The same prompt the
// iPhone app asks its own model (the server hands it over), so a photo read on
// the phone is read the same way.
func PhotoFieldsPrompt(about string, fields []PhotoField, lines []string) string {
	var b strings.Builder
	b.WriteString("Text read from a photo, line by line; cells on a line are separated by |:\n")
	b.WriteString(strings.Join(lines, "\n"))
	if about != "" {
		b.WriteString("\n\nWhat the photo is: " + about)
	}
	b.WriteString("\n\nRead these fields from the text:\n")
	for _, f := range fields {
		fmt.Fprintf(&b, "- %s: %s\n", f.Name, f.Rule)
	}
	b.WriteString("\nAnswer with one line per field, as \"name: value\", the value copied exactly as it is written in the text — nothing added, nothing else. When the text does not have a field, write \"name: -\".")
	return b.String()
}

// ParsePhotoFieldsAnswer reads the model's answer back into values, keeping
// only a value that is in the words — what was read, not what the model made
// up.
func ParsePhotoFieldsAnswer(answer string, fields []PhotoField, lines []string) map[string]string {
	text := strings.Join(lines, "\n")
	known := map[string]bool{}
	for _, f := range fields {
		known[f.Name] = true
	}
	out := map[string]string{}
	for _, line := range strings.Split(answer, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*• "))
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.Trim(strings.TrimSpace(name), "`*\"")
		value = strings.Trim(strings.TrimSpace(value), "「」\"'`")
		if !known[name] || value == "" || value == "-" {
			continue
		}
		// In the words — full-width and half-width the same: the recogniser
		// writes 79（＋7） where the model, copying it, writes 79（+7）. Kept as
		// the words have it.
		i := strings.Index(foldWidth(text), foldWidth(value))
		if i < 0 {
			continue
		}
		if orig, ok := unfoldAt(text, i, len([]rune(value))); ok {
			value = orig
		}
		out[name] = value
	}
	return out
}

// InterpretPhoto reads a photo's words into its fields with this machine's
// model. ok=false when there is no model here (Fly) — the iPhone app reads it
// then — or it could not be asked.
func InterpretPhoto(ctx context.Context, about string, fields []PhotoField, lines []string) (map[string]string, bool) {
	binary, ok := fmToolPath()
	if !ok || len(fields) == 0 || len(lines) == 0 {
		return nil, false
	}
	root, _ := os.UserHomeDir()
	r, err := fmServerFor(binary).askPlain(PhotoFieldsPrompt(about, fields, lines), root, fmDefaultTimeoutSeconds*time.Second)
	if err != nil {
		return nil, false
	}
	return ParsePhotoFieldsAnswer(r.Text, fields, lines), true
}

// PhotoFieldAsked is the field a question asks for — the one whose words it
// uses, the longest word winning — or "" when it names none.
func PhotoFieldAsked(question string, fields []PhotoField) string {
	q := strings.ToLower(question)
	best, name := 0, ""
	for _, f := range fields {
		for _, w := range append([]string{f.Name}, f.Words...) {
			if l := len([]rune(w)); l > best && strings.Contains(q, strings.ToLower(w)) {
				best, name = l, f.Name
			}
		}
	}
	return name
}

// photoFieldAnswer answers a question from a photo's fields, when it asks for
// one the photo has.
func photoFieldAnswer(question string, p AnswerPhoto) (string, bool) {
	name := PhotoFieldAsked(question, p.FieldSpec)
	if name == "" {
		return "", false
	}
	v := p.Fields[name]
	return v, v != ""
}

// AnswerFromPhotoFields is the answer to a question from the first photo
// whose fields have it, newest answer first, saying which answer's photo.
func AnswerFromPhotoFields(question string, photos []AnswerPhoto) (string, bool) {
	v, _, ok := AnswerFromPhotoFieldsOf(question, photos)
	return v, ok
}

// AnswerFromPhotoFieldsOf is AnswerFromPhotoFields and the photo (its thing
// id) the answer was read off — the evidence, which the robot goes to.
func AnswerFromPhotoFieldsOf(question string, photos []AnswerPhoto) (string, string, bool) {
	for _, p := range photos {
		if v, ok := photoFieldAnswer(question, p); ok {
			if p.About != "" {
				v = fmt.Sprintf("%s（%s の写真より）", v, p.About)
			}
			return v, p.ID, true
		}
	}
	return "", "", false
}

// AnswerEvidence, when set, is told what an answer was read off — a photo
// thing, a want, one of a want's answers (`<want id>/result/<key>`) — so the
// robot goes and stands beside it (the server's answer_evidence.go).
var AnswerEvidence func(id string)

// showAnswerEvidence tells AnswerEvidence, when there is one to tell.
func showAnswerEvidence(id string) {
	if AnswerEvidence != nil && id != "" {
		AnswerEvidence(id)
	}
}

// foldWidth writes full-width ASCII (！ to ～, and the ideographic space) as
// its half-width form, one rune for one, so positions line up.
func foldWidth(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			return r - 0xFEE0
		case r == 0x3000:
			return ' '
		}
		return r
	}, s)
}

// unfoldAt is the n runes of s at the byte offset i of foldWidth(s) — the
// original of what was found there.
func unfoldAt(s string, i, n int) (string, bool) {
	folded := []rune(foldWidth(s))
	orig := []rune(s)
	at := len([]rune(string([]byte(foldWidth(s))[:i])))
	if at+n > len(orig) || len(folded) != len(orig) {
		return "", false
	}
	return string(orig[at : at+n]), true
}
