package mywant

import "time"

// A character's ask ledger: the places each want it asked for named, and when
// that want first answered — what its home is learned from
// (engine/server/character_home.go).
//
// Kept in characters.yaml under `homes`, beside the characters rather than in
// them, and not in the wants: a want's fields are what reconcile reads, and a
// tally kept there would set it going. One entry per want, written when the
// want first answers, so the file is written once per ask, not per answer;
// still there after the want is deleted, gone with the character.

// AskPlace is one place a want named: its parameter, the value, and the
// value's kind (the parameter's subtype, e.g. station).
type AskPlace struct {
	Param string `yaml:"param" json:"param"`
	Value string `yaml:"value" json:"value"`
	Kind  string `yaml:"kind"  json:"kind"`
}

// CharacterAsk is one want a character asked for.
type CharacterAsk struct {
	Want   string     `yaml:"want"   json:"want"` // the want's id: an ask is counted once
	Type   string     `yaml:"type"   json:"type"`
	At     time.Time  `yaml:"at"     json:"at"` // when it first answered
	Places []AskPlace `yaml:"places" json:"places"`
}

// characterAsksKept is how many asks a character's ledger keeps, newest last:
// a habit is what is done now, not what was done a year ago.
const characterAsksKept = 200

// RecordAsk adds an ask to the character's ledger: false, and nothing written,
// when the character is unknown or the want is already in it.
func (m *characterManager) RecordAsk(characterID string, ask CharacterAsk) bool {
	if characterID == "" || ask.Want == "" || len(ask.Places) == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	known := false
	for _, c := range m.store.Characters {
		if c.ID == characterID {
			known = true
			break
		}
	}
	if !known {
		return false
	}
	asks := m.store.Homes[characterID]
	for _, a := range asks {
		if a.Want == ask.Want {
			return false
		}
	}
	asks = append(asks, ask)
	if len(asks) > characterAsksKept {
		asks = asks[len(asks)-characterAsksKept:]
	}
	if m.store.Homes == nil {
		m.store.Homes = map[string][]CharacterAsk{}
	}
	m.store.Homes[characterID] = asks
	m.homesVersion++
	m.save()
	return true
}

// HasAsk is whether the want is in the character's ledger already.
func (m *characterManager) HasAsk(characterID, wantID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.store.Homes[characterID] {
		if a.Want == wantID {
			return true
		}
	}
	return false
}

// Asks is a copy of the character's ledger, and its version: it changes
// whenever any ledger does, for whoever keeps what was worked out from it.
func (m *characterManager) Asks(characterID string) ([]CharacterAsk, uint64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]CharacterAsk(nil), m.store.Homes[characterID]...), m.homesVersion
}

func RecordCharacterAsk(characterID string, ask CharacterAsk) bool {
	return GetCharacterManager().RecordAsk(characterID, ask)
}
func CharacterHasAsk(characterID, wantID string) bool {
	return GetCharacterManager().HasAsk(characterID, wantID)
}
func CharacterAsks(characterID string) ([]CharacterAsk, uint64) {
	return GetCharacterManager().Asks(characterID)
}
