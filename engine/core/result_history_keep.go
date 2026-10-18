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
