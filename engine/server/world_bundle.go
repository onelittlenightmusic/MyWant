package server

import (
	"os"

	mywant "mywant/engine/core"

	"gopkg.in/yaml.v3"
)

// ── A world, as one document ─────────────────────────────────────────────────
//
// On disk a world is several files: <name>.yaml for its wants, and its things
// and their labels beside it in things/ (see world_things.go, which explains
// why they are apart — a thing is edited far more often than a world is
// switched, and they should not share a file).
//
// Leaving is different from living. A world that travels has to arrive whole,
// and a download of several files is not a thing a person can hand to someone
// else. So export folds them into one document and import unfolds it again.
// The split is a storage decision; it stops at the door.

const worldBundleVersion = 1

// worldBundle is that document. Field names are the ones already used on the
// wire and on disk, so a bundle can be read by eye.
type worldBundle struct {
	Version     int                          `yaml:"version"`
	Wants       []*mywant.Want               `yaml:"wants"`
	Things      []ThingEntry                 `yaml:"things,omitempty"`
	ThingLabels map[string]map[string]string `yaml:"thing_labels,omitempty"`
	// Where the characters were standing, and the rest of what the GUI keeps
	// per board. Optional: a bundle written before this existed simply has
	// none, and is still a whole world.
	GUIState map[string]any `yaml:"gui_state,omitempty"`
}

// readWorldBundle gathers a world off disk into one document.
//
// Only the wants are required: a world with nothing in it yet is still a world,
// and a missing things file means exactly that rather than an error.
func readWorldBundle(dir, name string) (*worldBundle, error) {
	wantsData, err := os.ReadFile(worldFilePath(dir, name))
	if err != nil {
		return nil, err
	}
	var wants []*mywant.Want
	if err := yaml.Unmarshal(wantsData, &wants); err != nil {
		return nil, err
	}

	bundle := &worldBundle{Version: worldBundleVersion, Wants: wants}

	if data, err := os.ReadFile(worldThingsPath(dir, name)); err == nil {
		var file thingFile
		if err := yaml.Unmarshal(data, &file); err == nil {
			bundle.Things = file.Things
		}
	}
	if data, err := os.ReadFile(worldThingLabelsPath(dir, name)); err == nil {
		labels := make(map[string]map[string]string)
		if err := yaml.Unmarshal(data, &labels); err == nil && len(labels) > 0 {
			bundle.ThingLabels = labels
		}
	}
	bundle.GUIState = readWorldGUIState(dir, name)
	return bundle, nil
}

// parseWorldUpload reads an uploaded world, in either shape it can arrive in.
//
// A bundle is a mapping; the older export — and any hand-written wants file, of
// which there are plenty in examples/ and in people's notes — is a bare
// sequence of wants. Told apart by what the YAML actually is rather than by
// looking for a version field, because the legacy form has no field to look at.
//
// `bundled` says which shape arrived, which decides how much of the target
// world the caller is entitled to replace: a bundle describes a whole world,
// a bare list says nothing at all about things and must not be read as saying
// there are none.
func parseWorldUpload(data []byte) (bundle *worldBundle, bundled bool, err error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, false, err
	}
	// An empty document unmarshals to a zero Node with no content.
	kind := yaml.Kind(0)
	if len(doc.Content) > 0 {
		kind = doc.Content[0].Kind
	}

	if kind == yaml.MappingNode {
		var b worldBundle
		if err := yaml.Unmarshal(data, &b); err != nil {
			return nil, false, err
		}
		return &b, true, nil
	}

	var wants []*mywant.Want
	if err := yaml.Unmarshal(data, &wants); err != nil {
		return nil, false, err
	}
	return &worldBundle{Version: worldBundleVersion, Wants: wants}, false, nil
}

// writeWorldBundle lays a bundle back out as the files a world is made of.
//
// The things are written only for an upload that was a bundle. A bare wants
// list is silent about them, and silence is not the same as "this world has
// none" — overwriting a world with a wants-only file leaves its things where
// they were. A bundle that carries none, on the other hand, is saying so, and
// the files are cleared to match.
func writeWorldBundle(dir, name string, b *worldBundle, bundled bool) error {
	wantsData, err := yaml.Marshal(b.Wants)
	if err != nil {
		return err
	}
	if err := os.WriteFile(worldFilePath(dir, name), wantsData, 0o644); err != nil {
		return err
	}
	if !bundled {
		return nil
	}

	if err := os.MkdirAll(worldThingsDir(dir), 0o755); err != nil {
		return err
	}
	thingsData, err := yaml.Marshal(thingFile{Version: thingSchemaVersion, Things: b.Things})
	if err != nil {
		return err
	}
	if err := os.WriteFile(worldThingsPath(dir, name), thingsData, 0o644); err != nil {
		return err
	}
	labels := b.ThingLabels
	if labels == nil {
		labels = map[string]map[string]string{}
	}
	labelsData, err := yaml.Marshal(labels)
	if err != nil {
		return err
	}
	if err := os.WriteFile(worldThingLabelsPath(dir, name), labelsData, 0o644); err != nil {
		return err
	}

	// Unlike the things, an absent gui_state is left absent rather than
	// written empty: a bundle carrying none is a world nobody has stood on
	// yet, and clearing the file would be claiming the opposite.
	if len(b.GUIState) == 0 {
		return nil
	}
	if err := os.MkdirAll(worldGUIStateDir(dir), 0o755); err != nil {
		return err
	}
	guiData, err := yaml.Marshal(b.GUIState)
	if err != nil {
		return err
	}
	return os.WriteFile(worldGUIStatePath(dir, name), guiData, 0o644)
}
