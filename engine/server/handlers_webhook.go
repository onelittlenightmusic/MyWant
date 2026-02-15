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

// --- Teams state config ---

var teamsStateCfg = webhookStateConfig{
	LatestMessageKey: "teams_latest_message",
	MessagesKey:      "teams_messages",
	MessageCountKey:  "teams_message_count",
	LogPrefix:        "[TEAMS-WEBHOOK]",
}

// --- Slack state config ---

var slackStateCfg = webhookStateConfig{
	LatestMessageKey: "slack_latest_message",
	MessagesKey:      "slack_messages",
	MessageCountKey:  "slack_message_count",
	LogPrefix:        "[SLACK-WEBHOOK]",
}

// receiveWebhook handles POST /api/v1/webhooks/{id}
func (s *Server) receiveWebhook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, `{"error":"invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	want := s.findWantByIDOrName(wantID)
	if want == nil {
		http.Error(w, `{"error":"want not found"}`, http.StatusNotFound)
		return
	}

	// Detect Teams payload by channelId == "msteams"
	if channelID, ok := payload["channelId"].(string); ok && channelID == "msteams" {
		s.handleTeamsWebhook(w, r, want, body, payload)
		return
	}

	// Detect Slack payload by type field
	if payloadType, ok := payload["type"].(string); ok {
		if payloadType == "url_verification" || payloadType == "event_callback" {
			s.handleSlackWebhook(w, r, want, body, payload)
			return
		}
	}

	// Generic webhook: store payload as-is
	want.StoreStateMultiForAgent(map[string]any{
		"webhook_payload":     payload,
		"webhook_received_at": time.Now().Format(time.RFC3339),
		"action_by_agent":     "webhook_handler",
	})

	log.Printf("[WEBHOOK] Received generic webhook for want %s\n", wantID)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "received",
	})
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

// --- Teams Webhook Handler ---

func (s *Server) handleTeamsWebhook(w http.ResponseWriter, r *http.Request, want *mywant.Want, body []byte, payload map[string]any) {
	// HMAC-SHA256 signature verification
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "HMAC ") {
		signature := strings.TrimPrefix(authHeader, "HMAC ")
		secret, _ := want.GetStateString("webhook_secret", "")
		if secret != "" {
			if !verifyTeamsHMAC(body, signature, secret) {
				http.Error(w, `{"error":"invalid signature"}`, http.StatusUnauthorized)
				return
			}
		}
	}

	// Extract message fields from Teams payload
	sender := ""
	if from, ok := payload["from"].(map[string]any); ok {
		if name, ok := from["name"].(string); ok {
			sender = name
		}
	}
	text, _ := payload["text"].(string)
	timestamp, _ := payload["timestamp"].(string)
	if timestamp == "" {
		timestamp = time.Now().Format(time.RFC3339)
	}
	channelID := ""
	if conversation, ok := payload["conversation"].(map[string]any); ok {
		if id, ok := conversation["id"].(string); ok {
			channelID = id
		}
	}

	storeWebhookMessage(want, webhookMessage{
		Sender:    sender,
		Text:      text,
		Timestamp: timestamp,
		ChannelID: channelID,
	}, teamsStateCfg)

	json.NewEncoder(w).Encode(map[string]string{
		"type": "message",
		"text": "Received",
	})
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

// --- Slack Webhook Handler ---

func (s *Server) handleSlackWebhook(w http.ResponseWriter, r *http.Request, want *mywant.Want, body []byte, payload map[string]any) {
	payloadType, _ := payload["type"].(string)

	// Handle URL verification challenge (must respond before signature check
	// so that Slack can verify the endpoint during initial setup)
	if payloadType == "url_verification" {
		challenge, _ := payload["challenge"].(string)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, challenge)
		return
	}

	// Slack signature verification (event_callback and other events)
	slackSignature := r.Header.Get("X-Slack-Signature")
	slackTimestamp := r.Header.Get("X-Slack-Request-Timestamp")
	if slackSignature != "" && slackTimestamp != "" {
		secret, _ := want.GetStateString("webhook_secret", "")
		if secret != "" {
			if !verifySlackSignature(body, slackSignature, slackTimestamp, secret) {
				http.Error(w, `{"error":"invalid signature"}`, http.StatusUnauthorized)
				return
			}
		}
	}

	// Handle event_callback
	if payloadType != "event_callback" {
		http.Error(w, `{"error":"unsupported payload type"}`, http.StatusBadRequest)
		return
	}

	event, ok := payload["event"].(map[string]any)
	if !ok {
		http.Error(w, `{"error":"missing event field"}`, http.StatusBadRequest)
		return
	}

	// Only process message events (skip subtypes like bot_message, message_changed, etc.)
	eventType, _ := event["type"].(string)
	if eventType != "message" {
		json.NewEncoder(w).Encode(map[string]string{"status": "ignored"})
		return
	}
	if _, hasSubtype := event["subtype"]; hasSubtype {
		json.NewEncoder(w).Encode(map[string]string{"status": "ignored"})
		return
	}

	user, _ := event["user"].(string)
	text, _ := event["text"].(string)
	channel, _ := event["channel"].(string)
	ts, _ := event["ts"].(string)

	timestamp := ts
	if ts != "" {
		// Convert Slack timestamp (e.g. "1234567890.123456") to RFC3339
		parts := strings.Split(ts, ".")
		if sec, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
			timestamp = time.Unix(sec, 0).Format(time.RFC3339)
		}
	}
	if timestamp == "" {
		timestamp = time.Now().Format(time.RFC3339)
	}

	storeWebhookMessage(want, webhookMessage{
		Sender:    user,
		Text:      text,
		Timestamp: timestamp,
		ChannelID: channel,
	}, slackStateCfg)

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
		for _, wt := range webhookTypes {
			if want.Metadata.Type == wt {
				statusKey := strings.Replace(wt, " ", "_", 1) + "_status"
				status := "active"
				if st, ok := want.GetStateString(statusKey, ""); ok && st != "" {
					status = st
				}
				endpoints = append(endpoints, webhookEndpoint{
					WantID:   want.Metadata.ID,
					WantName: want.Metadata.Name,
					WantType: want.Metadata.Type,
					URL:      fmt.Sprintf("/api/v1/webhooks/%s", want.Metadata.Name),
					Status:   status,
				})
				break
			}
		}
	}

	if endpoints == nil {
		endpoints = []webhookEndpoint{}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"endpoints": endpoints,
		"count":     len(endpoints),
	})
}
