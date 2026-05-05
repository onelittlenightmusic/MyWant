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

type pluginCache struct {
	mu    sync.RWMutex
	cache map[string][]byte
}

var globalPluginCache = &pluginCache{cache: make(map[string][]byte)}

func customTypesBaseDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mywant", "custom-types")
}

var pluginExtensions = []string{"plugin.jsx", "plugin.tsx", "plugin.js", "plugin.ts"}

func findPluginFile(id string) (string, error) {
	viewDir := filepath.Join(customTypesBaseDir(), id, "view")
	for _, ext := range pluginExtensions {
		candidate := filepath.Join(viewDir, ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func loaderFromPath(path string) api.Loader {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jsx":
		return api.LoaderJSX
	case ".tsx":
		return api.LoaderTSX
	case ".ts":
		return api.LoaderTS
	default:
		return api.LoaderJS
	}
}

func (s *Server) listPlugins(w http.ResponseWriter, r *http.Request) {
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
		// Follow symlinks: use os.Stat instead of entry.IsDir so symlinked dirs are included.
		info, err := os.Stat(filepath.Join(base, entry.Name()))
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := findPluginFile(entry.Name()); err == nil {
			urls = append(urls, "/api/v1/plugins/"+entry.Name()+".js")
		}
	}
	if urls == nil {
		urls = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(urls)
}

func (s *Server) servePlugin(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]
	if !strings.HasSuffix(filename, ".js") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	pluginID := strings.TrimSuffix(filename, ".js")

	pluginFile, err := findPluginFile(pluginID)
	if err != nil {
		http.Error(w, "plugin not found: "+pluginID, http.StatusNotFound)
		return
	}

	source, err := os.ReadFile(pluginFile)
	if err != nil {
		http.Error(w, "failed to read plugin", http.StatusInternalServerError)
		return
	}

	cacheKey := fmt.Sprintf("%x", md5.Sum(source))
	globalPluginCache.mu.RLock()
	if cached, ok := globalPluginCache.cache[cacheKey]; ok {
		globalPluginCache.mu.RUnlock()
		w.Header().Set("Content-Type", "application/javascript")
		w.Write(cached)
		return
	}
	globalPluginCache.mu.RUnlock()

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

	globalPluginCache.mu.Lock()
	globalPluginCache.cache[cacheKey] = compiled
	globalPluginCache.mu.Unlock()

	w.Header().Set("Content-Type", "application/javascript")
	w.Write(compiled)
}
