package server

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// maxPushSubscriptions caps the on-disk subscription list. Oldest dropped first.
const maxPushSubscriptions = 500

// PushSubscription is one browser's Web Push registration.
//
// WantID scopes it: "" means "every want alert", a want id means "only that
// want" — which is what the /w/<id> home-screen app subscribes with, so the
// per-want app only buzzes for its own want.
type PushSubscription struct {
	Endpoint    string `json:"endpoint"    yaml:"endpoint"`
	P256dh      string `json:"p256dh"      yaml:"p256dh"`
	Auth        string `json:"auth"        yaml:"auth"`
	WantID      string `json:"wantId,omitempty"      yaml:"wantId,omitempty"`
	CharacterID string `json:"characterId,omitempty" yaml:"characterId,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"   yaml:"createdAt,omitempty"`
}

// PushStore persists subscriptions to ~/.mywant/push-subscriptions.yaml.
// Same load-modify-save-under-a-mutex shape as NotificationStore.
type PushStore struct {
	path string
	mu   sync.Mutex
}

func newPushStore() *PushStore {
	home, _ := os.UserHomeDir()
	return &PushStore{path: filepath.Join(home, ".mywant", "push-subscriptions.yaml")}
}

func (p *PushStore) load() []PushSubscription {
	bytes, err := os.ReadFile(p.path)
	if err != nil {
		return nil
	}
	var subs []PushSubscription
	_ = yaml.Unmarshal(bytes, &subs)
	return subs
}

func (p *PushStore) save(subs []PushSubscription) error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return err
	}
	bytes, err := yaml.Marshal(subs)
	if err != nil {
		return err
	}
	return os.WriteFile(p.path, bytes, 0o644)
}

// Add stores a subscription, replacing any existing one with the same endpoint
// (a browser re-subscribing keeps one row, with its latest scope).
func (p *PushStore) Add(sub PushSubscription) error {
	if sub.Endpoint == "" {
		return nil
	}
	if sub.CreatedAt == "" {
		sub.CreatedAt = time.Now().Format(time.RFC3339)
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	subs := p.load()
	next := make([]PushSubscription, 0, len(subs)+1)
	for _, s := range subs {
		if s.Endpoint != sub.Endpoint {
			next = append(next, s)
		}
	}
	next = append(next, sub)
	if len(next) > maxPushSubscriptions {
		next = next[len(next)-maxPushSubscriptions:]
	}
	return p.save(next)
}

// Remove drops the subscription with this endpoint (called on unsubscribe and
// when the push service reports it gone).
func (p *PushStore) Remove(endpoint string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	subs := p.load()
	next := make([]PushSubscription, 0, len(subs))
	changed := false
	for _, s := range subs {
		if s.Endpoint == endpoint {
			changed = true
			continue
		}
		next = append(next, s)
	}
	if !changed {
		return nil
	}
	return p.save(next)
}

// ForWant returns the subscriptions that should receive an alert for wantID:
// the want-scoped ones for that want, plus the unscoped "everything" ones.
func (p *PushStore) ForWant(wantID string) []PushSubscription {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []PushSubscription
	for _, s := range p.load() {
		if s.WantID == "" || s.WantID == wantID {
			out = append(out, s)
		}
	}
	return out
}

// All returns every stored subscription.
func (p *PushStore) All() []PushSubscription {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.load()
}
