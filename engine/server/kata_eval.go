package server

import (
	"fmt"
	"sort"
	"strings"

	mywant "mywant/engine/core"
)

// Kata evaluation.
//
// Progress is computed on demand from live data — deployed wants, the thing store
// and past practice records — so "どこが足りないか" is always current without a
// background loop. The only thing written back is a practice record, the moment a
// kata is found 極まった for a set of witnesses it has not been credited for yet.
//
// A kata that declares a join is evaluated once per scope: its 所作 must all
// resolve inside the SAME group, which is how two wants are known to be about the
// same place. Nothing here looks at time or order — a kata is a set of wants that
// belong together, never a schedule.

// WazaProgress is one 所作 with its current standing.
type WazaProgress struct {
	Waza      mywant.Waza `json:"waza"`
	Satisfied bool        `json:"satisfied"`
	Have      int         `json:"have"`
	Need      int         `json:"need"`
	// MatchedIDs are the want IDs (or things) that satisfied this waza.
	MatchedIDs []string `json:"matchedIDs,omitempty"`
	// Hint is what the user would have to do to satisfy it, in one short phrase.
	Hint string `json:"hint,omitempty"`
}

// KataSuggestion is one move that would complete a form the player already
// knows, on a constellation that is already on the board.
//
// The point is that it makes nothing. A form is not produced, no value is
// invented, nothing is placed — it names a connection the player could draw
// between two things they already have, and the drawing is theirs to do. The
// board's own answer to "what now?", built out of what is already there.
//
// Only for forms the player HAS: a veiled one is withheld here as everywhere
// else, because "add a station to 国分寺 and you will find something" is the
// answer to the question the veil is asking.
type KataSuggestion struct {
	KataID string `json:"kataID"`
	Name   string `json:"name"`
	// Mark is what the completed form would leave, so the offer can be drawn
	// as the thing it would become.
	Mark *mywant.KataMark `json:"mark,omitempty"`
	// The constellation this is about — the one place the move happens.
	Constellation string `json:"constellation"`
	// Lone says the "constellation" is a single value standing on its own —
	// there is no group yet, and accepting the offer makes one.
	Lone bool `json:"lone,omitempty"`
	// What is missing: a `thing` of this subtype, or a `want_type` of this
	// type. The two are different offers — one joins two tiles that exist, the
	// other puts a tile down that does not — so the board draws them
	// differently, but both are "one move and this form holds".
	Kind    string `json:"kind"`
	Subtype string `json:"subtype,omitempty"`
	Type    string `json:"type,omitempty"`
	// One short phrase naming the move, from the waza's own hint.
	Hint string `json:"hint,omitempty"`
	// AnchorKind says which map the anchor lives in: a thing of this scope, or
	// a want already on the board. A wire offer anchors on the WANT it would be
	// wired to — that is where the move happens, and beside a station is not.
	AnchorKind string `json:"anchorKind,omitempty"`
	// Anchor is a thing of this scope that is on the board — where the offer is
	// drawn from. Candidate is the thing that would complete it, for a `thing`
	// offer; a `want_type` offer has none, because the want it asks for does
	// not exist yet.
	Anchor    string `json:"anchor"`
	Candidate string `json:"candidate,omitempty"`
}

