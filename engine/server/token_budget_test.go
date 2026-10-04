package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// A want shaped as GET /wants/{id} hands it out: a small present and a big past.
func budgetTestWant(id string, historyRows int) map[string]any {
	hist := []any{}
	for i := 0; i < historyRows; i++ {
		hist = append(hist, map[string]any{"timestamp": "2026-10-04T10:00:00Z",
			"stateValue": map[string]any{"note": strings.Repeat("past value ", 20)}})
	}
	return map[string]any{
		"metadata": map[string]any{"id": id, "name": "w-" + id, "type": "reservation",
			"labels": map[string]any{"category-bg-dark": "linear-gradient(#000,#111)", "aliases": "ゴルフ"}},
		"spec":             map[string]any{"params": map[string]any{"store": "x"}},
		"status":           "achieved",
		"hash":             "h-" + id,
		"state":            map[string]any{"current": map[string]any{"next_store": "北新宿店", "achieving_percentage": 100}},
		"state_timestamps": map[string]any{"next_store": "2026-10-04T10:00:00Z"},
		"history":          map[string]any{"stateHistory": hist},
	}
}

func roundTrip(v any) any {
	raw, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(raw, &out)
	return out
}

// The history goes first, and with it gone the present is all still there.
func TestBudgetFitDropsHistoryFirst(t *testing.T) {
	v := roundTrip(budgetTestWant("a", 60))
	cut, res := budgetFit(v, 300)
	if !res.Fits {
		t.Fatalf("did not fit: %+v", res)
	}
	if len(res.Stages) == 0 || res.Stages[0] != "history" {
		t.Fatalf("stages %v, want history first", res.Stages)
	}
	m := cut.(map[string]any)
	if _, ok := m["history"]; ok {
		t.Fatal("history kept")
	}
	cur := m["state"].(map[string]any)["current"].(map[string]any)
	if cur["next_store"] != "北新宿店" {
		t.Fatalf("the present was lost: %v", cur)
	}
}

// Within budget nothing is touched.
func TestBudgetFitLeavesWhatFits(t *testing.T) {
	v := roundTrip(budgetTestWant("b", 1))
	_, res := budgetFit(v, 100000)
	if !res.Fits || len(res.Stages) != 0 {
		t.Fatalf("touched what fit: %+v", res)
	}
}

// A list of many is cut in its objects first, then to its first items.
func TestBudgetFitShortensAList(t *testing.T) {
	var list []any
	for i := 0; i < 30; i++ {
		list = append(list, budgetTestWant(string(rune('a'+i)), 5))
	}
	v := roundTrip(map[string]any{"wants": list})
	cut, res := budgetFit(v, 400)
	if !res.Fits {
		t.Fatalf("did not fit: %+v", res)
	}
	if res.Stages[len(res.Stages)-1] != "items" {
		t.Fatalf("stages %v, want items last", res.Stages)
	}
	if n := len(cut.(map[string]any)["wants"].([]any)); n < 1 || n >= 30 {
		t.Fatalf("kept %d wants", n)
	}
}

// The ledger decides, without measuring, once it has seen the object.
func TestBudgetLedgerRemembers(t *testing.T) {
	o := budgetObjects(roundTrip(budgetTestWant("c", 10)))[0]
	first := budgetLedger.lookup(o)
	budgetLedger.mu.Lock()
	e := budgetLedger.entries[o.key]
	budgetLedger.mu.Unlock()
	if e.Fingerprint != "h-c" || len(first) != len(budgetStages)+1 || first[1] >= first[0] {
		t.Fatalf("entry %+v", e)
	}
	again := budgetLedger.lookup(o)
	if again[0] != first[0] {
		t.Fatal("measured again for the same version")
	}
}

func TestParseTokenLimit(t *testing.T) {
	for q, want := range map[string]int{"token-limit=4k": 4096, "token_limit=4096": 4096, "token-limit=8K": 8192, "token-limit=1.5k": 1536} {
		r := httptest.NewRequest("GET", "/api/v1/wants?"+q, nil)
		if got, ok := parseTokenLimit(r); !ok || got != want {
			t.Errorf("%s: %d %v", q, got, ok)
		}
	}
	if _, ok := parseTokenLimit(httptest.NewRequest("GET", "/api/v1/wants", nil)); ok {
		t.Error("no limit asked, one found")
	}
}
