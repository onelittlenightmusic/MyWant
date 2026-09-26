package ext

import (
	"reflect"
	"testing"
)

func TestMergePatchKeepsSiblings(t *testing.T) {
	target := map[string]any{"canvas": map[string]any{
		"chr-a": map[string]any{"scale": 1.0, "center_x": 3.0},
		"chr-b": map[string]any{"scale": 0.5},
	}}
	got := Merge(target, map[string]any{"canvas": map[string]any{
		"chr-a": map[string]any{"scale": 2.0},
	}})
	want := map[string]any{"canvas": map[string]any{
		"chr-a": map[string]any{"scale": 2.0, "center_x": 3.0},
		"chr-b": map[string]any{"scale": 0.5},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge lost a sibling:\n got %v\nwant %v", got, want)
	}
	if target["canvas"].(map[string]any)["chr-a"].(map[string]any)["scale"] != 1.0 {
		t.Fatal("MergePatch wrote through to target")
	}
}

func TestMergePatchNullRemoves(t *testing.T) {
	got := Merge(map[string]any{"canvas": map[string]any{"dpad": true, "x": 1.0}},
		map[string]any{"canvas": map[string]any{"dpad": nil}})
	if !reflect.DeepEqual(got, map[string]any{"canvas": map[string]any{"x": 1.0}}) {
		t.Fatalf("null did not remove: %v", got)
	}
}

func TestSetGet(t *testing.T) {
	m := Set(nil, true, "canvas", "dpad")
	if Get(m, "canvas", "dpad") != true {
		t.Fatalf("Get after Set = %v", Get(m, "canvas", "dpad"))
	}
	if Get(m, "canvas", "dpad", "deeper") != nil || Get(m, "nope") != nil {
		t.Fatal("Get through a non-object should be nil")
	}
	m2 := Set(m, nil, "canvas", "dpad")
	if Get(m2, "canvas", "dpad") != nil || Get(m, "canvas", "dpad") != true {
		t.Fatal("Set nil should remove, on a copy")
	}
}
