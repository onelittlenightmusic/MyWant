package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// receiveWebhook handles POST /api/v1/webhooks/{id}
func (s *Server) receiveWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "failed to read request body", err.Error())
		return
	}
	defer r.Body.Close()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid JSON payload", err.Error())
		return
	}

	// [replay webhook] Detect -start / -stop suffix for replay want type recording control
	if want, action := s.findWantAndActionByWebhookID(wantID); want != nil {
		s.handleReplayWebhook(w, want, action)
		return
	}

	want := s.findWantByIDOrName(wantID)
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "want not found", "")
		return
	}

	// Claude Code webhook: structured message handling
	if want.Metadata.Type == "claude_code_thread" {
		text, _ := payload["text"].(string)
		sender, _ := payload["sender"].(string)
		storeWebhookMessage(want, webhookMessage{
			Sender:    sender,
			Text:      text,
			Timestamp: time.Now().Format(time.RFC3339),
		}, ccStateCfg)
		log.Printf("[CC-WEBHOOK] Received request for want %s: %s\n", wantID, text)
		s.JSONResponse(w, http.StatusOK, map[string]string{"status": "received"})
		return
	}

	// Generic webhook receiver: handle challenge, verify, and store raw payload.
	// verify_type / challenge_field / secret_param are want params so that platform-specific
	// YAML types (teams_notify, slack_notify, …) supply their own defaults without Go changes.
	s.handleWebhookReceiver(w, r, want, body, payload)
}

// handleWebhookReceiver is the single generic handler for all webhook_receiver-based want types.
// It handles URL verification challenges, HMAC / Slack-signing verification, and raw payload storage.
func (s *Server) handleWebhookReceiver(w http.ResponseWriter, r *http.Request, want *mywant.Want, body []byte, payload map[string]any) {
	// Handle challenge before signature check (e.g. Slack URL verification)
	challengeField := want.GetStringParam("challenge_field", "")
	if challengeField != "" {
		if payloadType, _ := payload["type"].(string); payloadType == "url_verification" {
			challenge, _ := payload[challengeField].(string)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, challenge)
			return
		}
	}

	// Signature verification driven by verify_type param
	verifyType := want.GetStringParam("verify_type", "none")
	secretParam := want.GetStringParam("secret_param", "webhook_secret")
	secret := want.GetStringParam(secretParam, "")
	if secret != "" {
		switch verifyType {
		case "hmac_sha256":
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "HMAC ") {
				sig := strings.TrimPrefix(authHeader, "HMAC ")
				if !verifyTeamsHMAC(body, sig, secret) {
					http.Error(w, `{"error":"invalid signature"}`, http.StatusUnauthorized)
					return
				}
			}
		case "slack_signing":
			slackSig := r.Header.Get("X-Slack-Signature")
			slackTS := r.Header.Get("X-Slack-Request-Timestamp")
			if slackSig != "" && slackTS != "" {
				if !verifySlackSignature(body, slackSig, slackTS, secret) {
					http.Error(w, `{"error":"invalid signature"}`, http.StatusUnauthorized)
					return
				}
			}
		}
	}

	// Store raw payload in FIFO buffer using state_prefix param
	statePrefix := want.GetStringParam("state_prefix", "webhook")
	cfg := webhookStateConfig{
		LatestMessageKey: statePrefix + "_latest_message",
		MessagesKey:      statePrefix + "_messages",
		MessageCountKey:  statePrefix + "_message_count",
		LogPrefix:        "[WEBHOOK-RECEIVER:" + statePrefix + "]",
	}
	storeRawWebhookPayload(want, payload, cfg)

	s.JSONResponse(w, http.StatusOK, map[string]string{"status": "received"})
}

// findWantAndActionByWebhookID detects -start / -stop / -debug-start / -debug-stop suffixes
// used by replay want type. Returns the matching Want and action string, or (nil, "") if no match.
func (s *Server) findWantAndActionByWebhookID(id string) (*mywant.Want, string) {
	// Check longer suffixes first to avoid ambiguity
	if strings.HasSuffix(id, "-replay") {
		baseID := strings.TrimSuffix(id, "-replay")
		if want := s.findWantByIDOrName(baseID); want != nil {
			return want, "start_replay"
		}
	}
	if strings.HasSuffix(id, "-debug-start") {
		baseID := strings.TrimSuffix(id, "-debug-start")
		if want := s.findWantByIDOrName(baseID); want != nil {
			return want, "start_debug_recording"
		}
	}
	if strings.HasSuffix(id, "-debug-stop") {
		baseID := strings.TrimSuffix(id, "-debug-stop")
		if want := s.findWantByIDOrName(baseID); want != nil {
			return want, "stop_debug_recording"
		}
	}
	if strings.HasSuffix(id, "-start") {
		baseID := strings.TrimSuffix(id, "-start")
		if want := s.findWantByIDOrName(baseID); want != nil {
			return want, "start_recording"
		}
	}
	if strings.HasSuffix(id, "-stop") {
		baseID := strings.TrimSuffix(id, "-stop")
		if want := s.findWantByIDOrName(baseID); want != nil {
			return want, "stop_recording"
		}
	}
	return nil, ""
}

