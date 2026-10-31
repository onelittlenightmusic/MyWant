package mywant

import (
	"sync"
	"testing"
	"time"
)

// What a want is about to say is heard as it will be shown: spaced between
// sentences, trimmed, the empty left out — and a `say` written into its state
// is heard too.
func TestLinesSaidAreHeardAsShown(t *testing.T) {
	var mu sync.Mutex
	var heard []string
	OnLinesSaid = func(lines []string) {
		mu.Lock()
		heard = append(heard, lines...)
		mu.Unlock()
	}
	t.Cleanup(func() { OnLinesSaid = nil })

	voiceLinesSaid([]string{"雪だ！足元に気をつけて", "  "})
	w := &Want{Metadata: Metadata{Name: "w-say"}}
	w.StoreState("say", " ボスが現れた！ ")

	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		n := len(heard)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	want := map[string]bool{"雪だ！ 足元に気をつけて": true, "ボスが現れた！": true}
	if len(heard) != 2 || !want[heard[0]] || !want[heard[1]] {
		t.Errorf("heard %q", heard)
	}
}
