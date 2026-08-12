package types

import "testing"

func TestClassifyClaudeSessionState(t *testing.T) {
	cases := []struct {
		name    string
		entries []sessionEntry
		want    string
	}{
		{"empty", nil, "idle"},
		{"user last", []sessionEntry{{Role: "user", Content: "hi"}}, "waiting_for_response"},
		{"assistant yielded, japanese question", []sessionEntry{
			{Role: "assistant", Content: "扉を開けますか？（先に確認します）", StopReason: "end_turn"}}, "waiting_for_input"},
		{"assistant yielded, statement", []sessionEntry{
			{Role: "assistant", Content: "実装しました。", StopReason: "end_turn"}}, "waiting_for_input"},
		{"assistant yielded, ascii question", []sessionEntry{
			{Role: "assistant", Content: "shall I proceed?", StopReason: "end_turn"}}, "waiting_for_input"},
		{"assistant still running a tool", []sessionEntry{
			{Role: "assistant", Content: "確認します。", StopReason: "tool_use"}}, "waiting_for_response"},
	}
	for _, c := range cases {
		if got := classifyClaudeSessionState(c.entries); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
