package bundled

import "embed"

// WantTypes is the embedded filesystem containing built-in want type definitions.
// Populated at build time from engine/bundled/want_types/.
//
//go:embed want_types
var WantTypes embed.FS
