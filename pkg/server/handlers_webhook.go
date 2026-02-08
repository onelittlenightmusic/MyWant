package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	mywant "mywant/engine/src"

	"github.com/gorilla/mux"
)

// teamsMessage represents an incoming Teams webhook message
type teamsMessage struct {
	Sender    string `json:"sender"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	ChannelID string `json:"channel_id"`
}

// receiveWebhook handles POST /api/v1/webhooks/{id}
// The {id} corresponds to a Want ID. Teams Outgoing Webhooks should be
// configured to POST to this URL.
func (s *Server) receiveWebhook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse as generic JSON
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, `{"error":"invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	// Find the Want by ID
	want := s.findWantByID(wantID)
	if want == nil {
		http.Error(w, `{"error":"want not found"}`, http.StatusNotFound)
		return
	}

	// Detect Teams payload by channelId field
	if channelID, ok := payload["channelId"].(string); ok && channelID == "msteams" {
		s.handleTeamsWebhook(w, r, want, body, payload)
		return
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

// findWantByID searches for a Want across globalBuilder and execution builders
func (s *Server) findWantByID(wantID string) *mywant.Want {
	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			return want
		}
	}
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				return want
			}
		}
	}
	return nil
}

// handleTeamsWebhook processes a Teams Outgoing Webhook payload
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

	msg := teamsMessage{
		Sender:    sender,
		Text:      text,
		Timestamp: timestamp,
		ChannelID: channelID,
	}

	// Get existing messages list
	var messages []any
	if existing, ok := want.GetState("teams_messages"); ok {
		if arr, ok := existing.([]any); ok {
			messages = arr
		}
	}

	// Append new message (FIFO, keep last 20)
	msgMap := map[string]any{
		"sender":     msg.Sender,
		"text":       msg.Text,
		"timestamp":  msg.Timestamp,
		"channel_id": msg.ChannelID,
	}
	messages = append(messages, msgMap)
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
	}

	// Get current message count
	var messageCount int
	if countVal, ok := want.GetState("teams_message_count"); ok {
		switch v := countVal.(type) {
		case int:
			messageCount = v
		case float64:
			messageCount = int(v)
		case json.Number:
			if n, err := v.Int64(); err == nil {
				messageCount = int(n)
			}
		}
	}
	messageCount++

	// Update want state
	want.StoreStateMultiForAgent(map[string]any{
		"teams_latest_message": msgMap,
		"teams_messages":       messages,
		"teams_message_count":  messageCount,
		"action_by_agent":      "webhook_handler",
	})

	want.StoreLog("[TEAMS-WEBHOOK] Received message from %s: %s", msg.Sender, msg.Text)

	// Return Teams-compatible response
	json.NewEncoder(w).Encode(map[string]string{
		"type": "message",
		"text": "Received",
	})
}

// verifyTeamsHMAC verifies the HMAC-SHA256 signature from Teams
func verifyTeamsHMAC(body []byte, signature string, secret string) bool {
	// Teams sends the secret as base64-encoded
	secretBytes, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		// Try using secret as raw bytes
		secretBytes = []byte(secret)
	}

	mac := hmac.New(sha256.New, secretBytes)
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSignature := base64.StdEncoding.EncodeToString(expectedMAC)

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// listWebhookEndpoints handles GET /api/v1/webhooks
// Returns all Wants that can receive webhooks
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

	// Scan all wants in the global builder
	allWants := s.globalBuilder.GetAllWantStates()
	for _, want := range allWants {
		if want.Metadata.Type == "teams webhook" {
			status := "active"
			if st, ok := want.GetStateString("teams_webhook_status", ""); ok && st != "" {
				status = st
			}
			endpoints = append(endpoints, webhookEndpoint{
				WantID:   want.Metadata.ID,
				WantName: want.Metadata.Name,
				WantType: want.Metadata.Type,
				URL:      fmt.Sprintf("/api/v1/webhooks/%s", want.Metadata.ID),
				Status:   status,
			})
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
