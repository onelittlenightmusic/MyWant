# gopkg.in/yaml.v3 v3.0.1 — patched copy (temporary)

**This copy is temporary. Once `go.yaml.in/yaml/v4` is released as a stable
version with the fix below, move to it and delete this directory and the
`replace` lines in `engine/go.mod` and `client/go.mod`.**

## Why

yaml.v3's emitter never gives back the slots of events it has written
(`yaml_emitter_emit` in `emitterc.go`): the event queue grows by one slot per
event for the whole document. Marshalling the 4.7MB state.yaml grew the heap
by 255MB (423MB allocated) per write — on fly's 512MB VM this took the backend
down (2026-11-01). With the patch: 23MB (40MB allocated). Output unchanged
(byte-identical on the real state.yaml).

## The patch

Four lines at the end of `yaml_emitter_emit` (marked "mywant patch"), plus
`event_queue_mywant_test.go`. Identical to upstream PR
https://github.com/yaml/go-yaml/pull/432 ("Reuse the emitter event queue once
it drains", open as of 2026-11-01, targeting v4 `main`). Upstream keeps v3
frozen, so the fix will not come to v3; go.yaml.in/yaml/v3 v3.0.6 still has
the bug, and v4.0.0-rc.7 too.

## Moving to v4 (when #432 is merged and v4 is stable)

1. Check the release notes / `internal/libyaml/emitter.go` that the queue reset
   is in.
2. `github.com/onelittlenightmusic/want-spec` first: its `WantSpec` implements
   `UnmarshalYAML(*gopkg.in/yaml.v3.Node)`, which v4 refuses to call
   ("v4 cannot invoke v3 Node hooks"). Move it to v4's Node and tag it.
3. Here: replace the import `gopkg.in/yaml.v3` with `go.yaml.in/yaml/v4`
   (46 files; tried on v4.0.0-rc.7 — builds, output identical, only the
   want-spec hook failed), bump want-spec, drop the `replace` lines and this
   directory, and remove the COPY of engine/third_party in the Dockerfile.