// KataProgress is a kata's standing: how far along, how deep, and whether it is
// even visible yet.
type KataProgress struct {
	KataID  string `json:"kataID"`
	Name    string `json:"name"`
	Reading string `json:"reading,omitempty"`
	Level   string `json:"level"`
	Intent  string `json:"intent,omitempty"`
	Yields  string `json:"yields,omitempty"`
	// Mark is what the form leaves where it was 極まった, drawn rather than
	// described. Behind a veil only its icon ships, without the word under it —
	// enough to be curious about, not enough to be the answer.
	Mark      *mywant.KataMark `json:"mark,omitempty"`
	Contains  []string         `json:"contains,omitempty"`
	Variation string           `json:"variation,omitempty"`
	// Suggestions are the moves that would complete this form somewhere it is
	// one 所作 short. Empty for a form that is masked, complete everywhere, or
	// nowhere near.
	Suggestions []KataSuggestion `json:"suggestions,omitempty"`
	// Constellations is every group this form currently stands on, in the order
	// they were measured. `Constellation` below is the one the card speaks for;
	// this is the whole set, because a form is not held in one place only — two
	// stations are a line on 中央線 AND on 丸の内線 — and the board has to mark
	// every line it stands on, not the first one the loop happened to meet.
	Constellations []string `json:"constellations,omitempty"`
	// Group is the scope this standing was measured against — the shared thing
	// the 所作 are all about, whether a constellation or a single value standing
	// on its own. Empty for kata that declare no join.
	Group string `json:"group,omitempty"`
	// Constellation is Group under the name the rest of the system uses for it.
	Constellation string `json:"constellation,omitempty"`

	Waza      []WazaProgress `json:"waza"`
	Satisfied int            `json:"satisfied"`
	Total     int            `json:"total"`
	Complete  bool           `json:"complete"`

	// Live: the kata stands right now, on evidence that is still on the board.
	// Not the same as Complete — a 口伝 built from `repeat` is satisfied out of
	// the record book, so it completes with nothing currently deployed and must
	// not be drawn as a burning constellation.
	Live bool `json:"live"`
	// Recorded: it has been 極まった at least once before. Live and Recorded are
	// independent — either can hold without the other.
	Recorded bool `json:"recorded"`
	// The live evidence itself: want ids, and things as "catalogKey::value"
	// so they match the member ids constellations are made of.
	LiveWantIDs []string `json:"liveWantIDs,omitempty"`
	LiveThings  []string `json:"liveThings,omitempty"`
	// AlmostThere marks あと一所作 — the only moment a kata is allowed to speak up.
	AlmostThere bool `json:"almostThere"`

	Mastery     int                           `json:"mastery"`
	MasteryRank string                        `json:"masteryRank,omitempty"`
	Thresholds  mywant.MasteryThresholds      `json:"thresholds"`
	Unlocks     map[string]mywant.KataUnlocks `json:"unlocks,omitempty"`
	Earned      []mywant.KataUnlocks          `json:"earned,omitempty"`

	// Locked: the belt this kata belongs to has not been opened yet.
	Locked bool `json:"locked"`
	// Masked: a 口伝 that is neither complete nor one waza away — its 所作 are
	// withheld server-side so it genuinely cannot be ground for.
	Masked bool `json:"masked"`
	Hidden bool `json:"hidden"`
	// Veiled means the blanks are shown and their contents are not — see the
	// masking below, and Kata.Veiled for why the two secrets differ.
	Veiled bool `json:"veiled"`
}

// LevelProgress is one belt's standing.
type LevelProgress struct {
	mywant.KataLevel
	Unlocked bool     `json:"unlocked"`
	Achieved int      `json:"achieved"` // kata in this level that are 極まった at least once
	Required int      `json:"required"`
	Promoted bool     `json:"promoted"`
	KataIDs  []string `json:"kataIDs"`
}

// thingScope is one scope a joined kata is measured in: a set of values taken to
// be one thing.
type thingScope struct {
	Name string
	// Values that belong to the group, keyed by subtype.
	BySubtype map[string]map[string]bool
	// Every value in the group regardless of subtype.
	AllValues map[string]bool
	// MemberIDs are the thing ids behind those values — what a suggestion needs
	// in order to be a line between two tiles rather than a sentence about two
	// names. Positions live on a thing's own labels, and labels are keyed by id.
	MemberIDs []string
	// Lone marks a value standing on its own — no constellation, just itself.
	// A form that needs one remembered value holds in such a scope; one that
	// needs two different KINDS of value cannot, which is the whole point.
	Lone bool
}

// has reports whether the group holds a value of this subtype.
func (g *thingScope) has(subtype string) bool { return len(g.BySubtype[subtype]) > 0 }

