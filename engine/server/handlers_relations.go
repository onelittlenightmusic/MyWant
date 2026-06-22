package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// RelationRecord represents a single expose-based relation between a provider and a consumer.
type RelationRecord struct {
	ID           string `json:"id"`
	ProviderID   string `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	ConsumerID   string `json:"consumer_id"`
	ConsumerName string `json:"consumer_name"`
	FieldName    string `json:"field_name"`  // bare field name, e.g. "album_art_url"
	FieldLabel   string `json:"field_label"` // prefixed, e.g. "expose/album_art_url"
	DataType     string `json:"data_type,omitempty"`
}

// computeRelationID produces a deterministic short ID for a provider+field pair.
// All consumers of the same expose entry share the same relation ID.
func computeRelationID(providerID, fieldName string) string {
	h := sha256.Sum256([]byte(providerID + ":" + fieldName))
	return fmt.Sprintf("%x", h)[:12]
}

// buildRelations collects all expose-based RelationRecords from the live want set.
func (s *Server) buildRelations(filter func(providerID, consumerID string) bool) []RelationRecord {
	wantsByID := make(map[string]*mywant.Want)
	if s.globalBuilder != nil {
		for _, w := range s.globalBuilder.GetAllWantStates() {
			wantsByID[w.Metadata.ID] = w
		}
	}

	seen := make(map[string]bool) // pairKey → already added
	var records []RelationRecord

	for _, w := range wantsByID {
		for _, corr := range w.Metadata.Correlation {
			// stateAccess/consumer:expose/X → this want is the PROVIDER
			for _, l := range corr.Labels {
				if !strings.HasPrefix(l, "stateAccess/consumer:expose/") {
					continue
				}
				fieldName := strings.TrimPrefix(l, "stateAccess/consumer:expose/")
				providerID := w.Metadata.ID
				consumerID := corr.WantID
				pairKey := providerID + "\x00" + fieldName + "\x00" + consumerID
				if seen[pairKey] {
					continue
				}
				seen[pairKey] = true
				if filter != nil && !filter(providerID, consumerID) {
					continue
				}
				peer := wantsByID[consumerID]
				consumerName := consumerID
				if peer != nil {
					consumerName = peer.Metadata.Name
				}
				records = append(records, RelationRecord{
					ID:           computeRelationID(providerID, fieldName),
					ProviderID:   providerID,
					ProviderName: w.Metadata.Name,
					ConsumerID:   consumerID,
					ConsumerName: consumerName,
					FieldName:    fieldName,
					FieldLabel:   "expose/" + fieldName,
					DataType:     corr.DataType,
				})
			}
		}
	}
	return records
}

// listRelations handles GET /api/v1/relations
func (s *Server) listRelations(w http.ResponseWriter, r *http.Request) {
	records := s.buildRelations(nil)
	s.JSONResponse(w, http.StatusOK, map[string]any{"relations": records})
}

// listWantRelations handles GET /api/v1/wants/{id}/relations
func (s *Server) listWantRelations(w http.ResponseWriter, r *http.Request) {
	wantID := mux.Vars(r)["id"]
	records := s.buildRelations(func(providerID, consumerID string) bool {
		return providerID == wantID || consumerID == wantID
	})
	s.JSONResponse(w, http.StatusOK, map[string]any{"relations": records})
}

// deleteRelationByID handles DELETE /api/v1/relations/{id}
// Finds the matching expose entry by relation ID and removes it (same logic as removeRelation).
func (s *Server) deleteRelationByID(w http.ResponseWriter, r *http.Request) {
	relationID := mux.Vars(r)["id"]

	// Search all wants for a provider whose computeRelationID matches.
	var providerWant *mywant.Want
	var fieldName string

	if s.globalBuilder != nil {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			for _, corr := range want.Metadata.Correlation {
				for _, l := range corr.Labels {
					if !strings.HasPrefix(l, "stateAccess/consumer:expose/") {
						continue
					}
					fn := strings.TrimPrefix(l, "stateAccess/consumer:expose/")
					if computeRelationID(want.Metadata.ID, fn) == relationID {
						providerWant = want
						fieldName = fn
						break
					}
				}
				if providerWant != nil {
					break
				}
			}
			if providerWant != nil {
				break
			}
		}
	}

	if providerWant == nil {
		s.JSONError(w, r, http.StatusNotFound, "Relation not found", "")
		return
	}

	// Remove the ExposeEntry from the provider.
	newExposes := make([]mywant.ExposeEntry, 0, len(providerWant.Spec.Exposes))
	removed := false
	for _, e := range providerWant.Spec.Exposes {
		if e.As == fieldName {
			removed = true
		} else {
			newExposes = append(newExposes, e)
		}
	}
	if !removed {
		s.JSONError(w, r, http.StatusNotFound, "Expose entry not found", "")
		return
	}
	providerWant.Spec.Exposes = newExposes
	s.globalBuilder.UpdateWant(providerWant)

	// Remove matching import key from all consumers.
	for _, want := range s.globalBuilder.GetWants() {
		if _, ok := want.Spec.Imports[fieldName]; !ok {
			continue
		}
		newImports := make(map[string]string, len(want.Spec.Imports)-1)
		for k, v := range want.Spec.Imports {
			if k != fieldName {
				newImports[k] = v
			}
		}
		want.Spec.Imports = newImports
		s.globalBuilder.UpdateWant(want)
	}

	s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/relations/{id}", relationID, "success", http.StatusNoContent, "", "Relation removed")
	w.WriteHeader(http.StatusNoContent)
}
