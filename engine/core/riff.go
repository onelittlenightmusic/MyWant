package mywant

import (
	"math/rand"
	"strings"
)

// A riff is the smallest generative move in mywant: take the things someone has
// NAMED in their world (aura definitions) and the TOYS the world offers (effect
// want types), and propose one absurd, personal wiring between them —
// "『会社』に着いたら、世界に花火を上げる。" The point is not usefulness; it is
// the surprise of seeing your own word ("会社") wired to something silly. Because
// the vocabulary is yours, the proposal is yours even when it surprises you.
//
// The generator is deliberately combinatorial, not LLM-driven: the delight comes
// from the juxtaposition of a real named thing with a playful reaction, which a
// template captures without any model. Wit can be layered on later; first prove
// that "it uses MY world" already raises a smile.

// RiffNamedThing is one entry from someone's named vocabulary (an aura
// definition): a place, a person, a station — whatever they pressed X to name.
type RiffNamedThing struct {
	Kind string // catalog kind, e.g. "place", "location_coordinate", "station"
	Name string // the name they gave it, e.g. "会社"
}

// RiffSource is a deployed want that can act as a trigger in its own right
// (a location feed, a weather want) even before anything is named.
type RiffSource struct {
	Type string // want type, e.g. "weather", "location", "spotify"
	Name string // the deployed want's name
}

// RiffReaction is a toy the world can perform — an effect want type.
type RiffReaction struct {
	Type  string // want type, e.g. "fireworks_effect"
	Title string // human title, e.g. "Fireworks"
}

// RiffInput is the raw material a riff is generated from.
type RiffInput struct {
	Named     []RiffNamedThing
	Sources   []RiffSource
	Reactions []RiffReaction
}

// RiffProposal is one generated wiring, ready to show (and later, to deploy).
type RiffProposal struct {
	Text         string `json:"text"`         // the one-line proposal, in the user's voice
	TriggerKind  string `json:"triggerKind"`  // "named" or "source"
	TriggerName  string `json:"triggerName"`  // the named thing or source want name
	ReactionType string `json:"reactionType"` // the effect want type to deploy
}

// riffTrigger is an internal, already-phrased trigger clause plus its origin.
type riffTrigger struct {
	clause      string // "「会社」に着いたら"
	kind        string // "named" | "source"
	name        string // origin name, for the proposal's TriggerName
	fromNamed   bool   // true if grounded in a user-named thing (preferred)
}

// GenerateRiffs returns up to n proposals wiring the input's triggers to its
// reactions. It favours triggers grounded in named things — that personal hook
// is the whole point — and avoids repeating the same trigger/reaction pair.
// seed makes the selection deterministic (for tests and reproducible surfacing);
// pass a clock-derived seed in production to vary suggestions between calls.
//
// With no reactions or no triggers it returns nothing: that emptiness is itself
// the signal to go name more things (or deploy a toy) before the world can riff.
func GenerateRiffs(in RiffInput, n int, seed int64) []RiffProposal {
	if n <= 0 || len(in.Reactions) == 0 {
		return nil
	}
	triggers := buildTriggers(in)
	if len(triggers) == 0 {
		return nil
	}

	rng := rand.New(rand.NewSource(seed))
	// Named-thing triggers first, so when we have both, the personal ones win
	// the limited proposal slots. Within each group, shuffle for variety.
	named := filterTriggers(triggers, true)
	rest := filterTriggers(triggers, false)
	rng.Shuffle(len(named), func(i, j int) { named[i], named[j] = named[j], named[i] })
	rng.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	ordered := append(named, rest...)

	seen := map[string]bool{}
	out := make([]RiffProposal, 0, n)
	for _, tr := range ordered {
		re := in.Reactions[rng.Intn(len(in.Reactions))]
		key := tr.name + "|" + re.Type
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, RiffProposal{
			Text:         tr.clause + "、" + reactionClause(re) + "。",
			TriggerKind:  tr.kind,
			TriggerName:  tr.name,
			ReactionType: re.Type,
		})
		if len(out) >= n {
			break
		}
	}
	return out
}

func buildTriggers(in RiffInput) []riffTrigger {
	triggers := make([]riffTrigger, 0, len(in.Named)+len(in.Sources))
	for _, t := range in.Named {
		triggers = append(triggers, riffTrigger{
			clause:    namedTriggerClause(t),
			kind:      "named",
			name:      t.Name,
			fromNamed: true,
		})
	}
	for _, s := range in.Sources {
		triggers = append(triggers, riffTrigger{
			clause:    sourceTriggerClause(s),
			kind:      "source",
			name:      s.Name,
			fromNamed: false,
		})
	}
	return triggers
}

func filterTriggers(all []riffTrigger, fromNamed bool) []riffTrigger {
	out := make([]riffTrigger, 0, len(all))
	for _, t := range all {
		if t.fromNamed == fromNamed {
			out = append(out, t)
		}
	}
	return out
}

// namedTriggerClause phrases a trigger from a named thing, leaning on the kind
// to guess how you'd "arrive at" it. Unknown kinds fall back to a generic
// "when 「X」 comes up", which still carries the personal name.
func namedTriggerClause(t RiffNamedThing) string {
	k := strings.ToLower(t.Kind)
	switch {
	case strings.Contains(k, "place"), strings.Contains(k, "location"), strings.Contains(k, "coordinate"), strings.Contains(k, "station"), strings.Contains(k, "city"), strings.Contains(k, "address"):
		return "「" + t.Name + "」に着いたら"
	case strings.Contains(k, "person"), strings.Contains(k, "people"), strings.Contains(k, "attendee"), strings.Contains(k, "contact"):
		return "次の予定に「" + t.Name + "」がいたら"
	default:
		return "「" + t.Name + "」が動いたら"
	}
}

// sourceTriggerClause phrases a trigger from a deployed source want by its type.
func sourceTriggerClause(s RiffSource) string {
	switch strings.ToLower(s.Type) {
	case "weather":
		return "雨が降りそうなとき"
	case "location":
		return "あなたが動き出したら"
	case "spotify":
		return "曲が変わったら"
	case "transit", "transit_search":
		return "電車が遅れたら"
	default:
		return "「" + s.Name + "」が変わったら"
	}
}

// reactionClause phrases an effect want type as something the world does.
func reactionClause(r RiffReaction) string {
	switch strings.ToLower(r.Type) {
	case "fireworks_effect":
		return "世界に花火を上げる"
	case "chime_effect":
		return "チャイムを鳴らす"
	case "heart_effect":
		return "ハートを飛ばす"
	case "aura":
		return "足元の床をあなたの色に染める"
	case "aura_erase":
		return "足元の色を消し去る"
	case "weather":
		return "世界の天気を変える"
	default:
		title := r.Title
		if title == "" {
			title = r.Type
		}
		return title + "を起こす"
	}
}
