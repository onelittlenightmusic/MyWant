package mywant

import (
	"encoding/json"
	"testing"
)

func TestCharacterDesignMovesToLabels(t *testing.T) {
	c := Character{ID: "c", TileDesign: "forest", Labels: map[string]string{LabelAuraDesign: "sky"}, AuraDesign: "cubic"}
	c.moveDesignToLabels()
	if c.TileDesign != "" || c.AuraDesign != "" {
		t.Fatalf("old fields kept: %+v", c)
	}
	if c.Labels[LabelTileDesign] != "forest" || c.Labels[LabelAuraDesign] != "sky" {
		t.Fatalf("labels = %v, want tile=forest and the existing aura label kept", c.Labels)
	}
	b, _ := json.Marshal(c)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	if out["tile_design"] != "forest" || out["aura_design"] != "sky" {
		t.Fatalf("JSON lost the old shape: %s", b)
	}
}

func TestWithDesignLabelsEmptyInherits(t *testing.T) {
	orig := map[string]string{LabelTileDesign: "forest", "other": "x"}
	got := withDesignLabels(orig, "", "sky")
	if _, ok := got[LabelTileDesign]; ok || got[LabelAuraDesign] != "sky" || got["other"] != "x" {
		t.Fatalf("withDesignLabels = %v", got)
	}
	if orig[LabelTileDesign] != "forest" {
		t.Fatal("withDesignLabels wrote into the caller's map")
	}
}
