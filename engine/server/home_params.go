package server

// What a request leaves out, filled with where the person who asked goes.
//
// 「天気」 said in the evening is the weather at home — the character's home
// at this part of the day (character_home.go): the place they ask for most
// then, in the type's own words (weather's `at` takes nakano where transit's
// `to` takes 中野坂上). A place the request said is never changed; one it did
// not is filled only when the character has a home then and the type takes it.
//
// Only a parameter that is not required: a required one left out means the
// request was not this type's after all (「国分寺はどこ？」 read as a route
// has no `to`), and filling it with home would make a want nobody asked for.
// Nor the type's here-param, which is where the person is now.

import (
	"fmt"
	"strings"
	"time"
)

// robotHomeParams fills with the asker's home what they did not say, and says
// what it filled, for the robot to tell; "" when nothing.
func (s *Server) robotHomeParams(typ string, params map[string]any, who string) string {
	return s.robotHomeParamsAt(typ, params, who, personNow())
}

// robotHomeParamsAt is robotHomeParams at a time of day: an ask tested as it
// was said (laya_asks_check.go).
func (s *Server) robotHomeParamsAt(typ string, params map[string]any, who string, at time.Time) string {
	who = strings.TrimSpace(who)
	if who == "" {
		return ""
	}
	def := s.typeDefinition(typ)
	if def == nil {
		return ""
	}
	home := s.characterHomeAt(who, []string{typ}, at)
	word, ok := home.Says[typ]
	if !ok || word.Param == "" || word.Value == "" || word.Param == def.Metadata.Labels[robotHereLabel] {
		return ""
	}
	if v, said := params[word.Param]; said && v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" {
		return ""
	}
	for _, p := range def.Parameters {
		if p.Name == word.Param && p.Required {
			return ""
		}
	}
	params[word.Param] = word.Value
	return fmt.Sprintf(" (%s=%s: where the person goes at this part of the day (%s), as none was said)", word.Param, word.Value, home.Part)
}

// turnAsker is the character that asked a turn: the one it says, else the
// device's, else the one who just said its words (story.go).
func (s *Server) turnAsker(id string) string {
	turnsMu.Lock()
	var found *robotTurn
	if robot, err := s.robotWantNow(); err == nil {
		for _, t := range robotLoadTurns(robot) {
			if t.ID == id {
				t := t
				found = &t
				break
			}
		}
	}
	turnsMu.Unlock()
	if found == nil {
		return ""
	}
	return story.characterOf(found.Character, found.DeviceID, found.Question, storyDeviceCharacter)
}

// askerOf is who asked what was said, with no turn: the character given, else
// the one who just said these words.
func askerOf(character, said string) string {
	if c := strings.TrimSpace(character); c != "" {
		return c
	}
	return story.characterOf("", "", said, storyDeviceCharacter)
}
