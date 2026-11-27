package server

import (
	"testing"

	mywant "mywant/engine/core"

	ws "github.com/onelittlenightmusic/want-spec"
)

func TestCharacterTimingText(t *testing.T) {
	std := ws.CharacterStandardTimings()
	state := map[string]any{
		ws.CharacterStateErrandReport: "Heroさん、やっぱり違いました。",
		ws.CharacterStateHeard:        "雨になりました",
	}
	if got := characterTimingText(std[0], state); got != "Heroさん、やっぱり違いました。" {
		t.Fatalf("errand: %q", got)
	}
	if got := characterTimingText(std[1], state); got != "気になることができた：「雨になりました」" {
		t.Fatalf("uneasy: %q", got)
	}
	state[ws.CharacterStatePlace] = "戸塚の天気"
	if got := characterTimingText(ws.TimingDef{Name: "arrived", Description: "別の場所に来た", OnChange: ws.CharacterStatePlace}, state); got != "別の場所に来た：戸塚の天気" {
		t.Fatalf("arrived: %q", got)
	}
}

func TestCharacterDocumentIsAKind(t *testing.T) {
	doc := characterDocument(characterForTest())
	if doc.Kind != ws.CharacterKind || doc.APIVersion != "mywant.io/v1" || doc.Metadata.ID != "chr-1" || doc.Spec.Avatar != "🦊" {
		t.Fatalf("%+v", doc)
	}
}

func characterForTest() (c mywant.Character) {
	c.ID, c.Name, c.Avatar = "chr-1", "aaa", "🦊"
	return c
}
