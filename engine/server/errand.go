package server

// errand.go — sent to look again, a robot comes back and says how it went.
//
// Asked 「戸塚への乗り換えを再検索して」, a robot (the robot, or a character
// acting as one — robot_body.go) answers at once — it has run the route again
// — and the route answers a moment later. What it found was said to nobody:
// the answer had gone before the want had anything to say. And the robot
// had been there before. What it heard then is in its memory
// (character_memory.go), and that is what it expected to find.
//
// So an errand is three things in the robot's story:
//
//   - errand.expect: when it runs a want again, what it remembers that want
//     saying — what it expects, or nothing when it has never heard it;
//   - errand.find: once the want has answered, what it says now, and whether
//     that is the same as expected, different, or cannot be told;
//   - errand.report: what it then says, in its chat and its bubble, to whoever
//     sent it — 「やっぱり違いました」 when it is not what it expected.
//
// Same or different is not a matter of the words: a route searched at
// midnight and again at eight differs in every number and may be just as
// expected. Apple's model judges it, given what was asked and when each was
// heard, and words the report — from these records only, so the robot says
// nothing it did not hear. With no model to ask, the report says what was
// heard before and now, side by side, and leaves the judging to the person.

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	mywant "mywant/engine/core"

	ws "github.com/onelittlenightmusic/want-spec"
)

const (
	storyKindErrandExpect = "errand.expect"
	storyKindErrandFind   = "errand.find"
	storyKindErrandReport = "errand.report"

	// errandAnswerWait: how long a want run again is waited for.
	errandAnswerWait = 90 * time.Second
	// errandSettle: a want just restarted still shows its last status for a
	// moment; nothing it says before this is its new answer.
	errandSettle = 2 * time.Second
	// errandModelWait: how long the model is given to judge and word it.
	errandModelWait = 30 * time.Second
)

// The verdicts of an errand.find.
const (
	errandSame      = "same"
	errandDifferent = "different"
	errandUnknown   = "unknown"
	errandFirst     = "first" // nothing expected: never heard it before
)

// errand is one going to a want to see.
type errand struct {
	body     robotBody
	wantID   string
	question string // what was asked, as said
	asker    string // who sent it (a character id), when known
	expected *Memory
	at       time.Time
}

// robotRunsAgain: the tool's run had the want answer again — an errand.
func robotRunsAgain(name string, args map[string]string, out string) bool {
	switch name {
	case "deploy_want":
		return strings.HasPrefix(out, "Ran ") || strings.HasPrefix(out, "Brought ")
	case "use_capability":
		return args["capability"] == "restart"
	}
	return false
}

// startErrand sets an errand going once a tool has run a want again: what is
// expected written down now, and the want waited on for what it says.
func (s *Server) startErrand(b robotBody, wantID, question, asker string, expected *Memory) {
	e := errand{body: b, wantID: wantID, question: question, asker: asker, expected: expected, at: time.Now()}
	detail := map[string]string{"question": question}
	if asker != "" {
		detail["asker"] = asker
	}
	if expected != nil {
		detail["expected"] = expected.Say
		detail["heard_at"] = expected.At
	}
	story.add(ws.StoryEvent{Character: b.Character, Kind: storyKindErrandExpect, To: s.storyWant(wantID), Detail: detail})
	go s.finishErrand(e)
}

// rememberedOf is what a character remembers a want saying, if anything.
func rememberedOf(character, wantID string) *Memory {
	for _, m := range memories.get(character).Memories {
		if m.WantID == wantID {
			mm := m
			return &mm
		}
	}
	return nil
}

// finishErrand waits for the want's answer, compares it with what was
// expected, and reports.
func (s *Server) finishErrand(e errand) {
	w := s.waitForAnswer(e.wantID)
	if w == nil {
		return
	}
	say, mood := s.standingWords(w)
	if strings.TrimSpace(say) == "" {
		return
	}
	if mood == "" {
		mood = "calm"
	}
	// Heard, as any asking is (and kept, if it moves them).
	s.heardBy(e.body.Character, w, say, mood, 0, memories.now())

	verdict, why, report := s.judgeErrand(e, w, say)
	story.add(ws.StoryEvent{Character: e.body.Character, Kind: storyKindErrandFind, To: storyWantPlace(w),
		Detail: map[string]string{"found": say, "verdict": verdict, "why": why}})
	cards := s.robotAnswerCards(robotTurn{Steps: []robotStep{{Card: &robotCard{Kind: "want", ID: w.Metadata.ID, Name: w.Metadata.Name}}}})
	s.robotChatSayAs(e.body, report, cards, nil, "")
	if err := s.sayBody(e.body, report, speechNotice); err != nil {
		log.Printf("[errand] %s could not say it: %v", e.body.Character, err)
	}
	story.add(ws.StoryEvent{Character: e.body.Character, Kind: storyKindErrandReport, To: storyWantPlace(w),
		Detail: map[string]string{"said": report, "verdict": verdict, "asker": e.asker}})
	// Back: their timings read with how it went (character_timings.go) — the
	// standard one tells of an errand that found other than expected.
	s.characterMoment(e.body.Character, map[string]any{
		ws.CharacterStateErrandVerdict:   verdict,
		ws.CharacterStateErrandDifferent: verdict == errandDifferent,
		ws.CharacterStateErrandReport:    report,
	}, &characterAbout{wantID: w.Metadata.ID, wantName: w.Metadata.Name, wantType: w.Metadata.Type})
	log.Printf("[errand] %s at %s: %s — %s", e.body.Character, w.Metadata.Name, verdict, report)
}