// handleReplayWebhook sets start/stop recording signal flags on a replay want's state.
func (s *Server) handleReplayWebhook(w http.ResponseWriter, want *mywant.Want, action string) {
	switch action {
	case "start_recording":
		mywant.StoreStateMulti(want, map[string]any{
			"start_recording_requested": true,
			"stop_recording_requested":  false,
			"action_by_agent":           "webhook_handler",
		})
		log.Printf("[REPLAY-WEBHOOK] start_recording signal set for want %s\n", want.Metadata.ID)
	case "stop_recording":
		mywant.StoreStateMulti(want, map[string]any{
			"stop_recording_requested": true,
			"action_by_agent":          "webhook_handler",
		})
		log.Printf("[REPLAY-WEBHOOK] stop_recording signal set for want %s\n", want.Metadata.ID)
	case "start_debug_recording":
		mywant.StoreStateMulti(want, map[string]any{
			"start_debug_recording_requested": true,
			"stop_debug_recording_requested":  false,
			"action_by_agent":                 "webhook_handler",
		})
		log.Printf("[REPLAY-WEBHOOK] start_debug_recording signal set for want %s\n", want.Metadata.ID)
	case "stop_debug_recording":
		mywant.StoreStateMulti(want, map[string]any{
			"stop_debug_recording_requested": true,
			"action_by_agent":                "webhook_handler",
		})
		log.Printf("[REPLAY-WEBHOOK] stop_debug_recording signal set for want %s\n", want.Metadata.ID)
	case "start_replay":
		mywant.StoreStateMulti(want, map[string]any{
			"start_replay_requested": true,
			"action_by_agent":        "webhook_handler",
		})
		log.Printf("[REPLAY-WEBHOOK] start_replay signal set for want %s\n", want.Metadata.ID)
	}
	s.JSONResponse(w, http.StatusOK, map[string]string{"status": "ok", "action": action})
}

// findWantByIDOrName searches for a Want across globalBuilder and execution builders by ID or Name
func (s *Server) findWantByIDOrName(idOrName string) *mywant.Want {
	if s.globalBuilder != nil {
		// Try by ID first
		if want, _, found := s.globalBuilder.FindWantByID(idOrName); found {
			return want
		}
		// Try by Name
		if want, found := s.globalBuilder.FindWantByName(idOrName); found {
			return want
		}
	}
	for _, execution := range s.wants {
		if execution.Builder != nil {
			// Try by ID first
			if want, _, found := execution.Builder.FindWantByID(idOrName); found {
				return want
			}
			// Try by Name
			if want, found := execution.Builder.FindWantByName(idOrName); found {
				return want
			}
		}
	}
	return nil
}

// verifyTeamsHMAC verifies the HMAC-SHA256 signature from Teams
func verifyTeamsHMAC(body []byte, signature string, secret string) bool {
	secretBytes, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		secretBytes = []byte(secret)
	}

	mac := hmac.New(sha256.New, secretBytes)
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSignature := base64.StdEncoding.EncodeToString(expectedMAC)

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// verifySlackSignature verifies the Slack request signature.
// Slack computes: v0={timestamp}:{body} → HMAC-SHA256 with signing secret → hex
// The signature header is "v0=<hex>".
func verifySlackSignature(body []byte, signature, timestamp, secret string) bool {
	// Reject requests older than 5 minutes to prevent replay attacks
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if abs(time.Now().Unix()-ts) > 300 {
		return false
	}

	baseString := "v0:" + timestamp + ":" + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(baseString))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// --- List Webhook Endpoints ---

// listWebhookEndpoints handles GET /api/v1/webhooks
// Any want that has a non-empty "webhook_url" state is treated as an active endpoint.
func (s *Server) listWebhookEndpoints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type webhookEndpoint struct {
		WantID   string `json:"want_id"`
		WantName string `json:"want_name"`
		WantType string `json:"want_type"`
		URL      string `json:"url"`
		Status   string `json:"status"`
	}

	var endpoints []webhookEndpoint

	allWants := s.globalBuilder.GetAllWantStates()
	for _, want := range allWants {
		webhookURL, hasURL := want.GetStateString("webhook_url", "")
		if !hasURL || webhookURL == "" {
			continue
		}
		statePrefix := want.GetStringParam("state_prefix", "webhook")
		statusKey := statePrefix + "_webhook_status"
		status := "active"
		if st, ok := want.GetStateString(statusKey, ""); ok && st != "" {
			status = st
		}
		endpoints = append(endpoints, webhookEndpoint{
			WantID:   want.Metadata.ID,
			WantName: want.Metadata.Name,
			WantType: want.Metadata.Type,
			URL:      webhookURL,
			Status:   status,
		})
	}

	if endpoints == nil {
		endpoints = []webhookEndpoint{}
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"endpoints": endpoints,
		"count":     len(endpoints),
	})
}
