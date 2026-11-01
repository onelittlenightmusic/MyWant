package mywant

import "testing"

// A field a type's new version adds reaches a want of it already running:
// its label known, its fetchFrom filled from the source the want already has,
// and what the want had kept as it was (fly, 2026-11-01: goto_template).
func TestRefreshWantTypeDefinitionReachesRunningWants(t *testing.T) {
	old := &WantTypeDefinition{}
	old.Metadata.Name = "list"
	old.State = []StateDef{
		{Name: "times", Label: "current", FetchFrom: "raw", OnFetchData: "times"},
	}
	w := &Want{Metadata: Metadata{ID: "want-1", Name: "w1", Type: "list"}}
	cb := newUpdateWantTestBuilder(w)
	live := cb.wants["want-1"].want
	live.SetWantTypeDefinition(old)
	live.StoreState("raw", map[string]any{"times": []any{"20:00"}, "template": map[string]any{"v": "{{v}}"}})
	live.BeginProgressCycle()
	live.EndProgressCycle()
	if _, ok := live.GetCurrent("times"); !ok {
		t.Fatal("times not filled under the old definition")
	}

	newer := &WantTypeDefinition{}
	newer.Metadata.Name = "list"
	newer.State = append(append([]StateDef(nil), old.State...),
		StateDef{Name: "template", Label: "current", FetchFrom: "raw", OnFetchData: "template", InitialValue: map[string]any{}})
	if n := cb.RefreshWantTypeDefinition(newer); n != 1 {
		t.Fatalf("refreshed %d wants, want 1", n)
	}
	live.BeginProgressCycle()
	live.EndProgressCycle()

	got, ok := live.GetCurrent("template")
	if !ok {
		t.Fatal("the new field is not current-labeled on the running want")
	}
	if m, _ := got.(map[string]any); m["v"] != "{{v}}" {
		t.Fatalf("the new field was not filled from the source: %v", got)
	}
	if v, _ := live.GetCurrent("times"); v == nil {
		t.Fatal("what the want had was lost")
	}
	if raw, _ := live.getState("raw"); raw == nil {
		t.Fatal("the source was reset")
	}
}
