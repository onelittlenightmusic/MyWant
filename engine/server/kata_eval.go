package server

import (
	"fmt"
	"sort"
	"strings"

	mywant "mywant/engine/core"
)

// Kata evaluation.
//
// Progress is computed on demand from live data — deployed wants, the memo store
// and past practice records — so "どこが足りないか" is always current without a
// background loop. The only thing written back is a practice record, the moment a
// kata is found 極まった for a set of witnesses it has not been credited for yet.
//
// A kata that declares a join is evaluated once per memo group: its 所作 must all
// resolve inside the SAME group, which is how two wants are known to be about the
// same place. Nothing here looks at time or order — a kata is a set of wants that
// belong together, never a schedule.

// WazaProgress is one 所作 with its current standing.
type WazaProgress struct {
	Waza      mywant.Waza `json:"waza"`
	Satisfied bool        `json:"satisfied"`
	Have      int         `json:"have"`
	Need      int         `json:"need"`
	// MatchedIDs are the want IDs (or memo values) that satisfied this waza.
	MatchedIDs []string `json:"matchedIDs,omitempty"`
	// Hint is what the user would have to do to satisfy it, in one short phrase.
	Hint string `json:"hint,omitempty"`
}

// KataProgress is a kata's standing: how far along, how deep, and whether it is
// even visible yet.
type KataProgress struct {
	KataID    string   `json:"kataID"`
	Name      string   `json:"name"`
	Reading   string   `json:"reading,omitempty"`
	Level     string   `json:"level"`
	Intent    string   `json:"intent,omitempty"`
	Yields    string   `json:"yields,omitempty"`
	Contains  []string `json:"contains,omitempty"`
	Variation string   `json:"variation,omitempty"`
	// Group is the memo group this standing was measured against — the shared
	// thing the 所作 are all about. Empty for kata that declare no join.
	Group string `json:"group,omitempty"`

	Waza      []WazaProgress `json:"waza"`
	Satisfied int            `json:"satisfied"`
	Total     int            `json:"total"`
	Complete  bool           `json:"complete"`
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

// memoGroup is one constellation of memo values, indexed for join lookups.
type memoGroup struct {
	Name string
	// Values that belong to the group, keyed by subtype.
	BySubtype map[string]map[string]bool
	// Every value in the group regardless of subtype.
	AllValues map[string]bool
}

// has reports whether the group holds a value of this subtype.
func (g *memoGroup) has(subtype string) bool { return len(g.BySubtype[subtype]) > 0 }

// collectKataGroups turns constellation membership into join-ready indexes.
// Members arrive as "catalogKey::value" (e.g. "stations::<name>"), so the catalog
// key is mapped back to its data type name.
func (s *Server) collectKataGroups() []memoGroup {
	keyToSubtype := make(map[string]string, len(dataTypeDefs))
	for name, info := range dataTypeDefs {
		// First declaration wins: several subtypes share a key (int/integer),
		// and either name resolves the same values.
		if _, seen := keyToSubtype[info.Key]; !seen {
			keyToSubtype[info.Key] = name
		}
	}

	var out []memoGroup
	for _, g := range s.collectMemoConstellations() {
		mg := memoGroup{
			Name:      g.Name,
			BySubtype: map[string]map[string]bool{},
			AllValues: map[string]bool{},
		}
		for _, member := range g.Members {
			key, value, ok := strings.Cut(member, "::")
			if !ok {
				continue
			}
			subtype := keyToSubtype[key]
			if subtype == "" {
				subtype = strings.TrimSuffix(key, "s")
			}
			if mg.BySubtype[subtype] == nil {
				mg.BySubtype[subtype] = map[string]bool{}
			}
			mg.BySubtype[subtype][value] = true
			mg.AllValues[value] = true
		}
		out = append(out, mg)
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
	groups := s.collectKataGroups()

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
			p.Group = ""
			p.Unlocks = nil
			// The name and its reading name the form as surely as its 所作 do,
			// so they are withheld too — only the ID and the count of 所作 ship.
			p.Name = ""
			p.Reading = ""
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

// evaluateOneKata scores every variation, against every memo group when the kata
// declares a join, and returns the closest standing.
// Practice is only credited when the kata's belt is open.
func (s *Server) evaluateOneKata(
	k mywant.Kata,
	wantsByType map[string][]*mywant.Want,
	groups []memoGroup,
	beltOpen bool,
) (KataProgress, bool) {
	var best KataProgress
	bestScore := -1
	credited := false

	// A joined kata is measured once per group; an unjoined one once, globally.
	scopes := []*memoGroup{nil}
	if k.Join.Kind == "memo_group" {
		scopes = nil
		for i := range groups {
			scopes = append(scopes, &groups[i])
		}
		if len(scopes) == 0 {
			// No groups yet: still report standing so the card can say so.
			scopes = []*memoGroup{{Name: "", BySubtype: map[string]map[string]bool{}, AllValues: map[string]bool{}}}
		}
	}

	for _, henka := range k.Variations() {
		for _, scope := range scopes {
			wazaProgress := make([]WazaProgress, 0, len(henka.Waza))
			satisfied := 0
			witnesses := make([]string, 0, 8)

			for _, wz := range henka.Waza {
				wp := s.evaluateWaza(wz, wantsByType, scope)
				if wp.Satisfied {
					satisfied++
					witnesses = append(witnesses, wp.MatchedIDs...)
				}
				wazaProgress = append(wazaProgress, wp)
			}

			if satisfied <= bestScore {
				continue
			}
			bestScore = satisfied

			groupName := ""
			if scope != nil {
				groupName = scope.Name
			}
			total := len(henka.Waza)
			complete := total > 0 && satisfied == total
			best = KataProgress{
				KataID:      k.ID,
				Name:        k.Name,
				Reading:     k.Reading,
				Level:       k.Level,
				Intent:      k.Intent,
				Yields:      k.Yields,
				Contains:    k.Contains,
				Variation:   henka.ID,
				Group:       groupName,
				Waza:        wazaProgress,
				Satisfied:   satisfied,
				Total:       total,
				Complete:    complete,
				AlmostThere: total > 1 && satisfied == total-1,
				Thresholds:  k.Mastery,
				Unlocks:     k.Unlocks,
				Hidden:      k.Hidden,
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
		}
	}

	best.Mastery = mywant.KataMasteryCount(k.ID)
	best.MasteryRank = k.RankFor(best.Mastery)
	best.Earned = earnedUnlocks(k, best.Mastery)
	return best, credited
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
// measured inside one memo group, and anything joined must resolve there.
func (s *Server) evaluateWaza(
	wz mywant.Waza,
	wantsByType map[string][]*mywant.Want,
	scope *memoGroup,
) WazaProgress {
	need := wz.Need()
	wp := WazaProgress{Waza: wz, Need: need}

	switch wz.Kind {
	case "want_type":
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
			wp.MatchedIDs = append(wp.MatchedIDs, w.Metadata.ID)
		}
		wp.Have = len(wp.MatchedIDs)
		if wp.Have < need {
			switch {
			case wz.Join != "" && scope != nil && scope.Name != "":
				wp.Hint = fmt.Sprintf("Place a %s aimed at %q", wz.Type, scope.Name)
			case wz.Status == "any":
				wp.Hint = "Place a " + wz.Type
			default:
				wp.Hint = "Complete a " + wz.Type
			}
		}

	case "memo":
		if scope != nil {
			// Inside a join, a memo 所作 asks whether the group holds a value of
			// this subtype — that is what makes the group name one thing.
			for v := range scope.BySubtype[wz.Subtype] {
				wp.MatchedIDs = append(wp.MatchedIDs, v)
			}
			sort.Strings(wp.MatchedIDs)
			wp.Have = len(wp.MatchedIDs)
			if wp.Have < need {
				if scope.Name == "" {
					wp.Hint = "Create a memo group"
				} else {
					wp.Hint = fmt.Sprintf("Add a %s to %q", wz.Subtype, scope.Name)
				}
			}
			break
		}
		// Ungrouped: a plain count over the whole memo store. Witnesses are the
		// values themselves, so naming one more counts as a fresh practice.
		values := s.memoStore.GetCategory(subtypeToKey(wz.Subtype))
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
