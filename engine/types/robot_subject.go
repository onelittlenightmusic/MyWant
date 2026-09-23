package types

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	. "mywant/engine/core"
)

// What a request to the robot is about, decided before anybody answers it.
//
// The robot used to find out what it had been asked about from whatever the
// model did: if the command it chose happened to read a picture, the answer
// was taken from the picture. The model is a poor judge of that. Asked for the
// score in a photo, the on-device model ran `point` and answered with where the
// photo stands; asked the same thing again, it ran nothing at all and answered
// from memory. Whether a question is about a photo is not a matter of taste, so
// it is settled here, first, from what was said and where it was said:
//
//  1. a want named in the question — "picture-instanceの写真のスコアは？";
//  2. failing that, a pointing word — これ, この写真, this — and the tile the
//     person is standing on or beside, from the position line the server
//     appends to what they said (contextForSpeaker in speech_context.go).
//
// Only wants that are answered in their own way are looked for — a picture
// today. For everything else the provider answers as it always has.
//
// What is common to every provider lives here: finding the subject, putting
// the photo and its words where the answerer can see them, and keeping the
// answer on the picture. How the photo is shown differs — Claude is told the
// file and opens it, the on-device model is handed it (agent_fm.go) — and
// nothing else does.

// subjectTypes are the want types a request is checked against.
var subjectTypes = map[string]bool{"picture": true}

// robotContextMarker opens the position line appended to what was said.
const robotContextMarker = "\n\n(context:"

// splitRobotRequest separates what the person said from the position line the
// server appended to it.
func splitRobotRequest(request string) (question, context string) {
	if i := strings.Index(request, robotContextMarker); i >= 0 {
		return strings.TrimSpace(request[:i]), request[i:]
	}
	return strings.TrimSpace(request), ""
}

// pointingWords are how a person says "the one I am at".
var pointingWords = []string{"これ", "この", "それ", "その", "ここ", "こちら", "this", "that", "here"}

// resolveRobotSubject is the want a request is about, or nil.
func resolveRobotSubject(request string, wants []*Want) *Want {
	question, context := splitRobotRequest(request)
	var candidates []*Want
	for _, w := range wants {
		if w != nil && subjectTypes[w.Metadata.Type] && w.Metadata.Name != "" {
			candidates = append(candidates, w)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	// Named. The longest name wins, so "photo-2" is not taken for "photo".
	lowerQ := strings.ToLower(question)
	var named *Want
	for _, w := range candidates {
		if strings.Contains(lowerQ, strings.ToLower(w.Metadata.Name)) &&
			(named == nil || len(w.Metadata.Name) > len(named.Metadata.Name)) {
			named = w
		}
	}
	if named != nil {
		return named
	}

	// Pointed at. Only with a pointing word: standing beside a photo and
	// asking where 新宿 is is not a question about the photo.
	if context == "" || !containsAny(lowerQ, pointingWords...) {
		return nil
	}
	onCell, besideCell := contextTiles(context)
	if w := onlyOne(candidates, onCell); w != nil {
		return w
	}
	return onlyOne(candidates, besideCell)
}

// contextTiles reads the two lists back out of the position line:
// "on that cell: A (want picture), B (thing station); next to them: …".
func contextTiles(context string) (onCell, beside string) {
	for _, part := range strings.Split(context, "; ") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "on that cell: "):
			onCell = strings.TrimPrefix(part, "on that cell: ")
		case strings.HasPrefix(part, "next to them: "):
			beside = strings.TrimPrefix(part, "next to them: ")
		}
	}
	return onCell, beside
}

// onlyOne is the single candidate the list names, or nil when it names none or
// several — two photos underfoot and a "これ" is a question to ask back, not a
// guess to make.
func onlyOne(candidates []*Want, list string) *Want {
	var found *Want
	for _, w := range candidates {
		if strings.Contains(list, w.Metadata.Name+" (want "+w.Metadata.Type+")") {
			if found != nil {
				return nil
			}
			found = w
		}
	}
	return found
}

// robotPicture is a photo a request is about, ready to be shown to whoever
// answers: saved to a file, with the words already read out of it.
type robotPicture struct {
	Want     *Want
	Question string // what the person said, without the position line
	Image    string // a temporary file; remove with close
	Lines    []string
}

