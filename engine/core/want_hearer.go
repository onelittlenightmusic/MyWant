package mywant

import (
	"context"
	"encoding/json"
	"strings"
)

// What a want does with a murmur it heard (want_hear.go), apart from its
// Progress: its type's hearer (a Go function, RegisterHearer, or
// <type>.hear.py) reads the murmur — what changed, the advice given — and the
// want as it is, and answers with a line to say and the things it proposes
// to do. A proposal is not done: the person is asked (the robot's はい /
// いいえ, server/hear.go), and only a yes does it.

// What a hearer may propose — each carried out by the engine, on a yes.
const (
	// ProposeAppend adds Value to the list in Field (a checklist's items).
	ProposeAppend = "append"
	// ProposeCreateWant makes a want (Want), marked as made by the hearer
	// about About — an item's key — so it can be found again.
	ProposeCreateWant = "create_want"
	// ProposeDeleteWants deletes the wants the hearer made about About.
	ProposeDeleteWants = "delete_wants"
	// ProposePostWebhook posts Body (JSON) to the URL in the hearer's param
	// URLParam.
	ProposePostWebhook = "post_webhook"
)

// What a hearer may do at once, unasked: only to itself, and harmless.
const (
	// ActWebhook hands the hearer an action as a webhook would (its
	// webhook_payload): a fireworks effect, trigger.
	ActWebhook = "webhook"
)

// Act is one thing a hearer does at once.
type Act struct {
	Action  string         `json:"action"`
	Payload map[string]any `json:"payload,omitempty"`
}

// ProposedWant is a want a hearer proposes to make.
type ProposedWant struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Params map[string]any `json:"params,omitempty"`
}

// Proposal is one thing a hearer proposes to do.
type Proposal struct {
	Action   string        `json:"action"`
	Field    string        `json:"field,omitempty"`
	Value    any           `json:"value,omitempty"`
	Want     *ProposedWant `json:"want,omitempty"`
	About    string        `json:"about,omitempty"`
	URLParam string        `json:"url_param,omitempty"`
	Body     any           `json:"body,omitempty"`
	// Ask is how the person is asked (「傘を持ち物に足しますか？」); Why, the
	// reason, from the advice it came of.
	Ask string `json:"ask"`
	Why string `json:"why,omitempty"`
}

// HearerInput is what a hearer reads.
type HearerInput struct {
	Want  HearerWant  `json:"want"`
	Heard HeardMurmur `json:"heard"`
}

// HearerWant is the hearer as it is: who, and its state (what it keeps).
type HearerWant struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Type  string         `json:"type"`
	State map[string]any `json:"state,omitempty"`
}

// HearerOutput is a hearer's answer: a line to say (none: its voice answers
// for it), and its proposals.
type HearerOutput struct {
	Say       string     `json:"say,omitempty"`
	Proposals []Proposal `json:"proposals,omitempty"`
	Do        []Act      `json:"do,omitempty"`
}

// Hearer answers a murmur a want of its type heard.
type Hearer func(in HearerInput) HearerOutput

var hearers = map[string]Hearer{}

// RegisterHearer gives a Go type its hearer.
func RegisterHearer(wantType string, h Hearer) { hearers[wantType] = h }

// HasHearer says whether a type answers murmurs by a hearer (a function, or
// a script — script, "" for none).
func HasHearer(wantType, script string) bool { return hearers[wantType] != nil || script != "" }

// HearOn is what a want's type does with a murmur it heard: its hearer, else
// its hear script. Proposals without an action it can carry out are dropped.
func HearOn(ctx context.Context, w *Want, m HeardMurmur, script string) HearerOutput {
	if w == nil {
		return HearerOutput{}
	}
	in := HearerInput{
		Want:  HearerWant{ID: w.Metadata.ID, Name: w.Metadata.Name, Type: w.Metadata.Type, State: w.GetAllState()},
		Heard: m,
	}
	var out HearerOutput
	if h := hearers[in.Want.Type]; h != nil {
		out = h(in)
	} else if script != "" {
		out = scriptHear(ctx, w, script, in)
	}
	kept := out.Proposals[:0]
	for _, p := range out.Proposals {
		if validProposal(p) {
			kept = append(kept, p)
		}
	}
	out.Proposals = kept
	acts := out.Do[:0]
	for _, a := range out.Do {
		if a.Action == ActWebhook && a.Payload != nil {
			acts = append(acts, a)
		}
	}
	out.Do = acts
	return out
}

// validProposal: an action the engine knows, with what it needs, asked.
func validProposal(p Proposal) bool {
	if strings.TrimSpace(p.Ask) == "" {
		return false
	}
	switch p.Action {
	case ProposeAppend:
		return p.Field != "" && p.Value != nil
	case ProposeCreateWant:
		return p.Want != nil && p.Want.Type != "" && p.About != ""
	case ProposeDeleteWants:
		return p.About != ""
	case ProposePostWebhook:
		return p.URLParam != "" && p.Body != nil
	}
	return false
}

func scriptHear(ctx context.Context, w *Want, script string, in HearerInput) HearerOutput {
	input, err := json.Marshal(in)
	if err != nil {
		return HearerOutput{}
	}
	got, err := runSideScript(ctx, script, input)
	if err != nil {
		ErrorLog("[HEAR] %s (%s): %v", w.Metadata.Name, script, err)
		return HearerOutput{}
	}
	raw, err := json.Marshal(got)
	if err != nil {
		return HearerOutput{}
	}
	var out HearerOutput
	_ = json.Unmarshal(raw, &out)
	return out
}

// MadeByLabel and AboutLabel mark a want a hearer made (ProposeCreateWant):
// which hearer, and about which item.
const (
	MadeByLabel = "mywant.io/made-by"
	AboutLabel  = "mywant.io/about"
)

// AppendTo does an append proposal the person said yes to, on the want that
// made it (the server carries out the rest, server/hear.go).
func AppendTo(w *Want, p Proposal) error {
	switch p.Action {
	case ProposeAppend:
		list, _ := w.getState(p.Field)
		items, _ := list.([]any)
		items = append(append([]any{}, items...), p.Value)
		w.StoreState(p.Field, items)
		return nil
	}
	return errUnknownProposal
}

type proposalError string

func (e proposalError) Error() string { return string(e) }

const errUnknownProposal = proposalError("unknown proposal action")
