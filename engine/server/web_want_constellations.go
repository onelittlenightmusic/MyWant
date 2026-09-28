package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// WebWantConstellation joins a page's aura marks the way a constellation joins
// tiles on the board: a named chain, in the order its marks were tied on the
// wire (Y), each drawn to the next. On the page, Z mode hops along it — from a
// mark to the ones it is joined to — where the plain D-pad hops to whatever is
// nearest.
//
// Marks are named by selector, the same key the page's elements are kept by.
type WebWantConstellation struct {
	Name      string   `json:"name"`
	Selectors []string `json:"selectors"`
}

// webWantConstellationsFile sits beside elements.json in the type's directory.
// Its own file, so elements.json keeps the {hostname: [elements]} shape every
// reader of it already expects.
const webWantConstellationsFile = "constellations.json"

// cleanWebWantConstellations keeps what can be drawn: a named chain of at least
// two marks the page actually has, each mark once. A chain whose marks were
// removed shrinks with them, and goes when fewer than two are left.
func cleanWebWantConstellations(in []WebWantConstellation, elements []WebWantElement) []WebWantConstellation {
	have := make(map[string]bool, len(elements))
	for _, e := range elements {
		if e.Selector != "" {
			have[e.Selector] = true
		}
	}
	var out []WebWantConstellation
	seenName := map[string]bool{}
	for _, c := range in {
		name := strings.TrimSpace(c.Name)
		if name == "" || seenName[name] {
			continue
		}
		var sels []string
		seen := map[string]bool{}
		for _, s := range c.Selectors {
			if have[s] && !seen[s] {
				seen[s] = true
				sels = append(sels, s)
			}
		}
		if len(sels) < 2 {
			continue
		}
		seenName[name] = true
		out = append(out, WebWantConstellation{Name: name, Selectors: sels})
	}
	return out
}

// writeWebWantConstellations replaces the type's constellations — PUT
// semantics, like the elements beside them. None left removes the file.
func writeWebWantConstellations(name string, cs []WebWantConstellation) error {
	path := filepath.Join(mywant.UserCustomTypesDir(), name, webWantConstellationsFile)
	if len(cs) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// readWebWantConstellations is the type's constellations, or none.
func readWebWantConstellations(name string) []WebWantConstellation {
	data, err := os.ReadFile(filepath.Join(mywant.UserCustomTypesDir(), name, webWantConstellationsFile))
	if err != nil {
		return []WebWantConstellation{}
	}
	var cs []WebWantConstellation
	if json.Unmarshal(data, &cs) != nil || cs == nil {
		return []WebWantConstellation{}
	}
	return cs
}

// getWebWantConstellations handles GET /api/v1/web-wants/{name}/constellations
// — for the Inspect overlay, which preloads the marks from GET /{name}.
func (s *Server) getWebWantConstellations(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if !validTypeName.MatchString(name) {
		s.JSONError(w, r, http.StatusBadRequest, "invalid type name", "")
		return
	}
	if info, err := os.Stat(filepath.Join(mywant.UserCustomTypesDir(), name)); err != nil || !info.IsDir() {
		s.JSONError(w, r, http.StatusNotFound, "web want type not found", "")
		return
	}
	s.JSONResponse(w, http.StatusOK, readWebWantConstellations(name))
}
