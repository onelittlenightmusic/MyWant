package mywant

import (
	"context"
	"testing"
)

func TestFieldChange(t *testing.T) {
	c := fieldChange("weather_condition", "sunny", "rain", false)
	if c == nil || c.Kind != ChangeChanged || c.From != "sunny" || c.To != "rain" {
		t.Fatalf("a value turned: %+v", c)
	}
	if fieldChange("weather_condition", "rain", "rain", false) != nil {
		t.Error("nothing changed, told as a change")
	}
	before := []any{map[string]any{"id": "a", "text": "財布"}}
	now := []any{map[string]any{"id": "a", "text": "財布"}, map[string]any{"id": "b", "text": "傘"}}
	c = fieldChange("items", before, now, true)
	if c == nil || c.Kind != ChangeAdded || len(c.Items) != 1 || c.Items[0].Key != "b" {
		t.Fatalf("an item added: %+v", c)
	}
	c = fieldChange("items", now, before, true)
	if c == nil || c.Kind != ChangeRemoved || len(c.Items) != 1 || c.Items[0].Key != "b" {
		t.Fatalf("an item removed: %+v", c)
	}
}

func TestItemKey(t *testing.T) {
	r := map[string]any{"room": "中野新橋店/打席予約(Room02)", "datetime_rfc3339": "2026-11-21T08:00:00+09:00"}
	if got := ItemKey(r); got != "中野新橋店/打席予約(Room02)@2026-11-21T08:00:00+09:00" {
		t.Errorf("a reservation is where and when: %q", got)
	}
	if got := ItemKey("傘"); got != "傘" {
		t.Errorf("a plain item is itself: %q", got)
	}
}

func TestAdviceOnKeepsKnownKinds(t *testing.T) {
	saved := advisors
	defer func() { advisors = saved }()
	advisors = map[string]Advisor{"t": func(AdviceInput) []Advice {
		return []Advice{{Kind: AdviceBring, Item: "傘"}, {Kind: AdviceBring, Item: "傘"}, {Kind: "dance", Item: "x"}, {Kind: AdviceWear, Item: " "}}
	}}
	w := &Want{Metadata: Metadata{ID: "w", Name: "w", Type: "t"}}
	got := AdviceOn(context.Background(), w, ResultHistoryEntry{}, "")
	if len(got) != 1 || got[0].Item != "傘" {
		t.Errorf("only known kinds, each once: %+v", got)
	}
}

func TestHearOnDropsWhatItCannotDo(t *testing.T) {
	saved := hearers
	defer func() { hearers = saved }()
	hearers = map[string]Hearer{"t": func(HearerInput) HearerOutput {
		return HearerOutput{Proposals: []Proposal{
			{Action: ProposeAppend, Field: "items", Value: "傘", Ask: "傘を足しますか？"},
			{Action: "delete_everything", Ask: "全部消しますか？"},
			{Action: ProposeAppend, Field: "items", Value: "鍵"}, // nothing to ask
		}}
	}}
	w := &Want{Metadata: Metadata{ID: "w", Name: "w", Type: "t"}}
	out := HearOn(context.Background(), w, HeardMurmur{}, "")
	if len(out.Proposals) != 1 || out.Proposals[0].Value != "傘" {
		t.Errorf("only what the engine can carry out, asked: %+v", out.Proposals)
	}
}
