package server

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"
)

// groupLabelPrefix is the reserved label-key prefix that expresses group
// membership. A group named "近い" is the label "group/近い"="true" carried by
// each member — a memo value (via MemoLabelStore) or a want (via
// metadata.labels). There is no separate group ledger; a group exists exactly
// as long as some member still carries its label.
const groupLabelPrefix = "group/"

func groupKey(name string) string { return groupLabelPrefix + name }

// groupNameFromKey returns the group name for a "group/<name>" key, or "" if the
// key isn't a group label.
func groupNameFromKey(key string) string {
	if !strings.HasPrefix(key, groupLabelPrefix) {
		return ""
	}
	return strings.TrimPrefix(key, groupLabelPrefix)
}

// groupDTO is the API shape (id == name; kind namespaces memo vs want).
type groupDTO struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Members []string `json:"members"`
}

// collectMemoGroups aggregates group/* labels across all memo values.
func (s *Server) collectMemoGroups() []groupDTO {
	byName := map[string][]string{}
	for valueID, labels := range s.memoLabels.All() {
		for key := range labels {
			if name := groupNameFromKey(key); name != "" {
				byName[name] = append(byName[name], valueID)
			}
		}
	}
	return groupsFromMap(byName, "memo")
}

// collectWantGroups aggregates group/* labels across all live wants.
func (s *Server) collectWantGroups() []groupDTO {
	byName := map[string][]string{}
	if s.globalBuilder != nil {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			for key := range want.Metadata.Labels {
				if name := groupNameFromKey(key); name != "" {
					byName[name] = append(byName[name], want.Metadata.ID)
				}
			}
		}
	}
	return groupsFromMap(byName, "want")
}

func groupsFromMap(byName map[string][]string, kind string) []groupDTO {
	out := make([]groupDTO, 0, len(byName))
	for name, members := range byName {
		sort.Strings(members)
		out = append(out, groupDTO{ID: name, Name: name, Kind: kind, Members: members})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// setGroupMembership applies (add=true) or clears (add=false) the group label on
// one member of the given kind.
func (s *Server) setGroupMembership(kind, name, member string, add bool) {
	key := groupKey(name)
	if kind == "want" {
		if s.globalBuilder == nil {
			return
		}
		if add {
			_ = s.globalBuilder.QueueWantAddLabel(member, key, "true")
		} else {
			_ = s.globalBuilder.QueueWantRemoveLabel(member, key)
		}
		return
	}
	// memo
	if add {
		_ = s.memoLabels.Set(member, key, "true")
	} else {
		_ = s.memoLabels.Remove(member, key)
	}
}

// GET /api/v1/groups?kind=memo|want
func (s *Server) getGroups(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	var groups []groupDTO
	switch kind {
	case "want":
		groups = s.collectWantGroups()
	case "memo":
		groups = s.collectMemoGroups()
	default:
		groups = append(s.collectMemoGroups(), s.collectWantGroups()...)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"groups": groups})
}

// POST /api/v1/groups   body: {name, kind, members}
func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string   `json:"name"`
		Kind    string   `json:"kind"`
		Members []string `json:"members"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if err := validateGroupName(body.Name); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid group name", err.Error())
		return
	}
	if body.Kind != "memo" && body.Kind != "want" {
		s.JSONError(w, r, http.StatusBadRequest, "invalid kind", "kind must be memo or want")
		return
	}
	for _, m := range body.Members {
		s.setGroupMembership(body.Kind, body.Name, m, true)
	}
	s.JSONResponse(w, http.StatusOK, groupDTO{ID: body.Name, Name: body.Name, Kind: body.Kind, Members: body.Members})
}

// PUT /api/v1/groups/{name}   body: {name?, members?, kind}
// Reconciles membership to the provided set and optionally renames the group.
func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request) {
	oldName := mux.Vars(r)["name"]
	var body struct {
		Name    *string   `json:"name"`
		Members *[]string `json:"members"`
		Kind    string    `json:"kind"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if body.Kind != "memo" && body.Kind != "want" {
		s.JSONError(w, r, http.StatusBadRequest, "invalid kind", "kind must be memo or want")
		return
	}
	newName := oldName
	if body.Name != nil {
		if err := validateGroupName(*body.Name); err != nil {
			s.JSONError(w, r, http.StatusBadRequest, "invalid group name", err.Error())
			return
		}
		newName = *body.Name
	}

	// Current members of the old group.
	current := s.membersOfGroup(body.Kind, oldName)

	if body.Members != nil {
		want := map[string]bool{}
		for _, m := range *body.Members {
			want[m] = true
		}
		// Remove members no longer wanted (from the OLD name).
		for _, m := range current {
			if !want[m] {
				s.setGroupMembership(body.Kind, oldName, m, false)
			}
		}
		// Under a rename, everything moves to the new key below; otherwise add
		// the newly-wanted members to the existing name here.
		if newName == oldName {
			for m := range want {
				s.setGroupMembership(body.Kind, oldName, m, true)
			}
		}
	}

	// Rename: move every (final) member from the old key to the new key.
	if newName != oldName {
		final := *orDefaultMembers(body.Members, current)
		for _, m := range final {
			s.setGroupMembership(body.Kind, oldName, m, false)
			s.setGroupMembership(body.Kind, newName, m, true)
		}
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "group updated", "name": newName})
}

// DELETE /api/v1/groups/{name}?kind=memo|want
func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	kind := r.URL.Query().Get("kind")
	if kind != "memo" && kind != "want" {
		s.JSONError(w, r, http.StatusBadRequest, "invalid kind", "kind query param must be memo or want")
		return
	}
	for _, m := range s.membersOfGroup(kind, name) {
		s.setGroupMembership(kind, name, m, false)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "group deleted"})
}

// membersOfGroup returns the current member ids of a group of the given kind.
func (s *Server) membersOfGroup(kind, name string) []string {
	key := groupKey(name)
	if kind == "want" {
		var out []string
		if s.globalBuilder != nil {
			for _, want := range s.globalBuilder.GetAllWantStates() {
				if _, ok := want.Metadata.Labels[key]; ok {
					out = append(out, want.Metadata.ID)
				}
			}
		}
		return out
	}
	return s.memoLabels.ValuesWithLabel(key)
}

func orDefaultMembers(members *[]string, fallback []string) *[]string {
	if members != nil {
		return members
	}
	return &fallback
}

func validateGroupName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errGroupName("group name must not be empty")
	}
	if strings.Contains(name, "/") {
		return errGroupName("group name must not contain '/'")
	}
	return nil
}

type errGroupName string

func (e errGroupName) Error() string { return string(e) }

// ── Raw memo-label endpoints (general facility; groups ride on top) ──────────

// GET /api/v1/memo/labels
func (s *Server) getMemoLabels(w http.ResponseWriter, _ *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]any{"labels": s.memoLabels.All()})
}

// POST /api/v1/memo/labels   body: {value_id, key, value}
func (s *Server) setMemoLabel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ValueID string `json:"value_id"`
		Key     string `json:"key"`
		Value   string `json:"value"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if err := s.memoLabels.Set(body.ValueID, body.Key, body.Value); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to set memo label", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "label set"})
}

// POST /api/v1/memo/labels/remove   body: {value_id, key}
func (s *Server) removeMemoLabel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ValueID string `json:"value_id"`
		Key     string `json:"key"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if err := s.memoLabels.Remove(body.ValueID, body.Key); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to remove memo label", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "label removed"})
}
