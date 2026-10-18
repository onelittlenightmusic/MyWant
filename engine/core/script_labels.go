package mywant

import (
	"fmt"
	"strings"
)

// scriptLabelPrefix is the only kind of label a script may write: how its
// want's tile is drawn (tile-effect-colors, tile-readout, …).
//
// A script reports what it found, and some of what it finds is how its want
// should look — a transit search that rides two lines knows the two colours
// its tile's tie should be. That much it may say. A label that places the
// tile, names its owner or wires it to another want is not a script's to
// change, so anything outside the prefix is refused rather than trusted.
const scriptLabelPrefix = "tile-"

// applyLabelUpdates writes a script's `label_updates` into its want's labels.
//
//	{"label_updates": {"tile-effect-colors": ["#9ACD32", "#F15A22"]}}
//
// A value is a string, or a list joined with commas (labels hold strings, and
// the board reads lists that way). An empty string or null takes the label
// away. A label already holding the value is left alone, so a monitor that
// finds the same lines every tick does not announce a change every tick.
func applyLabelUpdates(want *Want, raw map[string]any, agentLabel string) {
	updates, _ := raw["label_updates"].(map[string]any)
	for k, v := range updates {
		if !strings.HasPrefix(k, scriptLabelPrefix) {
			want.StoreLog("[%s] WARN: label_updates key %q is not a %s label; skipped", agentLabel, k, scriptLabelPrefix)
			continue
		}
		value, ok := scriptLabelValue(v)
		if !ok {
			want.StoreLog("[%s] WARN: label_updates %q is not a string or a list; skipped", agentLabel, k)
			continue
		}
		current, has := want.labelSet().Get(k)
		if value == "" {
			if has {
				want.DeleteLabel(k)
			}
			continue
		}
		if has && current == value {
			continue
		}
		want.SetLabel(k, value)
		want.StoreLog("[%s] label %q = %q", agentLabel, k, value)
	}
}

func scriptLabelValue(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", true
	case string:
		return strings.TrimSpace(t), true
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			s := strings.TrimSpace(fmt.Sprint(item))
			if item != nil && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ","), true
	}
	return "", false
}