// collectKataScopes builds every scope a joined kata can be measured in.
//
// A constellation is one scope: the values the user has declared to be one
// thing. But grouping is only ever needed to tie DIFFERENT values together —
// "国分寺" the station and "Kokubunji" the city — and a form that points a single
// want at a single remembered value needs no such declaration. So every value
// that is in no constellation also stands as a scope of its own.
//
// That keeps the constellation doing the work only it can do (傘 needs a station
// AND a city, which no lone value can supply) without making it a gate in front
// of the belt below it (空 is one city and one forecast, and asking for a group
// there would put a white-belt move in the way of a yellow-belt one).
//
// Members arrive as "catalogKey::value" (e.g. "stations::<name>"), so the catalog
// key is mapped back to its data type name.
func (s *Server) collectKataScopes() []thingScope {
	keyToSubtype := make(map[string]string, len(dataTypeDefs))
	for name, info := range dataTypeDefs {
		// First declaration wins: several subtypes share a key (int/integer),
		// and either name resolves the same values.
		if _, seen := keyToSubtype[info.Key]; !seen {
			keyToSubtype[info.Key] = name
		}
	}
	subtypeOf := func(key string) string {
		if subtype := keyToSubtype[key]; subtype != "" {
			return subtype
		}
		return strings.TrimSuffix(key, "s")
	}

	var out []thingScope
	grouped := map[string]bool{} // "catalogKey::value" already in a constellation

	byID := map[string]ThingEntry{}
	for _, e := range s.thingStore.Entries() {
		byID[e.ID] = e
	}
	for _, g := range s.collectThingConstellations() {
		mg := thingScope{
			Name:      g.Name,
			BySubtype: map[string]map[string]bool{},
			AllValues: map[string]bool{},
		}
		for _, member := range g.Members {
			// Members are thing ids, and an id is a UUID that says nothing about
			// what it is or what it is called — so ask the store rather than
			// taking the name apart, which is what this used to do.
			entry, ok := byID[member]
			if !ok {
				continue
			}
			key, value := entry.Catalog, entry.Value
			grouped[key+"::"+value] = true
			subtype := subtypeOf(key)
			if mg.BySubtype[subtype] == nil {
				mg.BySubtype[subtype] = map[string]bool{}
			}
			mg.BySubtype[subtype][value] = true
			mg.AllValues[value] = true
			mg.MemberIDs = append(mg.MemberIDs, member)
		}
		out = append(out, mg)
	}

	// Everything not spoken for stands alone. A value already inside a
	// constellation is left out: it would otherwise be measured twice and bank
	// two practices for one piece of evidence.
	for key, values := range s.thingStore.All() {
		subtype := subtypeOf(key)
		for _, value := range values {
			if value == "" || grouped[key+"::"+value] {
				continue
			}
			lone := thingScope{
				Name:      value,
				BySubtype: map[string]map[string]bool{subtype: {value: true}},
				AllValues: map[string]bool{value: true},
				Lone:      true,
			}
			for _, e := range s.thingStore.Entries() {
				if e.Catalog == key && e.Value == value {
					lone.MemberIDs = append(lone.MemberIDs, e.ID)
				}
			}
			out = append(out, lone)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// evaluateKata computes progress for every level and kata.
//
// A pass can credit practice, and credited practice can promote a belt — which
// changes what is unlocked, which changes what the next pass may credit. Run
// again whenever something was recorded so a kata never reports itself locked in
// the same breath as the promotion that opened it.
func (s *Server) evaluateKata() ([]LevelProgress, []KataProgress) {
	levels, kata, _ := s.evaluateKataTracking()
	return levels, kata
}

// evaluateKataTracking is evaluateKata plus the ids of every kata that became
// held during this call — what the SSE announcement is built from.
func (s *Server) evaluateKataTracking() ([]LevelProgress, []KataProgress, []string) {
	levels, kata, credited := s.evaluateKataPass()
	newly := append([]string{}, credited...)
	for i := 0; len(credited) > 0 && i < len(mywant.ListKataLevels()); i++ {
		levels, kata, credited = s.evaluateKataPass()
		newly = append(newly, credited...)
	}
	return levels, kata, newly
}

// evaluateKataPass is one evaluation sweep. It reports the ids of every kata it
// credited with new practice.
func (s *Server) evaluateKataPass() ([]LevelProgress, []KataProgress, []string) {
	levels := mywant.ListKataLevels()
	allKata := mywant.ListKata()

	// ── Gather evidence ───────────────────────────────────────────────────────
	wantsByType := map[string][]*mywant.Want{}
	if s.globalBuilder != nil {
		for _, w := range s.globalBuilder.GetAllWantStates() {
			wantsByType[w.Metadata.Type] = append(wantsByType[w.Metadata.Type], w)
		}
	}
	groups := s.collectKataScopes()

	// ── Belts first, from the records already on disk ─────────────────────────
	// Which belts are open has to be settled before anything is evaluated:
	// a kata in a belt you have not reached must not bank practice, or a locked
	// belt would quietly fill itself in from wants deployed for other reasons.
	sort.SliceStable(levels, func(i, j int) bool { return levels[i].Order < levels[j].Order })
	levelProgress := make([]LevelProgress, 0, len(levels))
	unlockedLevel := make(map[string]bool, len(levels))
	prevPromoted := true // there is no belt before the first one
	for _, lv := range levels {
		achieved := 0
		for _, id := range lv.Kata {
			if mywant.KataMasteryCount(id) > 0 {
				achieved++
			}
		}
		required := lv.Promote.RequiredKata
		if required <= 0 {
			required = len(lv.Kata)
		}
		unlocked := lv.Unlocked || prevPromoted
		promoted := achieved >= required
		unlockedLevel[lv.ID] = unlocked
		levelProgress = append(levelProgress, LevelProgress{
			KataLevel: lv,
			Unlocked:  unlocked,
			Achieved:  achieved,
			Required:  required,
			Promoted:  promoted,
			KataIDs:   lv.Kata,
		})
		// The next belt only opens once this one is both open and cleared.
		prevPromoted = unlocked && promoted
	}

	// ── Evaluate each kata, best variation wins ───────────────────────────────
	out := make([]KataProgress, 0, len(allKata))
	var recorded []string
	for _, k := range allKata {
		open := unlockedLevel[k.Level]
		p, credited := s.evaluateOneKata(k, wantsByType, groups, open)
		if credited {
			recorded = append(recorded, k.ID)
		}
		p.Locked = !open

		// 口伝: withhold the 所作 until it is complete or one away — and never
		// reveal one that belongs to a belt still shut.
		if p.Hidden && p.Mastery == 0 && (!open || (!p.Complete && !p.AlmostThere)) {
			p.Masked = true
			p.Waza = nil
			p.Satisfied = 0
			p.AlmostThere = false
			p.Intent = ""
			p.Yields = ""
			p.Mark = nil
			p.Group = ""
			p.Constellation = ""
			// The witnesses would draw the form on the canvas as plainly as the
			// 所作 list would spell it out.
			p.LiveWantIDs = nil
			p.LiveThings = nil
			p.Constellations = nil
			// "Add a station to 国分寺 and you will find something" is the
			// answer to the question the veil is asking.
			p.Suggestions = nil
			p.Unlocks = nil
			// The name and its reading name the form as surely as its 所作 do,
			// so they are withheld too — only the ID and the count of 所作 ship.
			p.Name = ""
			p.Reading = ""
		}

		// 合: a veiled form keeps its SHAPE and loses its contents.
		//
		// The opposite secret to the 口伝 above, and the reason both exist. A
		// 口伝 hides that it is there; a veiled form is on the shelf with the
		// right number of blanks in it, because "two 所作 make a form" is the
		// invitation — hide the arity too and there is nothing to be curious
		// about. So the 所作 survive as blanks, and only what goes in them,
		// what it leaves, and what it is called are withheld.
		//
		// Lifted the moment it is 極まった, once, and it stays lifted: a form
		// you have held is one you know. 練度 is not the gate here — finding
		// it is. What 練度 still measures is holding the same form on DIFFERENT
		// constellations, which the credit key already distinguishes: knowing
		// that stations and cities go together, and knowing it in three
		// places, are not the same knowing.
		if p.Veiled && p.Mastery == 0 && !p.Complete {
			p.Masked = true
			for i := range p.Waza {
				// Kind and arity survive; what goes in the blank does not. A form
				// asking for TWO of something is a different invitation from one
				// asking for two different things, and the card can only draw the
				// right number of blanks if the count comes with them.
				p.Waza[i] = WazaProgress{
					Waza: mywant.Waza{Kind: p.Waza[i].Waza.Kind},
					Need: p.Waza[i].Need,
				}
			}
			p.Satisfied = 0
			p.AlmostThere = false
			p.Intent = ""
			p.Yields = ""
			// The mark keeps its PICTURE and loses its word — the same trade the
			// 所作 make. A shape behind the veil is the invitation ("this one is
			// about walking, and I have never walked anything"); the kanji under
			// it would be the answer. A 口伝 above gets no such hint: hiding that
			// the form is there at all is the whole of what it does.
			if p.Mark != nil {
				p.Mark = &mywant.KataMark{Icon: p.Mark.Icon}
			}
			p.Name = ""
			p.Reading = ""
			p.LiveWantIDs = nil
			p.LiveThings = nil
			p.Constellations = nil
			// "Add a station to 国分寺 and you will find something" is the
			// answer to the question the veil is asking.
			p.Suggestions = nil
			p.Unlocks = nil
		}
		out = append(out, p)
	}

	// A practice just credited can promote a belt, so recount after evaluating.
	for i := range levelProgress {
		achieved := 0
		for _, id := range levelProgress[i].KataIDs {
			if mywant.KataMasteryCount(id) > 0 {
				achieved++
			}
		}
		levelProgress[i].Achieved = achieved
		levelProgress[i].Promoted = achieved >= levelProgress[i].Required
	}

	return levelProgress, out, recorded
}

// evaluateOneKata scores every variation, against every scope when the kata
// declares a join, and returns the closest standing.
// Practice is only credited when the kata's belt is open.
func (s *Server) evaluateOneKata(
	k mywant.Kata,
	wantsByType map[string][]*mywant.Want,
	groups []thingScope,
	beltOpen bool,
) (KataProgress, bool) {
	var best KataProgress
	bestScore := -1
	credited := false
	// Every constellation this form stands on, not just the one it speaks for.
	var standing []string
	// And every place it is one move from standing.
	var suggestions []KataSuggestion

	// A joined kata is measured once per group; an unjoined one once, globally.
	scopes := []*thingScope{nil}
	if joinsOneGroup(k.Join.Kind) {
		scopes = nil
		for i := range groups {
			scopes = append(scopes, &groups[i])
		}
		if len(scopes) == 0 {
			// No groups yet: still report standing so the card can say so.
			scopes = []*thingScope{{Name: "", BySubtype: map[string]map[string]bool{}, AllValues: map[string]bool{}}}
		}
	}

	for _, henka := range k.Variations() {
		for _, scope := range scopes {
			wazaProgress := make([]WazaProgress, 0, len(henka.Waza))
			satisfied := 0
			witnesses := make([]string, 0, 8)

			matchedByType := map[string][]string{}
			for _, wz := range henka.Waza {
				wp := s.evaluateWaza(wz, wantsByType, scope, matchedByType)
				if wp.Satisfied {
					satisfied++
					witnesses = append(witnesses, wp.MatchedIDs...)
				}
				if wz.Kind == "want_type" && wz.Type != "" && len(wp.MatchedIDs) > 0 {
					matchedByType[wz.Type] = append(matchedByType[wz.Type], wp.MatchedIDs...)
				}
				wazaProgress = append(wazaProgress, wp)
			}

			groupName := ""
			if scope != nil {
				groupName = scope.Name
			}
			total := len(henka.Waza)
			complete := total > 0 && satisfied == total
			liveWants, liveThings := liveEvidence(wazaProgress)

			// One move short of a form of THREE or more, and the move is
			// worth naming.
			//
			// The arity is the filter that makes this usable. A form of two is
			// one thing plus one other thing, so every value on the board is
			// one move from it and the offers come in dozens — 近 alone
			// accounted for twenty-two of them, all saying the same thing
			// about the same city. A form of three or more is something the
			// player has already half-built on purpose, and being told which
			// single piece would finish it is an answer to a question they
			// were actually asking.
			//
			// Collected per scope for the same reason the standings are: the
			// question "what would finish this" is asked of a place, and the
			// board has many. Only inside a scope with a name — a form one 所作
			// short of holding on nothing at all has no line for the player to
			// draw, and "remember another station" is advice, not a move.
			if total >= 3 && satisfied == total-1 && groupName != "" {
				if sg := s.suggestFor(k, henka.Waza, wazaProgress, scope, matchedByType); sg != nil {
					suggestions = append(suggestions, *sg)
				}
			}

			// Every place it stands, before anything is ranked.
			//
			// Crediting used to sit below the ranking test, which meant a
			// second constellation holding the SAME form scored equal, hit the
			// `continue`, and was never credited at all — so 練度, which is
			// supposed to measure exactly that (the same form held in
			// different places), could never pass one. It also meant the form
			// named one constellation when it stood on three, and the board
			// drew its mark on only the first.
			if complete && groupName != "" {
				standing = append(standing, groupName)
			}

			// Credit the practice the first time this exact set of witnesses
			// completes it. The group joins the key, so the same combination
			// held for two different places counts as two practices.
			if complete && beltOpen {
				key := append([]string{}, witnesses...)
				if groupName != "" {
					key = append(key, "group:"+groupName)
				}
				if mywant.RecordKataPractice(mywant.KataRecord{
					KataID:     k.ID,
					SessionKey: mywant.SessionKeyFor(key),
					WantIDs:    witnesses,
					Variation:  henka.ID,
				}) {
					credited = true
				}
			}

			// Which one the card SPEAKS for: the furthest along, first found.
			if satisfied <= bestScore {
				continue
			}
			bestScore = satisfied

			best = KataProgress{
				KataID:        k.ID,
				Name:          k.Name,
				Reading:       k.Reading,
				Level:         k.Level,
				Intent:        k.Intent,
				Yields:        k.Yields,
				Contains:      k.Contains,
				Variation:     henka.ID,
				Group:         groupName,
				Constellation: groupName,
				Waza:          wazaProgress,
				Satisfied:     satisfied,
				Total:         total,
				Complete:      complete,
				Live:          complete && len(liveWants)+len(liveThings) > 0,
				LiveWantIDs:   liveWants,
				LiveThings:    liveThings,
				AlmostThere:   total > 1 && satisfied == total-1,
				Thresholds:    k.Mastery,
				Unlocks:       k.Unlocks,
				Hidden:        k.Hidden,
				Veiled:        k.Veiled,
				Mark:          k.Mark,
			}
		}
	}
	best.Constellations = standing
	best.Suggestions = suggestions

	best.Mastery = mywant.KataMasteryCount(k.ID)
	best.MasteryRank = k.RankFor(best.Mastery)
	best.Earned = earnedUnlocks(k, best.Mastery)
	best.Recorded = best.Mastery > 0
	return best, credited
}

// liveEvidence splits what satisfied a standing into the two kinds of thing that
// can be drawn: wants on the board, and things.
//
// `repeat` 所作 contribute nothing — they are satisfied out of the record book,
// and a record has no position to draw at.
//
// Things come back as "catalogKey::value" so they match the member ids used
// by constellations and by the thing tiles on the canvas.
func liveEvidence(waza []WazaProgress) (wantIDs, memoIDs []string) {
	seenWant := map[string]bool{}
	seenMemo := map[string]bool{}
	for _, wp := range waza {
		if !wp.Satisfied {
			continue
		}
		switch wp.Waza.Kind {
		case "want_type":
			for _, id := range wp.MatchedIDs {
				if !seenWant[id] {
					seenWant[id] = true
					wantIDs = append(wantIDs, id)
				}
			}
		case "thing":
			key := thingCatalogKey(wp.Waza.Subtype)
			for _, v := range wp.MatchedIDs {
				id := key + "::" + v
				if !seenMemo[id] {
					seenMemo[id] = true
					memoIDs = append(memoIDs, id)
				}
			}
		}
	}
	sort.Strings(wantIDs)
	sort.Strings(memoIDs)
	return wantIDs, memoIDs
}

// joinsOneGroup reports whether a kata's 所作 must all resolve inside ONE
// constellation, rather than anywhere on the board.
//
// Two spellings, and the second one is why this function exists. The rename
// that turned every "memo" into a "thing" reached the seeds and not this test,
// so all thirteen joined kata declared `thing_group`, matched nothing, and
// were quietly measured globally — a kata meant to say "these are about the
// same place" was satisfied by any two things anywhere. Both names are
// accepted now rather than the seeds being rewritten to the older one, because
// the seeds have the current vocabulary and this was the straggler.
func joinsOneGroup(kind string) bool {
	return kind == "thing_group" || kind == "memo_group"
}

// thingCatalogKey maps a data type name to the thing.yaml section it is stored
// under ("station" → "stations"), falling back to the naive plural.
func thingCatalogKey(subtype string) string {
	if info, ok := dataTypeDefs[subtype]; ok && info.Key != "" {
		return info.Key
	}
	return subtype + "s"
}

// wantMatchesStatus applies a waza's status filter.
//
//	""/"achieved" — 極まった (with or without warnings), the default
//	"any"         — merely deployed; used for watchers that never complete
//	anything else — an exact status match
func wantMatchesStatus(w *mywant.Want, status string) bool {
	switch status {
	case "", "achieved":
		return mywant.IsAchievedStatus(w.GetStatus())
	case "any":
		return true
	default:
		return string(w.GetStatus()) == status
	}
}

// paramString reads a want parameter as a trimmed string.
func paramString(w *mywant.Want, name string) string {
	v, ok := w.Spec.Params[name]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

// evaluateWaza scores a single 所作. When scope is non-nil the waza is being
// measured inside one scope, and anything joined must resolve there.
func (s *Server) evaluateWaza(
	wz mywant.Waza,
	wantsByType map[string][]*mywant.Want,
	scope *thingScope,
	// What the 所作 before this one settled on, by want type. A wire has two
	// ends, and when a sibling 所作 has already said WHICH want of that type
	// counts here — the route that arrives in this group, not any route — the
	// wire has to run to that one. Empty for the types nothing has pinned yet.
	matchedByType map[string][]string,
) WazaProgress {
	need := wz.Need()
	wp := WazaProgress{Waza: wz, Need: need}

	switch wz.Kind {
	case "want_type":
		// Wants of the right type, in the right place, that fail only the
		// relation. The difference between "there is no alarm" and "the alarm
		// is not wired to anything" is the difference between two pieces of
		// advice, and only one of them is any use.
		related := 0
		for _, w := range wantsByType[wz.Type] {
			if !wantMatchesStatus(w, wz.Status) {
				continue
			}
			// Joined: the want must name a value that belongs to this group.
			if wz.Join != "" && scope != nil {
				if !scope.AllValues[paramString(w, wz.Join)] {
					continue
				}
			}
			// Wired: the want must be FED BY another, not merely accompanied
			// by one. See Waza.ImportFrom.
			if wz.ImportFrom != nil && !s.importsFrom(w, *wz.ImportFrom, wantsByType, matchedByType) {
				related++
				continue
			}
			// Or the other way round: the want must be the PARENT of one, which
			// is how a budget is fed. See Waza.Owns.
			if wz.Owns != nil && !s.owns(w, *wz.Owns, wantsByType, matchedByType) {
				related++
				continue
			}
			wp.MatchedIDs = append(wp.MatchedIDs, w.Metadata.ID)
		}
		wp.Have = len(wp.MatchedIDs)
		if wp.Have < need {
			switch {
			case wz.ImportFrom != nil && related > 0:
				// One is standing there unwired: placing a second never helps.
				wp.Hint = fmt.Sprintf("Wire the %s's %s into the %s",
					wz.ImportFrom.Type, wz.ImportFrom.State, wz.Type)
			case wz.ImportFrom != nil:
				// Nothing of the kind on the board: both halves of the move,
				// because arriving with only the first done is arriving
				// nowhere.
				wp.Hint = fmt.Sprintf("Place a %s and wire the %s's %s into it",
					wz.Type, wz.ImportFrom.Type, wz.ImportFrom.State)
			case wz.Owns != nil && related > 0:
				wp.Hint = fmt.Sprintf("Put the %s under the %s", wz.Owns.Type, wz.Type)
			case wz.Owns != nil:
				wp.Hint = fmt.Sprintf("Place a %s and put the %s under it",
					wz.Type, wz.Owns.Type)
			case wz.Join != "" && scope != nil && scope.Name != "":
				wp.Hint = fmt.Sprintf("Place a %s aimed at %q", wz.Type, scope.Name)
			case wz.Status == "any":
				wp.Hint = "Place a " + wz.Type
			default:
				wp.Hint = "Complete a " + wz.Type
			}
		}

	case "thing":
		if scope != nil {
			// Inside a join, a thing 所作 asks whether the group holds a value of
			// this subtype — that is what makes the group name one thing.
			for v := range scope.BySubtype[wz.Subtype] {
				wp.MatchedIDs = append(wp.MatchedIDs, v)
			}
			sort.Strings(wp.MatchedIDs)
			wp.Have = len(wp.MatchedIDs)
			if wp.Have < need {
				switch {
				case scope.Name == "":
					wp.Hint = "Remember a value first"
				case scope.Lone:
					// The scope is one value standing alone, so what is missing
					// is the declaration that another value means the same place.
					wp.Hint = fmt.Sprintf("Put a %s in one constellation with %q", wz.Subtype, scope.Name)
				default:
					wp.Hint = fmt.Sprintf("Add a %s to %q", wz.Subtype, scope.Name)
				}
			}
			break
		}
		// Ungrouped: a plain count over the whole thing store. Witnesses are the
		// values themselves, so naming one more counts as a fresh practice.
		values := s.thingStore.GetCategory(subtypeToKey(wz.Subtype))
		wp.Have = len(values)
		wp.MatchedIDs = append(wp.MatchedIDs, values...)
		sort.Strings(wp.MatchedIDs)
		if wp.Have < need {
			wp.Hint = fmt.Sprintf("Remember %d more %s", need-wp.Have, wz.Subtype)
		}

	case "repeat":
		n := mywant.KataMasteryCount(wz.Kata)
		wp.Have = n
		if n >= need {
			wp.MatchedIDs = []string{fmt.Sprintf("%s:%d", wz.Kata, n)}
		} else {
			name := wz.Kata
			if other, ok := mywant.GetKata(wz.Kata); ok {
				name = other.Name
			}
			wp.Hint = fmt.Sprintf("Hold %s %d more time(s)", name, need-n)
		}
	}

	wp.Satisfied = wp.Have >= need
	return wp
}

// earnedUnlocks returns the unlock sets already granted at this mastery count,
// shallowest first.
func earnedUnlocks(k mywant.Kata, mastery int) []mywant.KataUnlocks {
	if mastery <= 0 || len(k.Unlocks) == 0 {
		return nil
	}
	var out []mywant.KataUnlocks
	for _, rank := range []string{mywant.MasteryShoden, mywant.MasteryKaiden} {
		threshold := 0
		switch rank {
		case mywant.MasteryShoden:
			threshold = k.Mastery.Shoden
		case mywant.MasteryKaiden:
			threshold = k.Mastery.Kaiden
		}
		if threshold <= 0 || mastery < threshold {
			continue
		}
		if u, ok := k.Unlocks[rank]; ok {
			out = append(out, u)
		}
	}
	return out
}
