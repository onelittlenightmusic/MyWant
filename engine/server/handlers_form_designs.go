package server

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/gorilla/mux"
)

// Form designs are how a want type is drawn on the canvas — the tile surface it
// is made of, the shape it takes on the ground, and how that shape is dressed.
// They are loaded from ~/.mywant/custom-types/<id>/form-design/plugin.<ext>.
//
// That is the same directory a want type already keeps its card view in, one
// level over: view/ is the card, form-design/ is the board. A type whose look
// only means anything alongside it — a lamp tile that reads an "on" field only
// that type has — travels with the type, so installing the custom installs the
// look and uninstalling it takes the look away again.
//
// Mechanically this mirrors handlers_plugins.go and handlers_design_plugins.go:
// scan the dir, esbuild-compile JSX/TSX → ESM on demand, MD5-cache. The compiled
// module self-registers via window.__mywant.registerTileSurface /
// registerFormType / registerFormStyle (see web/src/main.tsx in mywant-gui).

type formDesignCache struct {
	mu    sync.RWMutex
	cache map[string][]byte
}

var globalFormDesignCache = &formDesignCache{cache: make(map[string][]byte)}

var formDesignExtensions = []string{"plugin.jsx", "plugin.tsx", "plugin.js", "plugin.ts"}

func findFormDesignFile(id string) (string, error) {
	dir := filepath.Join(customTypesBaseDir(), id, "form-design")
	for _, ext := range formDesignExtensions {
		candidate := filepath.Join(dir, ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func (s *Server) listFormDesigns(w http.ResponseWriter, r *http.Request) {
	base := customTypesBaseDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]string{})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var urls []string
	for _, entry := range entries {
		// Follow symlinks: use os.Stat instead of entry.IsDir so symlinked dirs
		// are included — `mywant custom install` links customs into place.
		info, err := os.Stat(filepath.Join(base, entry.Name()))
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := findFormDesignFile(entry.Name()); err == nil {
			urls = append(urls, "/api/v1/form-designs/"+entry.Name()+".js")
		}
	}
	if urls == nil {
		urls = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(urls)
}

func (s *Server) serveFormDesign(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]
	if !strings.HasSuffix(filename, ".js") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	designID := strings.TrimSuffix(filename, ".js")

	designFile, err := findFormDesignFile(designID)
	if err != nil {
		http.Error(w, "form design not found: "+designID, http.StatusNotFound)
		return
	}

	source, err := os.ReadFile(designFile)
	if err != nil {
		http.Error(w, "failed to read form design", http.StatusInternalServerError)
		return
	}

	cacheKey := fmt.Sprintf("%x", md5.Sum(source))
	globalFormDesignCache.mu.RLock()
	if cached, ok := globalFormDesignCache.cache[cacheKey]; ok {
		globalFormDesignCache.mu.RUnlock()
		w.Header().Set("Content-Type", "application/javascript")
		w.Write(cached)
		return
	}
	globalFormDesignCache.mu.RUnlock()

	result := api.Transform(string(source), api.TransformOptions{
		Loader:      loaderFromPath(designFile),
		Format:      api.FormatESModule,
		JSXFactory:  "window.React.createElement",
		JSXFragment: "window.React.Fragment",
		Target:      api.ES2020,
	})

	if len(result.Errors) > 0 {
		http.Error(w, "compilation error: "+result.Errors[0].Text, http.StatusBadRequest)
		return
	}

	compiled := result.Code

	globalFormDesignCache.mu.Lock()
	globalFormDesignCache.cache[cacheKey] = compiled
	globalFormDesignCache.mu.Unlock()

	w.Header().Set("Content-Type", "application/javascript")
	w.Write(compiled)
}
