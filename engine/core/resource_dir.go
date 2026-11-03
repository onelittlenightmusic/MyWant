package mywant

// resource_dir.go — documents kept one file each, so they outlive the server.
//
// The robot's Plans and Acts (engine/server/plans.go, acts.go) are kept in
// ~/.mywant/plans/<name>.yaml and ~/.mywant/acts/<name>.yaml. One file a
// document: an act is written again at each step, and that rewrites only it.

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ResourceDir is a directory of YAML documents, one file each, named by the
// document's name.
type ResourceDir struct {
	Path string
}

// NewResourceDir is ~/.mywant/<kind>.
func NewResourceDir(kind string) *ResourceDir {
	return &ResourceDir{Path: filepath.Join(MyWantHomeDir(), kind)}
}

func (d *ResourceDir) file(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", fmt.Errorf("bad resource name %q", name)
	}
	return filepath.Join(d.Path, name+".yaml"), nil
}

// Save writes the document as <name>.yaml, whole or not at all (written
// aside, then renamed over).
func (d *ResourceDir) Save(name string, doc any) error {
	path, err := d.file(name)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d.Path, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(d.Path, "."+name+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Remove lets <name>.yaml go; one already gone is no error.
func (d *ResourceDir) Remove(name string) error {
	path, err := d.file(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Load reads every document there, each handed to read as its bytes. A file
// that cannot be read, or that read refuses, is logged and passed over.
func (d *ResourceDir) Load(read func(data []byte) error) {
	entries, err := os.ReadDir(d.Path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[WARN] resources: read %s: %v", d.Path, err)
		}
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		path := filepath.Join(d.Path, name)
		data, err := os.ReadFile(path)
		if err == nil {
			err = read(data)
		}
		if err != nil {
			log.Printf("[WARN] resources: %s: %v", path, err)
		}
	}
}
