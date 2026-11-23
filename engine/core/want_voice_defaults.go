package mywant

import "strings"

// want_voice_defaults.go — what a want says when its type gives it nothing to
// say: about itself (its status, how much it has done) to the one standing on
// it, and about each thing that happens to it (an event). One place, so the
// board's words are written once and not in every type's voice script, nor in
// the board's own code.
//
// Kept by language. Japanese is written first and is what is said today
// (VoiceLangDefault); another language is another book, and a line missing
// from it is said from the Japanese. For its own news (output, heard) a
// type's voice script (<type>.voice.py) is asked first and may say it its own
// way; what happens to a want on the board (hear_*) is said in these words.

// VoiceLangDefault is the language the default lines are said in.
const VoiceLangDefault = "ja"

// What happened, for a script saying a want's news rather than what it says
// to the one standing on it.
const (
	VoiceEventOutput   = "output"   // a new answer; Result is it
	VoiceEventAchieved = "achieved" // it got where it was going
	VoiceEventHeard    = "heard"    // it heard another want's murmur; Heard is it

	// Who a hearing want hears (spec.hear: the nearest of a type, within
	// earshot) changed because something on the board moved. Vars: from (the
	// one it hears now, or heard), to (the one heard instead).
	VoiceEventHearGained = "hear_gained" // one came into earshot: from
	VoiceEventHearFar    = "hear_far"    // the one it heard is too far now: from
	VoiceEventHearTaken  = "hear_taken"  // another is nearer: from, the one lost; to, the new one
)

// voiceStep is the lines of an experience threshold.
type voiceStep struct {
	at    int
	lines []string
}

// voiceBook is one language's default words.
type voiceBook struct {
	experience []voiceStep
	status     map[WantStatus][]string
	achieved   []string
	warning    string
	// events: by event kind; {name} is a var of the event.
	events map[string][]string
}

var voiceBooks = map[string]*voiceBook{
	"ja": {
		experience: jaExperience,
		status:     jaStatus,
		achieved:   jaAchieved,
		warning:    jaWarning,
		events: map[string][]string{
			VoiceEventHearGained: {"{from} が聞こえるようになった", "{from} の声が届くようになった"},
			VoiceEventHearFar:    {"{from} が遠くて聞こえなくなった", "{from} は遠すぎて聞こえない"},
			VoiceEventHearTaken:  {"{to} のほうが近い、{from} は聞こえなくなった"},
		},
	},
}

// defaultBook is the book of the language said today.
func defaultBook() *voiceBook { return voiceBooks[VoiceLangDefault] }

// EventVoiceLines is what any want says of an event in lang (the default
// language's when lang has none), its vars filled in; nil when nothing is
// said of it by default.
func EventVoiceLines(lang, kind string, vars map[string]string) []string {
	book := voiceBooks[lang]
	if book == nil || book.events[kind] == nil {
		book = defaultBook()
	}
	lines := book.events[kind]
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		for k, v := range vars {
			l = strings.ReplaceAll(l, "{"+k+"}", v)
		}
		out[i] = l
	}
	return out
}

// The Japanese book.

// jaExperience is what a want with no voice of its own says, by how much it
// has done: the lines of the highest threshold it has reached. Twenty is the
// top because the engine keeps twenty answers (resultHistoryDepth) and so
// twenty is "a lot"; below five it is still learning, and says so.
var jaExperience = []voiceStep{
	{20, []string{"ベテランです", "いつもありがとう", "なんでも聞いて", "今日も元気です", "ずっとここにいるよ"}},
	{15, []string{"もう目をつぶってもできる", "いい感じに回ってる"}},
	{10, []string{"%d回こなしたよ", "今日も任せて", "調子いいよ"}},
	{5, []string{"だんだん慣れてきたよ", "ちょっとずつ分かってきた"}},
	{1, []string{"まだ%d回だけ、覚えてる途中", "少しずつ慣れてきた"}},
	{0, []string{"はじめての仕事だ", "はじめまして、やってみるね"}},
}

// jaStatus is what a want says about where it stands, before anything about
// how much it has done: a want waiting for you or one that failed has that to
// say first. Quiet words, never a call — the board is somewhere to come back
// to, not something that shouts. A status not here (reaching, achieved past
// its first time) leaves it to experience; deleting says nothing.
var jaStatus = map[WantStatus][]string{
	WantStatusIdle:              {"準備してるよ", "これから始めるね"},
	WantStatusInitializing:      {"準備してるよ", "これから始めるね"},
	WantStatusPrepareAgent:      {"準備してるよ", "これから始めるね"},
	WantStatusWaitingUserAction: {"あなたの返事を待ってるよ", "急がなくていいよ"},
	WantStatusSuspended:         {"ひと休み中", "また呼んでね"},
	WantStatusFailed:            {"うまくいかなかった…", "もう一回やらせて"},
	WantStatusConfigError:       {"設定を見てもらえる？"},
	WantStatusModuleError:       {"調子が悪いみたい"},
	WantStatusCancelled:         {"別のに替わったよ"},
	WantStatusTerminated:        {"おしまい"},
	WantStatusArchived:          {"しまわれてます"},
}

// jaAchieved is what a want that has finished says while it has little
// experience to speak of; past that it talks like any seasoned want.
var jaAchieved = []string{"できたよ", "おわった、ひと休み"}

// jaWarning is mixed into what a want says when it got where it is with
// something logged against it (reaching/achieved with a warning).
const jaWarning = "…ちょっと気になることがある"
