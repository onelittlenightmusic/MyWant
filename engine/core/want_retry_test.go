package mywant

import (
	"testing"
	"time"
)

func TestIsTransientFailure(t *testing.T) {
	for _, e := range []string{
		`天気情報を取得できませんでした: open-meteo: Get "https://x": dial tcp: lookup geocoding-api.open-meteo.com: no such host`,
		`Yahoo!路線情報に接続できません: [Errno -2] Name does not resolve`,
		`context deadline exceeded`, `HTTP Error 504: Gateway Timeout`, `runner at http://127.0.0.1:8090 is not answering`,
	} {
		if !IsTransientFailure(e) {
			t.Errorf("not taken as passing: %s", e)
		}
	}
	for _, e := range []string{"", "invalid parameter 'to'", "exit status 1\nstderr: KeyError: 'foo'", "ルートが見つかりませんでした"} {
		if IsTransientFailure(e) {
			t.Errorf("taken as passing: %q", e)
		}
	}
}

func TestPassingFailureIsRunAgainThenCleared(t *testing.T) {
	w := &Want{Metadata: Metadata{ID: "want-retry", Name: "weather"}}
	w.State.Store(StateFieldError, "dial tcp: lookup x: no such host")
	before := time.Now()
	w.SetStatus(WantStatusFailed)
	at, ok := wakeOf(w)
	if !ok || at.Before(before.Add(50*time.Second)) || at.After(before.Add(70*time.Second)) {
		t.Fatalf("first wake %v, want about a minute on", at)
	}
	if c, _ := w.GetStateInt(RetryCountState, 0); c != 1 || !StillRetrying(w, time.Now()) {
		t.Fatalf("count %d, retrying %v", c, StillRetrying(w, time.Now()))
	}
	if StillRetrying(w, time.Now().Add(2*time.Hour)) {
		t.Fatal("still not a person's after two hours of failing")
	}
	w.SetStatus(WantStatusReaching)
	w.SetStatus(WantStatusFailed)
	if at2, _ := wakeOf(w); at2.Sub(time.Now()) < 4*time.Minute {
		t.Fatalf("second wake %v, want about five minutes on", at2)
	}
	w.SetStatus(WantStatusAchieved)
	if c, _ := w.GetStateInt(RetryCountState, 0); c != 0 || StillRetrying(w, time.Now()) {
		t.Fatal("an answer did not clear the failing")
	}

	other := &Want{Metadata: Metadata{ID: "want-cfg"}}
	other.State.Store(StateFieldError, "invalid parameter 'to'")
	other.SetStatus(WantStatusFailed)
	if _, ok := wakeOf(other); ok || StillRetrying(other, time.Now()) {
		t.Fatal("a failure only a person can mend was run again")
	}
}
