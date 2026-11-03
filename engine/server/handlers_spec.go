package server

// handlers_spec.go — the definitions, as want-spec gives them.
//
// Every object's definition (wants, want types, things, recipes, agents and
// capabilities, constellations, character skills, plans and acts) is
// want-spec's, embedded in this binary (want_spec.FS); the server keeps no
// copy of its own. These hand them out:
//
//	GET /api/v1/spec          the index: want-spec's module and version, and its spec files
//	GET /api/v1/spec/{file}   one of them (plan-spec.yaml, schemas/Want.yaml, …)
//
// YAML, or JSON when the request accepts application/json.

import (
	"io/fs"
	"net/http"
	"path"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	ws "github.com/onelittlenightmusic/want-spec"
	"gopkg.in/yaml.v3"
)

const wantSpecModule = "github.com/onelittlenightmusic/want-spec"

// specIndex is GET /api/v1/spec.
type specIndex struct {
	Module  string   `json:"module" yaml:"module"`
	Version string   `json:"version,omitempty" yaml:"version,omitempty"`
	Specs   []string `json:"specs" yaml:"specs"`
}

// wantSpecVersion is the want-spec this binary was built with.
func wantSpecVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, d := range info.Deps {
		if d.Path == wantSpecModule {
			if d.Replace != nil {
				return d.Replace.Version
			}
			return d.Version
		}
	}
	return ""
}

// specFiles is every spec file want-spec embeds, relative to spec/.
func specFiles() []string {
	var out []string
	_ = fs.WalkDir(ws.FS, "spec", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".yaml") {
			out = append(out, strings.TrimPrefix(p, "spec/"))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// GET /api/v1/spec
func (s *Server) getSpecIndex(w http.ResponseWriter, r *http.Request) {
	idx := specIndex{Module: wantSpecModule, Version: wantSpecVersion(), Specs: specFiles()}
	if wantsJSON(r) {
		s.JSONResponse(w, http.StatusOK, idx)
		return
	}
	data, err := yaml.Marshal(&idx)
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Failed to write the index", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(data)
}

// GET /api/v1/spec/{file}
func (s *Server) getSpecFile(w http.ResponseWriter, r *http.Request) {
	name := path.Clean("/" + mux.Vars(r)["file"])[1:]
	data, err := fs.ReadFile(ws.FS, "spec/"+name)
	if name == "" || err != nil {
		s.JSONError(w, r, http.StatusNotFound, "No such spec", "see GET /api/v1/spec for the list")
		return
	}
	if wantsJSON(r) {
		var body any
		if err := yaml.Unmarshal(data, &body); err != nil {
			s.JSONError(w, r, http.StatusInternalServerError, "Failed to parse the spec", err.Error())
			return
		}
		s.JSONResponse(w, http.StatusOK, body)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(data)
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}
