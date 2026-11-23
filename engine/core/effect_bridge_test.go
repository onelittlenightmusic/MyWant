package mywant

import "testing"

func TestTriggerWantPlaysAnEffectOverWhoeverSetItOff(t *testing.T) {
	var got [][3]string
	OnBoardEffect = func(on, id, typ string) { got = append(got, [3]string{on, id, typ}) }
	defer func() { OnBoardEffect = nil }()

	fire := &Want{Metadata: Metadata{ID: "f1", Type: "fireworks_effect"}}
	fire.WantTypeDefinition = &WantTypeDefinition{Metadata: WantTypeMetadata{Category: EffectCategory}}
	button := &Want{Metadata: Metadata{ID: "b1", Type: "button"}}
	button.WantTypeDefinition = &WantTypeDefinition{Metadata: WantTypeMetadata{Category: "system"}}

	TriggerWant(fire, map[string]any{"action": "trigger", "characterId": "chr-1"}) // a press
	TriggerWant(fire, map[string]any{"action": "trigger"})                         // a want that heard
	TriggerWant(button, map[string]any{"action": "press", "characterId": "chr-1"}) // no effect

	want := [][3]string{{"character", "chr-1", "fireworks_effect"}, {"want", "f1", "fireworks_effect"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("effects = %v, want %v", got, want)
	}
	if p, _ := fire.GetCurrent("webhook_payload"); p == nil {
		if v, ok := fire.State.Load("webhook_payload"); !ok || v == nil {
			t.Fatal("the trigger did not reach the want's webhook_payload")
		}
	}
}