// waitForAnswer waits until a want run again has finished, and is it then;
// nil when it never did.
func (s *Server) waitForAnswer(id string) *mywant.Want {
	time.Sleep(errandSettle)
	deadline := time.Now().Add(errandAnswerWait)
	for time.Now().Before(deadline) {
		w := s.findWantByIDOrName(id)
		if w == nil {
			return nil
		}
		if mywant.IsAchievedStatus(w.Status) || w.Status == mywant.WantStatusFailed {
			return w
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

// judgeErrand is the verdict, why, and what the robot reports.
func (s *Server) judgeErrand(e errand, w *mywant.Want, found string) (verdict, why, report string) {
	name := characterName(e.body.Character)
	to := ""
	if e.asker != "" && e.asker != e.body.Character {
		to = characterName(e.asker) + "さん、"
	}
	if e.expected == nil {
		return errandFirst, "", fmt.Sprintf("%s見てきました。%s：%s", to, w.Metadata.Name, found)
	}
	if strings.TrimSpace(e.expected.Say) == strings.TrimSpace(found) {
		return errandSame, "同じことば", fmt.Sprintf("%s見てきました。%sは前と同じでした：%s", to, w.Metadata.Name, found)
	}
	ctx, cancel := context.WithTimeout(context.Background(), errandModelWait)
	defer cancel()
	verdict = errandUnknown
	text, _, err := generateText(ctx, errandPrompt(e, name, w, found), false, errandModelWait)
	if err == nil {
		if v, y, ok := readErrandJudgement(text); ok {
			verdict, why = v, y
		} else {
			log.Printf("[errand] the model's judgement could not be read: %q", firstLine(text, 200))
		}
	} else {
		log.Printf("[errand] no model to judge it: %v", err)
	}
	return verdict, why, errandReport(verdict, to, w.Metadata.Name, e.expected.Say, found)
}

// errandReport is what the robot says. The model judges; the words heard are
// said as they were heard — a small model asked to say them in its own words
// said 「乗り換えが1回から2回に増えた」 of a route that went from two to one.
func errandReport(verdict, to, want, expected, found string) string {
	switch verdict {
	case errandDifferent:
		return fmt.Sprintf("%sやっぱり違いました。%sは、前は「%s」でしたが、今は「%s」です。", to, want, expected, found)
	case errandSame:
		return fmt.Sprintf("%s見てきました。%sは前と変わりません：%s", to, want, found)
	}
	return fmt.Sprintf("%s見てきました。%sは、前は「%s」、今は「%s」です。", to, want, expected, found)
}

// errandPrompt asks the model to judge an errand and word its report.
func errandPrompt(e errand, name string, w *mywant.Want, found string) string {
	heardAt := e.expected.At
	if t, err := time.Parse(time.RFC3339, e.expected.At); err == nil {
		heardAt = t.Format("1月2日 15:04")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "あなたは%sです。頼まれて「%s」を見に行き、戻ってきました。\n", name, w.Metadata.Name)
	if e.question != "" {
		fmt.Fprintf(&b, "頼まれたこと: %s\n", e.question)
	}
	fmt.Fprintf(&b, "前に聞いていたこと（%s）: %s\n", heardAt, e.expected.Say)
	fmt.Fprintf(&b, "いま聞いたこと（%s）: %s\n", time.Now().Format("1月2日 15:04"), found)
	b.WriteString(`
前に聞いていたことと、いま聞いたことを比べてください。時刻がたてば変わるのが当たり前のこと（今の時刻から探した電車など）は、それだけでは「違う」ではありません。頼んだ人の予定にとって意味のある変化があれば「違う」です。

次の2行だけを書いてください。
判定: 同じ / 違う / わからない のどれか
理由: 一文`)
	return b.String()
}

// readErrandJudgement reads the model's verdict and its reason.
func readErrandJudgement(text string) (verdict, why string, ok bool) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "-*・ "))
		key, val, found := strings.Cut(line, ":")
		if !found {
			key, val, found = strings.Cut(line, "：")
		}
		if !found {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "判定":
			switch {
			case strings.Contains(val, "わからない"):
				verdict = errandUnknown
			case strings.Contains(val, "違う"):
				verdict = errandDifferent
			case strings.Contains(val, "同じ"):
				verdict = errandSame
			}
		case "理由":
			why = val
		}
	}
	return verdict, why, verdict != ""
}

// characterName is a character's name, else its id.
func characterName(id string) string {
	if c, ok := mywant.GetCharacter(id); ok && c.Name != "" {
		return c.Name
	}
	return id
}
