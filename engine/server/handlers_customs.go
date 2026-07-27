package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// Custom endpoints let a client install packages onto the machine running the
// server, which is the only filesystem the want type / design plugin / recipe
// loaders ever read. Without them "mywant custom install" could only ever touch
// the operator's own laptop, never a remote backend.
//
// Installing runs git against a caller-supplied source and puts executable
// plugin code where the server will load it, so these routes are exactly as
// privileged as the server itself - keep them behind the same auth as the rest
// of the API.

type customInstallRequest struct {
	Source string `json:"source"`
	Name   string `json:"name,omitempty"`
	Kind   string `json:"kind,omitempty"` // comma separated, empty = auto-detect
	Force  bool   `json:"force,omitempty"`
}

// listCustoms handles GET /api/v1/customs
func (s *Server) listCustoms(w http.ResponseWriter, _ *http.Request) {
	reg, err := mywant.LoadCustomRegistry()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read customs registry: %v", err), http.StatusInternalServerError)
		return
	}

	customs := make([]mywant.CustomRecord, 0, len(reg.Customs))
	for _, rec := range reg.Customs {
		rec.Status = rec.DeriveStatus()
		customs = append(customs, rec)
	}

	untracked := mywant.FindUntrackedCustoms(reg)
	if untracked == nil {
		untracked = []mywant.UntrackedCustom{}
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"customs":   customs,
		"untracked": untracked,
	})
}

// installCustom handles POST /api/v1/customs
// Body: {"source": "...", "name": "...", "kind": "...", "force": false}
func (s *Server) installCustom(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var req customInstallRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON body: %v", err), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Source) == "" {
		http.Error(w, "source is required", http.StatusBadRequest)
		return
	}

	rec, err := mywant.InstallCustom(req.Source, req.Name, req.Kind, req.Force)
	if err != nil {
		http.Error(w, err.Error(), installErrorStatus(err))
		return
	}
	rec.Status = rec.DeriveStatus()

	loaded, warnings := s.reloadUserCustomTypesAndSync()
	s.globalBuilder.LogAPIOperation("POST", "/customs", rec.Name, "installed", loaded, "", "")

	s.JSONResponse(w, http.StatusCreated, map[string]any{
		"custom":         rec,
		"reloaded":       loaded,
		"warnings":       warnings,
		"restart_needed": len(rec.Agents) > 0,
		"message":        fmt.Sprintf("installed custom %s", rec.Name),
	})
}

// uninstallCustom handles DELETE /api/v1/customs/{name}?force=true
func (s *Server) uninstallCustom(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	force := r.URL.Query().Get("force") == "true"

	removed, hadAgents, err := mywant.UninstallCustom(name, force)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case strings.Contains(err.Error(), "is not installed"):
			status = http.StatusNotFound
		case strings.Contains(err.Error(), "use --force"), strings.Contains(err.Error(), "use force"):
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}

	loaded, warnings := s.reloadUserCustomTypesAndSync()
	s.globalBuilder.LogAPIOperation("DELETE", "/customs/"+name, name, "uninstalled", loaded, "", "")

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"name":           name,
		"removed":        removed,
		"reloaded":       loaded,
		"warnings":       warnings,
		"restart_needed": hadAgents,
		"message":        fmt.Sprintf("uninstalled custom %s", name),
	})
}

// installErrorStatus maps the install failures a caller can fix themselves to
// 4xx, so a bad source is not reported as a server fault.
func installErrorStatus(err error) int {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "already exists"), strings.Contains(msg, "is a symlink"):
		return http.StatusConflict
	case strings.Contains(msg, "unknown custom kind"),
		strings.Contains(msg, "invalid custom name"),
		strings.Contains(msg, "could not tell what this custom provides"),
		strings.Contains(msg, "custom.yaml"):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
