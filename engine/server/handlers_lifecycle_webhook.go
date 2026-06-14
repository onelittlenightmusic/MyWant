package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// Ensure time is used (for postLifecycleEvent timeout)
var _ = time.Second

// ── Types ────────────────────────────────────────────────────────────────────

// LifecycleWebhookConfig holds a registered notification endpoint (unfiltered).
type LifecycleWebhookConfig struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// LifecycleWebhookPayload is the JSON body POSTed to unfiltered endpoints.
type LifecycleWebhookPayload struct {
	Event string            `json:"event"`
	Want  LifecycleWantInfo `json:"want"`
	Rule  *LifecycleRuleRef `json:"rule,omitempty"` // set when sent by a filtered rule
}

// LifecycleRuleRef carries the matched rule's ID and action metadata in the payload.
type LifecycleRuleRef struct {
	ID       string         `json:"id"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// LifecycleWantInfo is the want metadata included in the notification.
type LifecycleWantInfo struct {
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	Type            string                    `json:"type"`
	Status          string                    `json:"status,omitempty"`
	Labels          map[string]string         `json:"labels,omitempty"`
	OwnerReferences []LifecycleOwnerReference `json:"owner_references,omitempty"`
	FinalResult     any                       `json:"final_result,omitempty"`
}

// LifecycleOwnerReference is a simplified ownerReference for the notification payload.
type LifecycleOwnerReference struct {
	Name       string `json:"name"`
	ID         string `json:"id"`
	Controller bool   `json:"controller"`
}

// ── Filtered Rule types ───────────────────────────────────────────────────────

// LifecycleWebhookRule is a filtered subscription: mywant only calls TargetURL
// when the event and match criteria are satisfied.  Metadata is forwarded to the
// caller so it can embed action instructions (e.g. RPG activate/run fields).
type LifecycleWebhookRule struct {
	ID        string               `json:"id"`
	Event     string               `json:"event"`              // "want_created", "want_achieved", "want_deleted"
	Match     mywant.LifecycleRuleMatch `json:"match"`
	TargetURL string               `json:"target_url"`
	Metadata  map[string]any       `json:"metadata,omitempty"` // forwarded to TargetURL as rule.metadata
}

// ── Registry ──────────────────────────────────────────────────────────────────

var (
	lifecycleMu    sync.RWMutex
	lifecycleHooks []LifecycleWebhookConfig
	lifecycleRules []LifecycleWebhookRule
)

// registerLifecycleRuleInternal adds a filtered rule and returns its ID.
func registerLifecycleRuleInternal(event, targetURL string, match mywant.LifecycleRuleMatch, metadata map[string]any) string {
	rule := LifecycleWebhookRule{
		ID:        generateWantID(),
		Event:     event,
		Match:     match,
		TargetURL: targetURL,
		Metadata:  metadata,
	}
	lifecycleMu.Lock()
	lifecycleRules = append(lifecycleRules, rule)
	lifecycleMu.Unlock()
	log.Printf("[LIFECYCLE-RULE] registered id=%s event=%s target=%s", rule.ID, rule.Event, rule.TargetURL)
	return rule.ID
}

// unregisterLifecycleRuleInternal removes a filtered rule by ID.
func unregisterLifecycleRuleInternal(id string) bool {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	var next []LifecycleWebhookRule
	found := false
	for _, r := range lifecycleRules {
		if r.ID == id {
			found = true
		} else {
			next = append(next, r)
		}
	}
	lifecycleRules = next
	if found {
		log.Printf("[LIFECYCLE-RULE] unregistered id=%s", id)
	}
	return found
}

// wireRuleGlobals binds the core package's global function vars to this server's
// in-process implementations.  Called once at server startup so want agents can
// register lifecycle rules without import cycles.
func wireRuleGlobals() {
	mywant.RegisterLifecycleRule = registerLifecycleRuleInternal
	mywant.UnregisterLifecycleRule = unregisterLifecycleRuleInternal
}

// ── HTTP handlers: unfiltered webhooks ───────────────────────────────────────

// POST /api/v1/lifecycle-webhooks
func (s *Server) registerLifecycleWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		s.JSONError(w, r, http.StatusBadRequest, "url is required", "")
		return
	}

	id := generateWantID()
	cfg := LifecycleWebhookConfig{ID: id, URL: body.URL}

	lifecycleMu.Lock()
	lifecycleHooks = append(lifecycleHooks, cfg)
	lifecycleMu.Unlock()

	log.Printf("[LIFECYCLE-WEBHOOK] registered id=%s url=%s", id, body.URL)
	s.JSONResponse(w, http.StatusCreated, map[string]any{"id": id, "url": body.URL})
}

// GET /api/v1/lifecycle-webhooks
func (s *Server) listLifecycleWebhooks(w http.ResponseWriter, r *http.Request) {
	lifecycleMu.RLock()
	out := make([]LifecycleWebhookConfig, len(lifecycleHooks))
	copy(out, lifecycleHooks)
	lifecycleMu.RUnlock()
	s.JSONResponse(w, http.StatusOK, map[string]any{"webhooks": out})
}

// DELETE /api/v1/lifecycle-webhooks/{id}
func (s *Server) deleteLifecycleWebhook(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	lifecycleMu.Lock()
	var next []LifecycleWebhookConfig
	found := false
	for _, h := range lifecycleHooks {
		if h.ID == id {
			found = true
		} else {
			next = append(next, h)
		}
	}
	lifecycleHooks = next
	lifecycleMu.Unlock()

	if !found {
		s.JSONError(w, r, http.StatusNotFound, "webhook not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// ── HTTP handlers: filtered rules ─────────────────────────────────────────────

// POST /api/v1/lifecycle-webhooks/rules
func (s *Server) registerLifecycleRule(w http.ResponseWriter, r *http.Request) {
	var rule LifecycleWebhookRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid rule body", err.Error())
		return
	}
	if rule.Event == "" {
		s.JSONError(w, r, http.StatusBadRequest, "event is required", "")
		return
	}
	if rule.TargetURL == "" {
		s.JSONError(w, r, http.StatusBadRequest, "target_url is required", "")
		return
	}
	id := registerLifecycleRuleInternal(rule.Event, rule.TargetURL, rule.Match, rule.Metadata)
	s.JSONResponse(w, http.StatusCreated, map[string]any{"id": id})
}

// GET /api/v1/lifecycle-webhooks/rules
func (s *Server) listLifecycleRules(w http.ResponseWriter, r *http.Request) {
	lifecycleMu.RLock()
	out := make([]LifecycleWebhookRule, len(lifecycleRules))
	copy(out, lifecycleRules)
	lifecycleMu.RUnlock()
	s.JSONResponse(w, http.StatusOK, map[string]any{"rules": out})
}

// DELETE /api/v1/lifecycle-webhooks/rules/{id}
func (s *Server) deleteLifecycleRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !unregisterLifecycleRuleInternal(id) {
		s.JSONError(w, r, http.StatusNotFound, "rule not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// ── Notification ─────────────────────────────────────────────────────────────

// notifyWantCreated fires lifecycle notifications for each newly created want.
// Sends to all unfiltered webhooks AND any matching filtered rules.
func (s *Server) notifyWantCreated(wants []*mywant.Want) {
	lifecycleMu.RLock()
	hooks := make([]LifecycleWebhookConfig, len(lifecycleHooks))
	copy(hooks, lifecycleHooks)
	rules := make([]LifecycleWebhookRule, len(lifecycleRules))
	copy(rules, lifecycleRules)
	lifecycleMu.RUnlock()

	for _, w := range wants {
		payload := buildPayload(w)

		// Unfiltered: all endpoints
		for _, h := range hooks {
			go postLifecycleEvent(h.URL, payload)
		}

		// Filtered: only matching rules
		for _, rule := range rules {
			if rule.Event != "want_created" {
				continue
			}
			if !ruleMatchesWant(rule.Match, w) {
				continue
			}
			rulePayload := payload
			rulePayload.Rule = &LifecycleRuleRef{ID: rule.ID, Metadata: rule.Metadata}
			go postLifecycleEvent(rule.TargetURL, rulePayload)
		}
	}
}

func buildPayload(w *mywant.Want) LifecycleWebhookPayload {
	return buildPayloadWithEvent("want_created", w)
}

func buildPayloadWithEvent(event string, w *mywant.Want) LifecycleWebhookPayload {
	info := LifecycleWantInfo{
		ID:     w.Metadata.ID,
		Name:   w.Metadata.Name,
		Type:   w.Metadata.Type,
		Status: string(w.GetStatus()),
		Labels: w.Metadata.Labels,
	}
	for _, ref := range w.Metadata.OwnerReferences {
		info.OwnerReferences = append(info.OwnerReferences, LifecycleOwnerReference{
			Name:       ref.Name,
			ID:         ref.ID,
			Controller: ref.Controller,
		})
	}
	if fr, ok := w.GetAllState()["final_result"]; ok {
		info.FinalResult = fr
	}
	return LifecycleWebhookPayload{Event: event, Want: info}
}

// RegisterAchievementCallback wires mywant core's OnWantAchieved global to fire
// want_achieved lifecycle webhooks and filtered rules.  Called once during server startup.
func (s *Server) RegisterAchievementCallback() {
	mywant.OnWantAchieved = func(w *mywant.Want) {
		lifecycleMu.RLock()
		hooks := make([]LifecycleWebhookConfig, len(lifecycleHooks))
		copy(hooks, lifecycleHooks)
		rules := make([]LifecycleWebhookRule, len(lifecycleRules))
		copy(rules, lifecycleRules)
		lifecycleMu.RUnlock()

		payload := buildPayloadWithEvent("want_achieved", w)
		log.Printf("[LIFECYCLE-WEBHOOK] want_achieved: %s (%s)", w.Metadata.Name, w.Metadata.ID)

		// Unfiltered
		for _, h := range hooks {
			go postLifecycleEvent(h.URL, payload)
		}

		// Filtered rules
		for _, rule := range rules {
			if rule.Event != "want_achieved" {
				continue
			}
			if !ruleMatchesWant(rule.Match, w) {
				continue
			}
			rulePayload := payload
			rulePayload.Rule = &LifecycleRuleRef{ID: rule.ID, Metadata: rule.Metadata}
			go postLifecycleEvent(rule.TargetURL, rulePayload)
		}
	}
}

// ruleMatchesWant returns true when all non-empty match criteria are satisfied.
func ruleMatchesWant(match mywant.LifecycleRuleMatch, w *mywant.Want) bool {
	if match.Name != "" && w.Metadata.Name != match.Name {
		return false
	}
	if match.Type != "" && w.Metadata.Type != match.Type {
		return false
	}
	if match.Owner != "" {
		found := false
		for _, ref := range w.Metadata.OwnerReferences {
			if ref.Controller && ref.Name == match.Owner {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for k, v := range match.Labels {
		if w.Metadata.Labels[k] != v {
			return false
		}
	}
	return true
}

// DELETE /api/v1/lifecycle-webhooks/achievement-cache — no-op kept for compat
func (s *Server) resetAchievementCache(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("no-op: achievement notifications are now push-based")})
}

func postLifecycleEvent(url string, payload LifecycleWebhookPayload) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[LIFECYCLE-WEBHOOK] marshal error: %v", err)
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[LIFECYCLE-WEBHOOK] POST %s error: %v", url, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("[LIFECYCLE-WEBHOOK] POST %s returned %d", url, resp.StatusCode)
	}
}
