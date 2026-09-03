package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Web Push for per-want alerts.
//
// The board already badges a want's tile (see handlers_notifications.go); this
// carries the same alert to a phone whose home screen holds the /w/<id> app,
// even when it is closed. The browser registers a service worker (web/public/
// sw.js), subscribes via the Push API with our VAPID public key, and POSTs the
// resulting subscription here; sendWantPush signs a message to each relevant
// subscription when an alert is raised.

type vapidKeys struct {
	Public  string `json:"public"`
	Private string `json:"private"`
	Subject string `json:"subject"`
}

// loadVAPIDKeys resolves the signing keypair: MYWANT_VAPID_PUBLIC /
// MYWANT_VAPID_PRIVATE if set, otherwise a pair generated once and kept in
// ~/.mywant/vapid.json so it survives restarts (a changed key silently breaks
// every existing subscription until each browser re-subscribes).
func loadVAPIDKeys() vapidKeys {
	subject := os.Getenv("MYWANT_VAPID_SUBJECT")
	if subject == "" {
		subject = "mailto:admin@mywant.local"
	}

	if pub, priv := os.Getenv("MYWANT_VAPID_PUBLIC"), os.Getenv("MYWANT_VAPID_PRIVATE"); pub != "" && priv != "" {
		return vapidKeys{Public: pub, Private: priv, Subject: subject}
	}

	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".mywant", "vapid.json")
	if b, err := os.ReadFile(path); err == nil {
		var k vapidKeys
		if json.Unmarshal(b, &k) == nil && k.Public != "" && k.Private != "" {
			k.Subject = subject
			return k
		}
	}

	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Printf("[push] could not generate VAPID keys: %v — Web Push disabled", err)
		return vapidKeys{Subject: subject}
	}
	k := vapidKeys{Public: pub, Private: priv, Subject: subject}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		if b, mErr := json.Marshal(k); mErr == nil {
			_ = os.WriteFile(path, b, 0o600)
		}
	}
	log.Printf("[push] generated a new VAPID keypair at %s", path)
	return k
}

func (s *Server) pushEnabled() bool {
	return s.vapid.Public != "" && s.vapid.Private != ""
}

// GET /api/v1/push/vapid-public-key
func (s *Server) getVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"key":     s.vapid.Public,
		"enabled": s.pushEnabled(),
	})
}

// POST /api/v1/push/subscribe — body { subscription: <PushSubscriptionJSON>, wantId?, characterId? }
func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Subscription struct {
			Endpoint string `json:"endpoint"`
			Keys     struct {
				P256dh string `json:"p256dh"`
				Auth   string `json:"auth"`
			} `json:"keys"`
		} `json:"subscription"`
		WantID      string `json:"wantId"`
		CharacterID string `json:"characterId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Subscription.Endpoint == "" {
		s.JSONError(w, r, http.StatusBadRequest, "a subscription with an endpoint is required", "")
		return
	}
	if err := s.pushStore.Add(PushSubscription{
		Endpoint:    body.Subscription.Endpoint,
		P256dh:      body.Subscription.Keys.P256dh,
		Auth:        body.Subscription.Keys.Auth,
		WantID:      body.WantID,
		CharacterID: body.CharacterID,
	}); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to store subscription", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusCreated, map[string]any{"ok": true})
}

// POST /api/v1/push/unsubscribe — body { endpoint }
func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Endpoint == "" {
		s.JSONError(w, r, http.StatusBadRequest, "endpoint is required", "")
		return
	}
	if err := s.pushStore.Remove(body.Endpoint); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to remove subscription", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"ok": true})
}

// sendWantPush signs and delivers one alert to every subscription that covers
// wantID. Best-effort and asynchronous per subscription; a 404/410 from the
// push service means the browser is gone, so the subscription is dropped.
func (s *Server) sendWantPush(wantID, title, body, url string) {
	if !s.pushEnabled() {
		return
	}
	subs := s.pushStore.ForWant(wantID)
	if len(subs) == 0 {
		return
	}

	_, unreadTotal := s.notificationTotals()
	payload, _ := json.Marshal(map[string]any{
		"title":      title,
		"body":       body,
		"url":        url,
		"wantId":     wantID,
		"badgeCount": unreadTotal,
	})

	for _, sub := range subs {
		sub := sub
		go func() {
			resp, err := webpush.SendNotification(payload, &webpush.Subscription{
				Endpoint: sub.Endpoint,
				Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
			}, &webpush.Options{
				Subscriber:      s.vapid.Subject,
				VAPIDPublicKey:  s.vapid.Public,
				VAPIDPrivateKey: s.vapid.Private,
				TTL:             60,
			})
			if err != nil {
				log.Printf("[push] send to %.40s… failed: %v", sub.Endpoint, err)
				return
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
				_ = s.pushStore.Remove(sub.Endpoint)
			}
		}()
	}
}

// notificationTotals returns (per-want counts, grand total) of unread alerts.
func (s *Server) notificationTotals() (map[string]int, int) {
	counts := s.notifications.UnreadWantCounts()
	total := 0
	for _, c := range counts {
		total += c
	}
	return counts, total
}
