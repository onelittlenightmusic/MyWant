package server

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"

	mywant "mywant/engine/core"
	"mywant/engine/labels"
)

// Labels on kata, as wants and things carry them.
//
// A kata's definition is bundled and read-only, so labels set at runtime live
// in their own file, by kata id, and are layered over the ones its definition
// writes — see mywant.Kata.Labels.

func newKataLabelStore() *labels.FileStore {
	home, err := os.UserHomeDir()
	if err != nil {
		return labels.NewFileStore("kata-labels.yaml")
	}
	return labels.NewFileStore(filepath.Join(home, ".mywant", "kata-labels.yaml"))
}

// kataLabelsOf is a kata's labels as the API reports them: its definition's,
// then the runtime ones over them. Nil when it has none.
func (s *Server) kataLabelsOf(k mywant.Kata) map[string]string {
	var runtime map[string]string
	if s.kataLabels != nil {
		runtime = s.kataLabels.Get(k.ID)
	}
	if len(k.Labels) == 0 && len(runtime) == 0 {
		return nil
	}
	out := labels.Clone(k.Labels)
	for key, v := range runtime {
		out[key] = v
	}
	return out
}

// POST /api/v1/kata/{id}/labels   body: {key, value}
func (s *Server) setKataLabel(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if _, ok := mywant.GetKata(id); !ok {
		s.JSONError(w, r, http.StatusNotFound, "kata not found", id)
		return
	}
	var body struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := DecodeRequest(r, &body); err != nil || body.Key == "" {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", "key is required")
		return
	}
	if err := s.kataLabels.Set(id, body.Key, body.Value); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to set kata label", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"labels": s.kataLabels.Get(id)})
}

// DELETE /api/v1/kata/{id}/labels/{key}
func (s *Server) removeKataLabel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, key := vars["id"], vars["key"]
	if _, ok := mywant.GetKata(id); !ok {
		s.JSONError(w, r, http.StatusNotFound, "kata not found", id)
		return
	}
	if err := s.kataLabels.Remove(id, key); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to remove kata label", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"labels": s.kataLabels.Get(id)})
}