func (p *robotPicture) close() {
	if p != nil && p.Image != "" {
		os.Remove(p.Image)
	}
}

// prepareRobotPicture finds the picture a request is about and gets it ready,
// or returns nil when the request is not about one. A picture that cannot be
// shown — no image yet, a download that failed — is logged and treated as no
// subject, so the question is still answered, the ordinary way.
func prepareRobotPicture(ctx context.Context, robot *Want, request string) *robotPicture {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil
	}
	var wants []*Want
	for _, w := range cb.GetAllWantStates() {
		wants = append(wants, w)
	}
	subject := resolveRobotSubject(request, wants)
	if subject == nil {
		return nil
	}
	imageURL := GetCurrent(subject, "image_url", "")
	if imageURL == "" {
		robot.StoreLog("[ROBOT] %s has no image yet; answering the ordinary way", subject.Metadata.Name)
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	file, err := downloadPicture(ctx, imageURL)
	if err != nil {
		robot.StoreLog("[ROBOT] could not fetch %s's photo: %v", subject.Metadata.Name, err)
		return nil
	}
	var lines []string
	if text := GetCurrent(subject, "text", ""); text != "" {
		lines = strings.Split(text, "\n")
	}
	question, _ := splitRobotRequest(request)
	robot.StoreLog("[ROBOT] the question is about picture %s", subject.Metadata.Name)
	RecordCCActivity(robot, CCActivityNote, "写真 "+subject.Metadata.Name+" について")
	return &robotPicture{Want: subject, Question: question, Image: file, Lines: lines}
}

// claudePicturePrompt is the request as Claude receives it when it is about a
// photo: what was said, and where the photo and its words are. Claude opens
// the file itself; the words are there because the recogniser copies small
// print exactly, which is worth having beside even a good eye.
func claudePicturePrompt(request string, p *robotPicture) string {
	var b strings.Builder
	b.WriteString(request)
	fmt.Fprintf(&b, "\n\n(This is about the picture want %q. Its photo is saved at %s — look at it with the Read tool before answering.", p.Want.Metadata.Name, p.Image)
	if len(p.Lines) > 0 {
		b.WriteString(" The text read from it by OCR, one line per row, cells separated by \" | \" — exact for the words it has, but it misses some, such as circled numbers:\n")
		b.WriteString(strings.Join(p.Lines, "\n"))
	}
	b.WriteString("\nAnswer the question from the photo, in the language it was asked in.)")
	return b.String()
}

// recordRobotAnswer puts one answer where every provider's answers go — the
// chat's ring buffer, and the robot's own mouth — and, when it was about a
// picture, onto the picture as well, with the chat entry saying which picture
// and which of its answers it is so the chat can offer 👍 / 違う on it.
func recordRobotAnswer(want *Want, answer, subtype string, about *robotPicture) {
	entry := map[string]any{
		"text":      answer,
		"timestamp": time.Now().Format(time.RFC3339),
		"subtype":   subtype,
	}
	if about != nil {
		id := fmt.Sprintf("ans-%d", time.Now().UnixNano())
		StoreStateMulti(about.Want, map[string]any{
			"webhook_payload": map[string]any{
				"action":   "record_answer",
				"id":       id,
				"question": about.Question,
				"answer":   answer,
				"by":       want.Metadata.Name,
			},
			"webhook_received_at": time.Now().Format(time.RFC3339Nano),
		})
		want.StoreLog("[ROBOT] answer %s kept on picture %s", id, about.Want.Metadata.Name)
		entry["picture_id"] = about.Want.Metadata.ID
		entry["picture_name"] = about.Want.Metadata.Name
		entry["answer_id"] = id
	}
	responses := GetCurrent(want, "cc_responses", []any{})
	responses = append(responses, entry)
	if len(responses) > 20 {
		responses = responses[len(responses)-20:]
	}
	want.SetCurrent("cc_responses", responses)
	// The robot answering is the robot speaking. Only the robot: a `coding`
	// want is somebody's agent on the board, not a character, and has no mouth.
	if want.Metadata.Type == "robot" {
		CharacterSpeaks("robot", answer, "agent")
	}
}
