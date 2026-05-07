package mywant

import (
	"io/fs"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadSystemWantConfigsFromFS reads the system wants YAML from an embedded fs.FS.
// path is the file path within fsys (e.g. "system_wants.yaml").
func LoadSystemWantConfigsFromFS(fsys fs.FS, path string) ([]*Want, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, nil
	}
	var cfg systemWantsFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	wants := make([]*Want, 0, len(cfg.SystemWants))
	for _, def := range cfg.SystemWants {
		w := &Want{
			Metadata: Metadata{
				ID:           def.ID,
				Name:         def.Name,
				Type:         def.Type,
				IsSystemWant: true,
			},
			Spec: def.Spec,
		}
		wants = append(wants, w)
	}
	log.Printf("[SystemWants] Loaded %d system want definitions from embedded FS", len(wants))
	return wants, nil
}

type systemWantDef struct {
	ID   string   `yaml:"id"`
	Name string   `yaml:"name"`
	Type string   `yaml:"type"`
	Spec WantSpec `yaml:"spec"`
}

type systemWantsFile struct {
	SystemWants []systemWantDef `yaml:"system_wants"`
}

// LoadSystemWantConfigs reads SystemWantsFile and returns the wants to inject.
// All returned wants have IsSystemWant=true. Returns nil on missing file (non-fatal).
func LoadSystemWantConfigs(path string) ([]*Want, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg systemWantsFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	wants := make([]*Want, 0, len(cfg.SystemWants))
	for _, def := range cfg.SystemWants {
		w := &Want{
			Metadata: Metadata{
				ID:           def.ID,
				Name:         def.Name,
				Type:         def.Type,
				IsSystemWant: true,
			},
			Spec: def.Spec,
		}
		wants = append(wants, w)
	}
	log.Printf("[SystemWants] Loaded %d system want definitions from %s", len(wants), path)
	return wants, nil
}

// SystemWantTypes returns the set of want types defined as system wants.
// Used to filter persisted state on server startup.
func SystemWantTypes(wants []*Want) map[string]bool {
	types := make(map[string]bool, len(wants))
	for _, w := range wants {
		types[w.Metadata.Type] = true
	}
	return types
}
