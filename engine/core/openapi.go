package mywant

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/getkin/kin-openapi/openapi3"
	want_spec "github.com/onelittlenightmusic/want-spec"
)

// errSpecNotFound is returned by loadOpenAPISpec when the spec file does not exist
// in the want-spec FS (e.g. Homebrew installs, built-in types). Callers that can
// safely skip validation check for this sentinel; callers that require the spec
// treat it as an error.
var errSpecNotFound = errors.New("openapi spec not found in want-spec module")

// loadOpenAPISpec loads and validates an OpenAPI spec from the embedded want-spec FS.
// Returns errSpecNotFound if the file is absent so callers can decide to skip or fail.
func loadOpenAPISpec(specPath string) (*openapi3.T, error) {
	specData, err := fs.ReadFile(want_spec.FS, specPath)
	if err != nil {
		return nil, errSpecNotFound
	}
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(specData)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec %s: %w", specPath, err)
	}
	if err = spec.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("OpenAPI spec %s is invalid: %w", specPath, err)
	}
	return spec, nil
}
