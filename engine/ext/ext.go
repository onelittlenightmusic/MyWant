// Package ext is the open slot a GUI extension keeps its own values in.
//
// The config, the gui_state want and a character's display each have one
// field named `ext`. What goes under it is the extension's business: the
// engine stores it, merges it and hands it back, and never reads a key inside
// it. That is the point — an extension (the canvas, today) can add a setting
// without anybody changing mywant, and the keys it adds are kept apart from
// the ones the engine does interpret.
//
// By convention the first level is the extension's name, so two of them never
// meet: ext.canvas.dpad, ext.canvas.<character>.scale.
//
// Writes are JSON Merge Patch (RFC 7396): a patch names only what changes, a
// nested object is merged rather than replaced, and null removes a key. Two
// windows each writing their own corner of ext.canvas therefore do not
// overwrite each other, which a top-level replace of `ext` would.
package ext

import "maps"

// MergePatch applies patch to target and returns the result. target is not
// modified; nested maps along the patched paths are copied first.
//
// A patch that is not an object replaces target outright, as the RFC says.
func MergePatch(target any, patch any) any {
	p, ok := patch.(map[string]any)
	if !ok {
		return clone(patch)
	}
	t, _ := target.(map[string]any)
	out := make(map[string]any, len(t)+len(p))
	maps.Copy(out, t)
	for k, v := range p {
		if v == nil {
			delete(out, k)
			continue
		}
		out[k] = MergePatch(out[k], v)
	}
	return out
}

// Merge is MergePatch for the common case of two objects, returning an object
// (empty rather than nil, so callers can store it as is).
func Merge(target, patch map[string]any) map[string]any {
	m, _ := MergePatch(target, patch).(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

// Get reads the value at a path ("canvas", "dpad"), or nil when any step is
// missing or not an object.
func Get(m map[string]any, path ...string) any {
	var cur any = m
	for _, k := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[k]
	}
	return cur
}

// Set returns a copy of m with value at path, creating objects on the way.
// Setting nil removes the key.
func Set(m map[string]any, value any, path ...string) map[string]any {
	if len(path) == 0 {
		return Clone(m)
	}
	patch := map[string]any{}
	cur := patch
	for _, k := range path[:len(path)-1] {
		next := map[string]any{}
		cur[k] = next
		cur = next
	}
	cur[path[len(path)-1]] = value
	return Merge(m, patch)
}

// Clone deep-copies an ext value, so a stored one is never shared with a
// caller who might write to it.
func Clone(m map[string]any) map[string]any {
	c, _ := clone(m).(map[string]any)
	if c == nil {
		c = map[string]any{}
	}
	return c
}

func clone(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = clone(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = clone(e)
		}
		return out
	default:
		return v
	}
}
