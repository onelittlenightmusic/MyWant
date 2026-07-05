package types

import (
	"encoding/json"
	"fmt"
	"strconv"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[ChoiceWant, ChoiceLocals]("choice")
	})
}

// ChoiceLocals holds type-specific local state.
type ChoiceLocals struct{}

// ChoiceWant allows selecting a value from a list and propagating it to a target parameter.
// A selection event is delivered via POST /api/v1/webhooks/{id} with {"action":"select","value":...}.
type ChoiceWant struct {
	Want
}

func (c *ChoiceWant) GetLocals() *ChoiceLocals {
	return CheckLocalsInitialized[ChoiceLocals](&c.Want)
}

func (c *ChoiceWant) Initialize() {
	if def := c.GetStringParam("default", ""); def != "" {
		c.SetCurrent("selected", def)
	}
	c.StoreState("last_action_at", "")
	c.refreshMemoChoices()
}

// IsAchieved always returns false — choice is a persistent control.
func (c *ChoiceWant) IsAchieved() bool { return false }

func (c *ChoiceWant) Progress() {
	c.refreshMemoChoices()
	ConsumeWebhookAction(&c.Want, "last_action_at", func(action string, pm map[string]any) bool {
		if action != "select" {
			return false
		}
		c.SetCurrent("selected", pm["value"])
		return true
	})
}

// ApplyAuraDefault implements types.AuraDefaultApplier: an aura-default mark
// for "selected" must still name one of the want's current options — unlike
// going/switch's plain bool toggle, a stale mark shouldn't silently overwrite
// selected with a value that's no longer offered.
func (c *ChoiceWant) ApplyAuraDefault(section, key, value string) bool {
	if section != "current" || key != "selected" {
		return false
	}
	raw, ok := c.GetCurrent("choices")
	choices, ok2 := raw.([]any)
	if !ok || !ok2 {
		return false
	}
	for _, ch := range choices {
		if choiceValueString(ch) == value {
			c.SetCurrent("selected", ch)
			return true
		}
	}
	return false
}

// choiceValueString mirrors the frontend's getChoiceValue (ChoiceCardPlugin.tsx):
// object choices compare by their JSON form, everything else by its string form.
func choiceValueString(c any) string {
	switch v := c.(type) {
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// refreshMemoChoices overwrites choices with memo values when globalMemoCategory is set.
func (c *ChoiceWant) refreshMemoChoices() {
	cat := c.GetStringParam("globalMemoCategory", "")
	if cat == "" {
		return
	}
	mr := GetGlobalMemoReader()
	if mr == nil {
		return
	}
	vals := mr.GetCategory(cat)
	choices := make([]any, len(vals))
	for i, v := range vals {
		choices[i] = v
	}
	c.SetCurrent("choices", choices)
}
