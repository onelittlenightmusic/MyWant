package mywant

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"

	want_spec "github.com/onelittlenightmusic/want-spec"
	"gopkg.in/yaml.v3"
)

// LoadConfigFromYAML loads a bare-array YAML file and returns []*want_spec.Want.
// The YAML format is a top-level array:
//
//   - metadata:
//     name: foo
//     type: bar
//     spec:
//     params: {}
func LoadConfigFromYAML(filename string) ([]*want_spec.Want, error) {
	return loadConfigFromYAML(filename)
}

// LoadConfigFromYAMLBytes loads wants from YAML bytes (bare array format).
func LoadConfigFromYAMLBytes(data []byte) ([]*want_spec.Want, error) {
	return loadConfigFromYAMLBytes(data)
}

// loadConfigFromYAML loads configuration from a YAML file.
func loadConfigFromYAML(filename string) ([]*want_spec.Want, error) {
	InfoLog("[CONFIG-YAML] 📖 Loading config from: %s\n", filename)

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}

	return loadConfigFromYAMLBytes(data)
}

// loadConfigFromYAMLBytes loads wants from YAML bytes.
func loadConfigFromYAMLBytes(data []byte) ([]*want_spec.Want, error) {
	var wants []*want_spec.Want
	if err := yaml.Unmarshal(data, &wants); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	assignWantIDs(wants)

	InfoLog("[CONFIG-YAML] ✅ Loaded %d wants from config\n", len(wants))
	for i, want := range wants {
		recipe := ""
		if want.Spec.Recipe != "" {
			recipe = fmt.Sprintf(", recipe=%s", want.Spec.Recipe)
		}
		InfoLog("[CONFIG-YAML]   [%d] %s (type=%s%s)\n", i, want.Metadata.Name, want.Metadata.Type, recipe)
		if unknown := want.Spec.UnknownFields; len(unknown) > 0 {
			log.Printf("[WARN][want-spec drift] want %q has unknown spec fields in YAML config: %v — config file may reference a newer want-spec version than this engine", want.Metadata.Name, unknown)
		}
	}

	return wants, nil
}

// GenerateUUID generates a UUID v4 for want IDs.
func GenerateUUID() string {
	uuid := make([]byte, 16)
	rand.Read(uuid)
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant
	return fmt.Sprintf("want-%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// assignWantIDs assigns unique IDs to wants that don't have them.
func assignWantIDs(wants []*want_spec.Want) {
	for i := range wants {
		if wants[i].Metadata.ID == "" {
			wants[i].Metadata.ID = GenerateUUID()
		}
	}
}
