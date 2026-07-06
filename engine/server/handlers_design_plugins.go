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

// Design plugins are swappable canvas skins (tile / aura / background renderers)
// loaded at runtime from ~/.mywant/design-plugin/<id>/plugin.<ext>. This mirrors
// the card-plugin handler (handlers_plugins.go): scan the dir, esbuild-compile
// JSX/TSX → ESM on demand, MD5-cache. The compiled module self-registers via
// window.__mywant.registerDesign (see web/src/main.tsx).

type designPluginCache struct {
	mu    sync.RWMutex
	cache map[string][]byte
}

var globalDesignPluginCache = &designPluginCache{cache: make(map[string][]byte)}

func designPluginBaseDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mywant", "design-plugin")
}

var designPluginExtensions = []string{"plugin.jsx", "plugin.tsx", "plugin.js", "plugin.ts"}

func findDesignPluginFile(id string) (string, error) {
	dir := filepath.Join(designPluginBaseDir(), id)
	for _, ext := range designPluginExtensions {
		candidate := filepath.Join(dir, ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func (s *Server) listDesignPlugins(w http.ResponseWriter, r *http.Request) {
	base := designPluginBaseDir()
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
		// Follow symlinks: use os.Stat instead of entry.IsDir so symlinked dirs are included.
		info, err := os.Stat(filepath.Join(base, entry.Name()))
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := findDesignPluginFile(entry.Name()); err == nil {
			urls = append(urls, "/api/v1/design-plugins/"+entry.Name()+".js")
		}
	}
	if urls == nil {
		urls = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(urls)
}

func (s *Server) serveDesignPlugin(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]
	if !strings.HasSuffix(filename, ".js") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	pluginID := strings.TrimSuffix(filename, ".js")

	pluginFile, err := findDesignPluginFile(pluginID)
	if err != nil {
		http.Error(w, "design plugin not found: "+pluginID, http.StatusNotFound)
		return
	}

	source, err := os.ReadFile(pluginFile)
	if err != nil {
		http.Error(w, "failed to read design plugin", http.StatusInternalServerError)
		return
	}

	cacheKey := fmt.Sprintf("%x", md5.Sum(source))
	globalDesignPluginCache.mu.RLock()
	if cached, ok := globalDesignPluginCache.cache[cacheKey]; ok {
		globalDesignPluginCache.mu.RUnlock()
		w.Header().Set("Content-Type", "application/javascript")
		w.Write(cached)
		return
	}
	globalDesignPluginCache.mu.RUnlock()

	result := api.Transform(string(source), api.TransformOptions{
		Loader:      loaderFromPath(pluginFile),
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

	globalDesignPluginCache.mu.Lock()
	globalDesignPluginCache.cache[cacheKey] = compiled
	globalDesignPluginCache.mu.Unlock()

	w.Header().Set("Content-Type", "application/javascript")
	w.Write(compiled)
}
