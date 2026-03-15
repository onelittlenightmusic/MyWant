package server

import (
	"net/http"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// listAchievements returns all achievements.
func (s *Server) listAchievements(w http.ResponseWriter, r *http.Request) {
	achievements := mywant.ListAchievements()
	if achievements == nil {
		achievements = []mywant.Achievement{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"achievements": achievements,
		"count":        len(achievements),
	})
}

// getAchievement returns a single achievement by ID.
func (s *Server) getAchievement(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	a, ok := mywant.GetAchievement(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Achievement not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, a)
}

// createAchievement adds a new achievement (manual / human-awarded).
func (s *Server) createAchievement(w http.ResponseWriter, r *http.Request) {
	var a mywant.Achievement
	if err := DecodeRequest(r, &a); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if a.AgentName == "" || a.Title == "" {
		s.JSONError(w, r, http.StatusBadRequest, "agentName and title are required", "")
		return
	}
	if a.AwardedBy == "" {
		a.AwardedBy = "human"
	}
	created := mywant.AddAchievement(a)
	s.JSONResponse(w, http.StatusCreated, created)
}

// updateAchievement replaces an achievement by ID.
func (s *Server) updateAchievement(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updated mywant.Achievement
	if err := DecodeRequest(r, &updated); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if !mywant.UpdateAchievement(id, updated) {
		s.JSONError(w, r, http.StatusNotFound, "Achievement not found", id)
		return
	}
	a, _ := mywant.GetAchievement(id)
	s.JSONResponse(w, http.StatusOK, a)
}

// lockAchievement sets Unlocked=false on an achievement, deactivating its capability.
func (s *Server) lockAchievement(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	a, ok := mywant.LockAchievement(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Achievement not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, a)
}

// unlockAchievement sets Unlocked=true on an achievement, activating its capability.
func (s *Server) unlockAchievement(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	a, ok := mywant.UnlockAchievement(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Achievement not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, a)
}

// deleteAchievement removes an achievement by ID.
func (s *Server) deleteAchievement(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !mywant.DeleteAchievement(id) {
		s.JSONError(w, r, http.StatusNotFound, "Achievement not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]string{"message": "deleted", "id": id})
}

// ── Rules ─────────────────────────────────────────────────────────────────────

func (s *Server) listAchievementRules(w http.ResponseWriter, r *http.Request) {
	rules := mywant.ListAchievementRules()
	if rules == nil {
		rules = []mywant.AchievementRule{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"rules": rules, "count": len(rules)})
}

func (s *Server) getAchievementRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	rule, ok := mywant.GetAchievementRule(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Rule not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, rule)
}

func (s *Server) createAchievementRule(w http.ResponseWriter, r *http.Request) {
	var rule mywant.AchievementRule
	if err := DecodeRequest(r, &rule); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if rule.Award.Title == "" {
		s.JSONError(w, r, http.StatusBadRequest, "award.title is required", "")
		return
	}
	created := mywant.AddAchievementRule(rule)
	s.JSONResponse(w, http.StatusCreated, created)
}

func (s *Server) updateAchievementRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updated mywant.AchievementRule
	if err := DecodeRequest(r, &updated); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if !mywant.UpdateAchievementRule(id, updated) {
		s.JSONError(w, r, http.StatusNotFound, "Rule not found", id)
		return
	}
	rule, _ := mywant.GetAchievementRule(id)
	s.JSONResponse(w, http.StatusOK, rule)
}

func (s *Server) deleteAchievementRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !mywant.DeleteAchievementRule(id) {
		s.JSONError(w, r, http.StatusNotFound, "Rule not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]string{"message": "deleted", "id": id})
}
