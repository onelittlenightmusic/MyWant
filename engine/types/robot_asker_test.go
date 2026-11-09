package types

import (
	"context"
	"testing"

	. "mywant/engine/core"
)

// A chat message that says who sent it carries that character to the request
// the robot answers, so the turn fmtool opens for it is theirs (the server's
// story.go); one that does not say leaves no one.
func TestRobotWebhookCarriesTheAsker(t *testing.T) {
	for _, c := range []struct {
		name string
		msg  map[string]any
		want string
	}{
		{"said", map[string]any{"text": "中野の天気は？", "sender": "user", "character": "chr-hero"}, "chr-hero"},
		{"not said", map[string]any{"text": "中野の天気は？", "sender": "user"}, ""},
	} {
		w := newCCWant()
		w.StateLabels.Set("webhook_auto_character", LabelCurrent)
		w.StateLabels.Set("cc_webhook_processed", LabelCurrent)
		w.StateLabels.Set("phase", LabelCurrent)
		w.SetCurrent("webhook_auto_character", "chr-stale")
		w.SetCurrent("cc_latest_message", c.msg)
		w.SetCurrent("cc_message_count", 1)
		_ = claudeCodeWatcherThink(context.Background(), w)
		if got := GetCurrent(w, "webhook_auto_request", ""); got == "" {
			t.Fatalf("%s: the message was not taken up", c.name)
		}
		if got := GetCurrent(w, "webhook_auto_character", ""); got != c.want {
			t.Errorf("%s: webhook_auto_character = %q, want %q", c.name, got, c.want)
		}
	}
}
