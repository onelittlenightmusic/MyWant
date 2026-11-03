package mywant

import (
	"os"
	"path/filepath"
	"testing"
)

// A recipe's metadata carries labels: allowed by want-spec's schema, and read
// back with the rest of its metadata.
func TestRecipeMetadataLabels(t *testing.T) {
	recipe := []byte(`recipe:
  metadata:
    name: labelled
    description: a recipe with labels
    version: "1.0.0"
    labels:
      team: robot
  wants:
    - metadata:
        name: one
        type: reminder
`)
	if err := validateRecipeWithSpec(recipe); err != nil {
		t.Fatalf("labels in a recipe's metadata are valid: %v", err)
	}
	path := filepath.Join(t.TempDir(), "labelled.yaml")
	if err := os.WriteFile(path, recipe, 0644); err != nil {
		t.Fatal(err)
	}
	meta, err := NewGenericRecipeLoader(t.TempDir()).GetRecipeMetadata(path)
	if err != nil || meta.Labels["team"] != "robot" {
		t.Errorf("read back: %+v %v", meta, err)
	}
}
