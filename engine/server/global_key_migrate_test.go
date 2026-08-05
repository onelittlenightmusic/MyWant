package server

import (
	"testing"

	mywant "mywant/engine/core"
)

func want(name string, spec mywant.WantSpec) *mywant.Want {
	w := &mywant.Want{}
	w.Metadata.Name = name
	w.Spec = spec
	return w
}

// The whole point: an expose and everything reading it move together, and only
// keys the server itself generated move at all.
func TestMigrateGlobalKeys(t *testing.T) {
	source := want("spotify instance", mywant.WantSpec{
		Exposes: []mywant.ExposeEntry{
			// Server-generated: slug + "_" + field.
			{CurrentState: "album_art_url", As: "spotify_instance_album_art_url"},
			// Hand-written: nothing to reconstruct it from, so it stays.
			{CurrentState: "lat", As: "device_lat"},
		},
	})
	consumer := want("dynamic background", mywant.WantSpec{
		Imports: map[string]string{
			"spotify_instance_album_art_url": "source_image_url",
			"device_lat":                     "lat",
		},
		Params: map[string]any{
			"tint":   map[string]any{"fromGlobalParam": "spotify_instance_album_art_url"},
			"anchor": map[any]any{"fromGlobalParam": "device_lat"},
			"plain":  "left alone",
		},
		When: []mywant.WhenSpec{{FromGlobalParam: "spotify_instance_album_art_url"}},
	})
	consumer.Metadata.Correlation = []mywant.CorrelationEntry{{
		WantID: "src",
		Labels: []string{
			"stateAccess/provider:expose/spotify_instance_album_art_url",
			"stateAccess/provider:expose/device_lat",
			"using.select/role=x",
		},
		Rate: 1,
	}}

	if n := migrateGlobalKeys([]*mywant.Want{source, consumer}, globalKeyRenames([]*mywant.Want{source, consumer})); n != 2 {
		t.Fatalf("expected both wants touched, got %d", n)
	}

	if got := source.Spec.Exposes[0].As; got != "spotify_instance.album_art_url" {
		t.Errorf("expose not renamed: %q", got)
	}
	if got := source.Spec.Exposes[1].As; got != "device_lat" {
		t.Errorf("hand-written key was rewritten: %q", got)
	}
	if _, ok := consumer.Spec.Imports["spotify_instance.album_art_url"]; !ok {
		t.Errorf("import not renamed: %v", consumer.Spec.Imports)
	}
	if _, ok := consumer.Spec.Imports["spotify_instance_album_art_url"]; ok {
		t.Errorf("old import key survived: %v", consumer.Spec.Imports)
	}
	if consumer.Spec.Imports["device_lat"] != "lat" {
		t.Errorf("hand-written import was disturbed: %v", consumer.Spec.Imports)
	}
	if got := paramRef(t, consumer.Spec.Params["tint"]); got != "spotify_instance.album_art_url" {
		t.Errorf("param ref not renamed: %q", got)
	}
	if got := paramRef(t, consumer.Spec.Params["anchor"]); got != "device_lat" {
		t.Errorf("hand-written param ref was rewritten: %q", got)
	}
	if consumer.Spec.Params["plain"] != "left alone" {
		t.Errorf("a plain param was touched: %v", consumer.Spec.Params["plain"])
	}
	if got := consumer.Spec.When[0].FromGlobalParam; got != "spotify_instance.album_art_url" {
		t.Errorf("when clause not renamed: %q", got)
	}
	labels := consumer.Metadata.Correlation[0].Labels
	if labels[0] != "stateAccess/provider:expose/spotify_instance.album_art_url" {
		t.Errorf("correlation label not renamed: %q", labels[0])
	}
	if labels[1] != "stateAccess/provider:expose/device_lat" {
		t.Errorf("hand-written key renamed in a label: %q", labels[1])
	}
	if labels[2] != "using.select/role=x" {
		t.Errorf("an unrelated label was touched: %q", labels[2])
	}
}

// Running it again must find nothing left to do — the keys are dotted now, so
// they no longer reconstruct to the underscore form.
func TestMigrateGlobalKeysIsIdempotent(t *testing.T) {
	w := want("spotify instance", mywant.WantSpec{
		Exposes: []mywant.ExposeEntry{{CurrentState: "album_art_url", As: "spotify_instance_album_art_url"}},
	})
	migrateGlobalKeys([]*mywant.Want{w}, globalKeyRenames([]*mywant.Want{w}))
	if n := migrateGlobalKeys([]*mywant.Want{w}, globalKeyRenames([]*mywant.Want{w})); n != 0 {
		t.Fatalf("second run renamed %d want(s); it should have found nothing", n)
	}
	if got := w.Spec.Exposes[0].As; got != "spotify_instance.album_art_url" {
		t.Errorf("key drifted on the second run: %q", got)
	}
}

func paramRef(t *testing.T, v any) string {
	t.Helper()
	switch m := v.(type) {
	case map[string]any:
		s, _ := m["fromGlobalParam"].(string)
		return s
	case map[any]any:
		s, _ := m["fromGlobalParam"].(string)
		return s
	}
	t.Fatalf("not a param ref: %#v", v)
	return ""
}
