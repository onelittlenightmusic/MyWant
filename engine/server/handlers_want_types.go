package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// registerWantType handles POST /api/v1/want-types
// Body: raw YAML of a wantType definition.
// The type is registered immediately (hot-reload) and persisted to yaml/want_types/custom/.
func (s *Server) registerWantType(w http.ResponseWriter, r *http.Request) {
	def, rawYAML, err := s.parseWantTypeBody(w, r)
	if err != nil {
		return // error already written
	}

	// Reject only if the type is truly Go-backed: no inline agents AND no external requires.
	// YAML-defined types that use external agents via "requires" have no inlineAgents but are not Go-backed.
	if existing := s.wantTypeLoader.GetDefinition(def.Metadata.Name); existing != nil &&
		len(existing.InlineAgents) == 0 && len(existing.Requires) == 0 {
		http.Error(w, fmt.Sprintf("want type %q is backed by Go code and cannot be overridden via API", def.Metadata.Name), http.StatusConflict)
		return
	}

	if err := s.applyWantType(def, rawYAML); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/want-types", def.Metadata.Name, "registered", 0, "", "")
	s.JSONResponse(w, http.StatusCreated, map[string]any{
		"name":    def.Metadata.Name,
		"message": "want type registered successfully",
	})
}

// updateWantType handles PUT /api/v1/want-types/{name}
// Replaces an existing YAML-only want type definition with the new one.
func (s *Server) updateWantType(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	def, rawYAML, err := s.parseWantTypeBody(w, r)
	if err != nil {
		return
	}

	if def.Metadata.Name != name {
		http.Error(w, fmt.Sprintf("URL name %q does not match YAML metadata.name %q", name, def.Metadata.Name), http.StatusBadRequest)
		return
	}

	// Reject if a Go-backed type exists with this name.
	if existing := s.wantTypeLoader.GetDefinition(name); existing != nil && len(existing.InlineAgents) == 0 {
		http.Error(w, fmt.Sprintf("want type %q is backed by Go code and cannot be updated via API", name), http.StatusConflict)
		return
	}

	if err := s.applyWantType(def, rawYAML); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.globalBuilder.LogAPIOperation("PUT", "/want-types/"+name, name, "updated", 0, "", "")
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"name":    def.Metadata.Name,
		"message": "want type updated successfully",
	})
}

// deleteWantType handles DELETE /api/v1/want-types/{name}
// Removes a YAML-only want type from the live registry and deletes its persisted file.
func (s *Server) deleteWantType(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	if s.wantTypeLoader == nil {
		http.Error(w, "want type loader not available", http.StatusServiceUnavailable)
		return
	}

	if err := s.wantTypeLoader.UnregisterDefinition(name); err != nil {
		status := http.StatusInternalServerError
		switch err.Error() {
		case fmt.Sprintf("want type %q not found", name):
			status = http.StatusNotFound
		case fmt.Sprintf("want type %q is a system type and cannot be deleted", name),
			fmt.Sprintf("want type %q is backed by Go code and cannot be deleted via API", name):
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}

	// Remove from builder registry so new wants cannot use it.
	s.globalBuilder.UnregisterWantType(name)

	s.globalBuilder.LogAPIOperation("DELETE", "/want-types/"+name, name, "deleted", 0, "", "")
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"name":    name,
		"message": "want type deleted successfully",
	})
}

// parseWantTypeBody reads and validates the YAML body from the request.
func (s *Server) parseWantTypeBody(w http.ResponseWriter, r *http.Request) (*mywant.WantTypeDefinition, []byte, error) {
	if s.wantTypeLoader == nil {
		http.Error(w, "want type loader not available", http.StatusServiceUnavailable)
		return nil, nil, fmt.Errorf("no loader")
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return nil, nil, err
	}

	def, err := s.wantTypeLoader.ParseDefinitionFromYAML(body)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid want type YAML: %v", err), http.StatusBadRequest)
		return nil, nil, err
	}
	return def, body, nil
}

// applyWantType registers the definition in the loader and builder, then persists it to disk.
func (s *Server) applyWantType(def *mywant.WantTypeDefinition, rawYAML []byte) error {
	// 1. Register in loader (makes it visible to GET /want-types).
	s.wantTypeLoader.RegisterDefinition(def)

	// 2. Register factory + inline agents in the builder (hot-wire into live registry).
	s.globalBuilder.StoreWantTypeDefinition(def)

	// 3. Persist to yaml/want_types/custom/ so it survives server restart.
	if err := persistWantTypeYAML(def.Metadata.Name, rawYAML); err != nil {
		// Non-fatal: log but don't fail the registration.
		s.globalBuilder.LogAPIOperation("WARN", "/want-types", def.Metadata.Name, "persist-failed", 0, err.Error(), "")
	}

	return nil
}

// persistWantTypeYAML saves the raw YAML to yaml/want_types/custom/<name>.yaml.
func persistWantTypeYAML(name string, data []byte) error {
	// Sanitise name to prevent path traversal.
	if !validWantTypeName.MatchString(name) {
		return fmt.Errorf("want type name %q contains invalid characters", name)
	}

	customDir := filepath.Join(mywant.WantTypesDir, "custom")
	if err := os.MkdirAll(customDir, 0755); err != nil {
		return fmt.Errorf("failed to create custom dir: %w", err)
	}

	path := filepath.Join(customDir, name+".yaml")
	return os.WriteFile(path, data, 0644)
}

var validWantTypeName = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)
