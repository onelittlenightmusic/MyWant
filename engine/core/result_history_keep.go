package mywant

import (
	"strings"
)

// Which answers a want keeps: all of them, unless its type says when.
//
// recordResultHistory is the one way an answer goes into a want's history,
// and it keeps every answer that is not empty. Some types know better what
// is an answer: smartgolf_check_reserved answers every check, and a check that
// found no reservation still carries the last one it saw in its result
// field — kept, it is a page saying 予約なし over a reservation, and a ball
// for it beside the tile. Such a type names the field that must be true for
// an answer to be kept (label history-if: is_reserved); an answer for which
// it is false is not kept, and those kept before the type said so are let go.

const historyIfLabel = "history-if"

// An answer can also be kept only when one field of it changes — label
// history-on-change: near_place. A want whose result moves all the time (a
// position, every second) is then news only when what the type cares about
// moves: the place it is at, not the metres. Whether it changed is read on
// every recording, kept or not, so 会社 → (nowhere) → 会社 is two arrivals.
// With history-if on the same field, only the changes to a value are kept:
// arriving, not leaving.
const historyOnChangeLabel = "history-on-change"

// historyOnChange is the field whose change keeps an answer, or "".
func (n *Want) historyOnChange() string {
	if n.WantTypeDefinition == nil {
		return ""
	}
	return strings.TrimSpace(n.WantTypeDefinition.Metadata.Labels[historyOnChangeLabel])
}

// answerChanged says whether the type's history-on-change field moved since
// the last recording (true when the type names none). The first time — a
// start — it is compared with the last answer kept, so a restart standing
// where the last arrival was is not another arrival.
func (n *Want) answerChanged(fields map[string]any) bool {
	field := n.historyOnChange()
	if field == "" {
		return true
	}
	now, ok := fields[field]
	if !ok {
		if v, inState := resolveNestedStateField(n, field); inState && !isEmptyAnswer(v) {
			now = v
		}
	}
	if !n.historyChangeKnown {
		n.historyChangeKnown = true
		if last := n.getHistoryManager().lastResultEntry(); last != nil {
			n.historyChangeSeen = last.Fields[field]
		}
	}
	was := n.historyChangeSeen
	n.historyChangeSeen = now
	return !sameAnswerResult(was, now)
}

// historyIf is the field an answer must have true to be kept, or "".
func (n *Want) historyIf() string {
	if n.WantTypeDefinition == nil {
		return ""
	}
	return strings.TrimSpace(n.WantTypeDefinition.Metadata.Labels[historyIfLabel])
}

// keepsAnswer says whether an answer with these fields is kept: its type's
// history-if field true among them — or, not among them, in the state.
func (n *Want) keepsAnswer(fields map[string]any) bool {
	field := n.historyIf()
	if field == "" {
		return true
	}
	if v, ok := fields[field]; ok {
		return truthyAnswerField(v)
	}
	v, ok := resolveNestedStateField(n, field)
	return ok && truthyAnswerField(v)
}

// pruneResultHistory lets go of the answers kept that the type would not keep
// now. One that does not carry the field at all — read from a history-from
// list, which holds only what happened — stays.
func (n *Want) pruneResultHistory() {
	field := n.historyIf()
	if field == "" {
		return
	}
	h := n.getHistoryManager()
	h.mu.Lock()
	defer h.mu.Unlock()
	entries := h.ResultHistoryRing.Snapshot(0)
	kept := entries[:0:0]
	for _, e := range entries {
		if v, ok := e.Fields[field]; ok && !truthyAnswerField(v) {
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == len(entries) {
		return
	}
	h.ResultHistoryRing.Clear()
	for _, e := range kept {
		h.ResultHistoryRing.Append(e)
	}
	n.resultHistoryChanged()
}

func truthyAnswerField(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s != "" && s != "false" && s != "0" && s != "no"
	default:
		return !isEmptyAnswer(v)
	}
}
