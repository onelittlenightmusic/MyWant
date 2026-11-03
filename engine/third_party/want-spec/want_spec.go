package want_spec

import "embed"

// FS is an embedded filesystem containing all OpenAPI specifications.
//
//go:embed spec/*.yaml spec/schemas/*.yaml spec/data/*.yaml
var FS embed.FS
