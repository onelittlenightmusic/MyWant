package mywant

import (
	"sync"
	"time"
)

// A chat's lines (cc_messages, what was said to a want; cc_responses, what it
// said) each carry seq: where the line falls in the one conversation the two
// lists make. They used to be read in pairs by index — said, answered, said,
// answered — and a line the want says unasked (the robot's offer, its news)
// put every answer after it beside the wrong question. Ordered by seq, every
// client (the GUI's chat, the phone's) shows the same conversation.
//
// Stamped once, when a line is first written, from its own timestamp — a
// history filled in afterwards (an agent's session read back) falls where it
// was said — and, within one second, in the order lines were written.

var chatSeqMu sync.Mutex
var chatSeqLast int64

// chatSeqPerSecond: room for that many lines a second, written in order.
const chatSeqPerSecond = 1_000_000

func nextChatSeq(at time.Time) int64 {
	chatSeqMu.Lock()
	defer chatSeqMu.Unlock()
	seq := at.Unix() * chatSeqPerSecond
	if seq <= chatSeqLast && chatSeqLast < seq+chatSeqPerSecond-1 {
		seq = chatSeqLast + 1 // the same second: after the last one written
	}
	if seq > chatSeqLast {
		chatSeqLast = seq
	}
	return seq
}

// stampChatSeq gives each line of a chat list that has none its seq.
func stampChatSeq(value any) {
	stamp := func(m map[string]any) {
		if m == nil {
			return
		}
		if _, ok := m["seq"]; ok {
			return
		}
		at := time.Now()
		if ts, ok := m["timestamp"].(string); ok && ts != "" {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				at = t
			}
		}
		m["seq"] = nextChatSeq(at)
	}
	switch list := value.(type) {
	case []any:
		for _, e := range list {
			if m, ok := e.(map[string]any); ok {
				stamp(m)
			}
		}
	case []map[string]any:
		for _, m := range list {
			stamp(m)
		}
	}
}
