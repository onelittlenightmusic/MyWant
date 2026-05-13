package bundled

import "embed"

// BuiltinFS is the embedded filesystem containing all built-in YAML definitions.
// Populated at build time from engine/bundled/.
//
//go:embed want_types recipes agents capabilities achievements data policies system_wants.yaml
var BuiltinFS embed.FS
