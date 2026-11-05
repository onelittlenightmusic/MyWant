package server

import (
	"os"
	"testing"
	"time"

	ws "github.com/onelittlenightmusic/want-spec"
)

func TestStoryPattern(t *testing.T) {
	for _, c := range []struct {
		question string
		params   map[string]string
		target   string
		want     string
	}{
		{"中野坂上から銀座の乗り換え", map[string]string{"from": "中野坂上", "to": "銀座"}, "", "{from}から{to}の乗り換え"},
		// a time said otherwise than its value: its numbers are {n}
		{"明日の朝9時40分までに戸塚に移動できる乗り換え", map[string]string{"to": "戸塚", "time": "09:40"}, "", "明日の朝{n}時{n}分までに{to}に移動できる乗り換え"},
		// the longest value first: 中野坂上 is not 中野 + 坂上
		{"中野坂上から中野の乗り換え？", map[string]string{"from": "中野坂上", "to": "中野"}, "", "{from}から{to}の乗り換え"},
		{"スマートゴルフ のスコアは？", nil, "スマートゴルフ", "{target}のスコアは"},
	} {
		got := storyPattern(c.question, &ws.PlanSpec{Params: c.params, Target: c.target})
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.question, got, c.want)
		}
	}
}

// A want type's worked example is a kind of question other questions are.
func TestStoryQADeclares(t *testing.T) {
	qa := newStoryQA("transit_search", "中野坂上から銀座の乗り換え", "deploy_want transit_search with params from=中野坂上, to=銀座")
	if qa.Template != "{from}から{to}の乗り換え" {
		t.Fatalf("template %q", qa.Template)
	}
	qas := []storyQA{qa}
	if storyDeclared(qas, "新宿から横浜の乗り換え", "") == nil {
		t.Error("新宿から横浜の乗り換え is not one of it")
	}
	if storyDeclared(qas, "中野の天気は？", "") != nil {
		t.Error("a weather question taken for a route")
	}
}

// storyJudged is a turn asked, planned, answered with want and judged: gone to
// (success) or not (ignored).
func storyJudged(r *storyRecorder, now *time.Time, turn, question string, plan ws.PlanSpec, card robotCard, went bool) {
	r.ask("chr-1", turn, "", question)
	*now = now.Add(5 * time.Second)
	r.actDone(ws.Act{Metadata: ws.ResourceMeta{Name: "act-" + turn}, Spec: ws.ActSpec{Turn: turn},
		Status: ws.ActStatus{Phase: "Succeeded", Cards: []*robotCard{&card}}}, plan)
	*now = now.Add(2 * time.Second)
	r.answer(turn, []robotCard{card}, false)
	*now = now.Add(10 * time.Second)
	if went {
		r.arrive("chr-1", storyWantAt(card.ID, card.Type))
	}
	*now = now.Add(storyJudgeWithin)
	r.noticeStops()
}

func TestStoryLedgerPatterns(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	r := storyTestRecorder(t, &now)
	route := robotCard{Kind: "want", ID: "want-r", Name: "中野坂上→銀座", Type: "transit_search"}
	storyJudged(r, &now, "t1", "中野坂上から銀座の乗り換え",
		ws.PlanSpec{Action: "new", Type: "transit_search", Params: map[string]string{"from": "中野坂上", "to": "銀座"}}, route, true)
	storyJudged(r, &now, "t2", "新宿から横浜の乗り換え",
		ws.PlanSpec{Action: "existing", Target: "中野坂上→銀座"}, route, false)
	storyJudged(r, &now, "t3", "新宿から横浜の乗り換え",
		ws.PlanSpec{Action: "new", Type: "transit_search", Params: map[string]string{"from": "新宿", "to": "横浜"}}, route, true)
	golf := robotCard{Kind: "want", ID: "want-g", Name: "スマートゴルフ", Type: "smartgolf"}
	storyJudged(r, &now, "t4", "スマートゴルフのスコアは？",
		ws.PlanSpec{Action: "existing", Target: "スマートゴルフ", Capability: "rounds"}, golf, true)

	qas := []storyQA{newStoryQA("transit_search", "中野坂上から銀座の乗り換え", "deploy_want transit_search with params from=中野坂上, to=銀座")}

	// A route asked now finds what the routes before it taught.
	got := r.patterns(storyPatternQuery{question: "渋谷から品川の乗り換え", qas: qas})
	if len(got) != 1 || got[0].Declared == nil || got[0].Declared.Type != "transit_search" || got[0].Count != 3 || got[0].Match != "pattern" {
		t.Fatalf("patterns %+v", got)
	}
	best, worst := got[0].Answers[0], got[0].Answers[len(got[0].Answers)-1]
	if best.Shape != "new transit_search(from,to)" || best.Count != 2 || best.SuccessRate != 1 ||
		best.Example == nil || best.Example.Params["to"] != "横浜" {
		t.Errorf("best %+v (example %+v)", best, best.Example)
	}
	if worst.Shape != "existing transit_search" || worst.Outcomes[ws.TurnOutcomeIgnored] != 1 || worst.SuccessRate != 0 {
		t.Errorf("worst %+v", worst)
	}

	// Every kind: the golf one is no type's worked example yet.
	all := r.patterns(storyPatternQuery{qas: qas})
	var golfKind *storyPatternStat
	for i := range all {
		if all[i].Pattern == "{target}のスコアは" {
			golfKind = &all[i]
		}
	}
	if len(all) != 2 || golfKind == nil || golfKind.Declared != nil || golfKind.Answers[0].Shape != "existing smartgolf#rounds" {
		t.Fatalf("all %+v", all)
	}

	// Kept: read again by another recorder, and built again from the stories
	// when the ledger is gone.
	if err := r.flush(); err != nil {
		t.Fatal(err)
	}
	again := storyTestRecorder(t, &now)
	again.dir = r.dir
	if n := again.patterns(storyPatternQuery{qas: qas}); len(n) != 2 || n[0].Count+n[1].Count != 4 {
		t.Fatalf("read again %+v", n)
	}
	if err := os.Remove(r.ledgerPath()); err != nil {
		t.Fatal(err)
	}
	rebuilt := storyTestRecorder(t, &now)
	rebuilt.dir = r.dir
	n := rebuilt.patterns(storyPatternQuery{question: "渋谷から品川の乗り換え", qas: qas})
	if len(n) != 1 || n[0].Count != 3 || n[0].Answers[0].Example == nil || n[0].Answers[0].Example.Params["from"] != "新宿" {
		t.Fatalf("rebuilt %+v", n)
	}
	if err := rebuilt.flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.ledgerPath()); err != nil {
		t.Fatalf("the rebuilt ledger was not written: %v", err)
	}
}
