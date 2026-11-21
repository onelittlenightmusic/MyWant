package mywant

import (
	"sync"
	"time"

	want_spec "github.com/onelittlenightmusic/want-spec"
)

// Wants hearing each other: a want's murmur — its answer told at one of its
// type's timings — heard by the wants that listen for it (spec.hear: a type,
// its timings, the nearest of them on the board). Each hearer is handed what
// it heard and woken; its Progress reads it (Hear, or a plugin script's
// current().heard) and may answer — a line of its own in `say`, naming the
// murmur it answers (SayInReply), so the conversation can be traced back.
// A want whose type answers nothing is answered for by its voice
// (server/hear.go): the type's voice script, told it heard.

// HearSpec is one kind of murmur a want hears (spec.hear).
type HearSpec = want_spec.HearSpec

// HeardMurmur is one murmur a want heard.
type HeardMurmur struct {
	// MurmurID is the murmur's own id (its notice's), the same everywhere it
	// is named — a reply to it carries it.
	MurmurID string         `json:"murmur_id"`
	FromID   string         `json:"from_want_id"`
	FromName string         `json:"from_want_name"`
	FromType string         `json:"from_want_type"`
	Timings  []string       `json:"timings,omitempty"`
	Text     string         `json:"text"`
	Result   any            `json:"result,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
	// Labels: what it is about in set words — MurmurLabelTime, MurmurLabelPlace.
	Labels map[string]string `json:"labels,omitempty"`
	At     time.Time         `json:"at"`
	// Meaning: what Hear was asked to find in it ("when", "where", …), each
	// with how likely each candidate is — its labels' when it has them.
	Meaning map[string]HeardChoice `json:"meaning,omitempty"`
}

// HeardChoice is what a murmur says for one thing asked of it: the likeliest
// candidate, and every candidate's likelihood.
type HeardChoice struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
}

// The things Hear can be asked to find in a murmur, and the label that
// answers each when the murmur carries it.
const (
	HearWhen  = "when"
	HearWhere = "where"
)

var hearAskLabel = map[string]string{HearWhen: MurmurLabelTime, HearWhere: MurmurLabelPlace}

// HearInterpret, when set (by the server: Laya, else rules), finds what an
// ask means in a murmur that carries no label for it.
var HearInterpret func(hearer *Want, m HeardMurmur, ask string) (HeardChoice, bool)

// Interpret fills in what each ask means in the murmur: from its labels
// first (a label is what the speaker said it is about), else HearInterpret.
func Interpret(hearer *Want, m HeardMurmur, asks []string) HeardMurmur {
	if len(asks) == 0 {
		return m
	}
	m.Meaning = map[string]HeardChoice{}
	for _, ask := range asks {
		if key, ok := hearAskLabel[ask]; ok {
			if v := m.Labels[key]; v != "" {
				m.Meaning[ask] = HeardChoice{Choice: v, Probabilities: map[string]float64{v: 1}}
				continue
			}
		}
		if HearInterpret != nil {
			if c, ok := HearInterpret(hearer, m, ask); ok {
				m.Meaning[ask] = c
			}
		}
	}
	return m
}

// The set labels a murmur carries (its voice's, else read off its answer —
// server/hear.go), the same for every type, so a want that hears can read
// them whoever said it: when it is about, and where.
const (
	MurmurLabelTime  = "time"  // RFC3339
	MurmurLabelPlace = "place" // a place's name
)

// SayReplyToState is the field naming the murmur a want's `say` answers.
const SayReplyToState = "say_reply_to"

// heardMax is how many murmurs a want keeps unread: the newest win.
const heardMax = 20

var (
	heardMu sync.Mutex
	heard   = map[string][]HeardMurmur{} // by hearer want id
)

// DeliverHeard hands a murmur to the want that heard it, to read when it runs.
func DeliverHeard(wantID string, m HeardMurmur) {
	if wantID == "" {
		return
	}
	heardMu.Lock()
	defer heardMu.Unlock()
	list := append(heard[wantID], m)
	if len(list) > heardMax {
		list = list[len(list)-heardMax:]
	}
	heard[wantID] = list
}

// Hear is what the want heard since it was last answered for, oldest first —
// read in its Progress to answer it. Asked for things (HearWhen, HearWhere),
// each murmur says what they are (Meaning): its labels' when it carries them,
// else found in it (HearInterpret). Reading does not use it up: what is not
// answered (SayInReply) is answered for by its voice after it runs.
func (n *Want) Hear(asks ...string) []HeardMurmur {
	heard := PeekHeard(n.Metadata.ID)
	for i := range heard {
		heard[i] = Interpret(n, heard[i], asks)
	}
	return heard
}

// PeekHeard is what the want heard, not taken.
func PeekHeard(wantID string) []HeardMurmur {
	heardMu.Lock()
	defer heardMu.Unlock()
	return append([]HeardMurmur(nil), heard[wantID]...)
}

// TakeHeard is what the want heard, taken: it is answered (or let go).
func TakeHeard(wantID string) []HeardMurmur {
	heardMu.Lock()
	defer heardMu.Unlock()
	out := heard[wantID]
	delete(heard, wantID)
	return out
}

// SayInReply says a line in answer to a murmur heard: its `say`, and the
// murmur it answers, for the conversation to be traced back.
func (n *Want) SayInReply(m HeardMurmur, line string) {
	if line == "" {
		return
	}
	n.StoreState(SayReplyToState, m.MurmurID)
	n.StoreState("say", line)
}

// heardForScript is what a plugin script is handed of it (MRSWantContext).
func heardForScript(wantID string) []map[string]any {
	var out []map[string]any
	for _, m := range PeekHeard(wantID) {
		out = append(out, map[string]any{
			"murmur_id": m.MurmurID, "from_want_id": m.FromID, "from_want_name": m.FromName,
			"from_want_type": m.FromType, "timings": m.Timings, "text": m.Text,
			"result": m.Result, "fields": m.Fields, "labels": m.Labels, "at": m.At.Format(time.RFC3339),
		})
	}
	return out
}
