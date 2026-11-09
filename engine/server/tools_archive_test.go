package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

)

// robotArchiveTestWant puts a want on the test server and waits for it.
func robotArchiveTestWant(t *testing.T, s *Server, name string, labels map[string]string, params map[string]any) string {
	t.Helper()
	w := patchTestDo(t, s, "POST", "/api/v1/wants", []map[string]any{{
		"metadata": map[string]any{"name": name, "type": "queue", "labels": labels},
		"spec":     map[string]any{"params": params},
	}})
	if w.Code != http.StatusCreated {
		t.Fatalf("create %s: %d %s", name, w.Code, w.Body.String())
	}
	for i := 0; i < 150; i++ {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			if want.Metadata.Name == name {
				return want.Metadata.ID
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", name)
	return ""
}

func robotParams(t *testing.T, v map[string]any) string {
	t.Helper()
	b, _ := json.Marshal(v)
	return string(b)
}

// The robot's 「荻窪から中野坂上の乗り換え」 on Fly, 2026-10-06: it named its new
// want after the type, an archived want already had that name, and the deploy
// failed with a 409 the person never saw the reason for.
func TestFMDeployTakesAnotherNameWhenAnArchivedWantHasIt(t *testing.T) {
	s := patchTestServer(t)
	robotArchiveTestWant(t, s, "transit_search",
		map[string]string{"mywant.io/archived": "true"}, map[string]any{"service_time": 0.1})

	out, err := s.robotDeployWant(map[string]string{
		"type": "queue", "name": "transit_search",
		"params": robotParams(t, map[string]any{"service_time": 0.7}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `Deployed "transit_search-2"`) {
		t.Errorf("out = %s", out)
	}
}

func TestFMArchivedWantIsOfferedBack(t *testing.T) {
	s := patchTestServer(t)
	id := robotArchiveTestWant(t, s, "old-route",
		map[string]string{"mywant.io/archived": "true", "mywant.io/canvas-x": "3", "mywant.io/canvas-y": "4"},
		map[string]any{"service_time": 0.2})

	// The same thing asked again: not made twice, offered back — with its card.
	deployArgs := map[string]string{
		"type": "queue", "params": robotParams(t, map[string]any{"service_time": 0.2}),
	}
	out, err := s.robotDeployWant(deployArgs)
	if card := robotTakeCard(deployArgs); card == nil || card.ID != id {
		t.Errorf("deploy card = %+v, want the archived want's (%s)", card, id)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "archived") || !strings.Contains(out, `restore_want with name "old-route"`) {
		t.Errorf("deploy out = %s", out)
	}
	for _, w := range s.globalBuilder.GetAllWantStates() {
		if w.Metadata.Name != "old-route" && w.Metadata.Type == "queue" {
			t.Errorf("a second want was made: %s", w.Metadata.Name)
		}
	}

	// Off the board: point has nowhere to walk, and says why — the card still shown.
	pointArgs := map[string]string{"name": "old-route"}
	out, err = s.robotPoint(pointArgs)
	if card := robotTakeCard(pointArgs); card == nil || card.ID != id {
		t.Errorf("point card = %+v, want the archived want's (%s)", card, id)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "archived") {
		t.Errorf("point out = %s", out)
	}

	// Yes: the archive label comes off.
	args := map[string]string{"name": "old-route"}
	out, err = s.robotRestoreWant(args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `Restored "old-route"`) {
		t.Errorf("restore out = %s", out)
	}
	want, _, _ := s.globalBuilder.FindWantByID(id)
	if want.GetLabels()["mywant.io/archived"] == "true" {
		t.Errorf("still archived: %v", want.GetLabels())
	}
	if _, err := s.robotRestoreWant(map[string]string{"name": "old-route"}); err == nil {
		t.Errorf("restoring a want that is not archived should say so")
	}
}

// A type that is a question (ask-again: run) found in the archive is not
// offered back: it is brought back with what was asked now and run again —
// 「中野坂上から国分寺への乗り換え」 asked a second time searches again.
func TestFMArchivedQuestionIsRunAgain(t *testing.T) {
	s := patchTestServer(t)
	def := s.wantTypeLoader.GetDefinition("queue")
	if def.Metadata.Labels == nil {
		def.Metadata.Labels = map[string]string{}
	}
	def.Metadata.Labels[robotAskAgainLabel] = "run"
	t.Cleanup(func() { delete(def.Metadata.Labels, robotAskAgainLabel) })

	id := robotArchiveTestWant(t, s, "old-route",
		map[string]string{"mywant.io/archived": "true"},
		map[string]any{"service_time": 0.2, "drop": "x"})

	for _, round := range []string{"params changed", "same params"} {
		args := map[string]string{
			"type": "queue", "params": robotParams(t, map[string]any{"service_time": 0.2}),
		}
		out, err := s.robotDeployWant(args)
		if err != nil {
			t.Fatalf("%s: %v", round, err)
		}
		if !strings.Contains(out, `Brought "old-route"`) || strings.Contains(out, "restore_want") {
			t.Errorf("%s: out = %s", round, out)
		}
		if card := robotTakeCard(args); card == nil || card.ID != id {
			t.Errorf("%s: card = %+v, want the brought-back want's (%s)", round, card, id)
		}
		want, _, _ := s.globalBuilder.FindWantByID(id)
		if want.GetLabels()["mywant.io/archived"] == "true" {
			t.Errorf("%s: still archived", round)
		}
		// What was not asked now is not asked: the old leftover goes.
		if _, ok := want.Spec.Params["drop"]; ok {
			t.Errorf("%s: params = %v, want drop gone", round, want.Spec.Params)
		}
		for _, w := range s.globalBuilder.GetAllWantStates() {
			if w.Metadata.Name != "old-route" && w.Metadata.Type == "queue" {
				t.Errorf("%s: a second want was made: %s", round, w.Metadata.Name)
			}
		}
		// Put away again for the next round.
		if w := patchTestDo(t, s, "PATCH", "/api/v1/wants/"+id, map[string]any{
			"metadata": map[string]any{"labels": map[string]any{"mywant.io/archived": "true"}},
		}); w.Code != http.StatusOK {
			t.Fatalf("re-archive: %d %s", w.Code, w.Body.String())
		}
	}
}

// The Mac's robot, asked 「新宿から横浜の乗り換え」, sent name "新宿→横浜" and
// params {}: a route with neither end was made, and it said it had searched.
// A required parameter left out now makes nothing, and the model is told which.
func TestFMDeployRefusesMissingRequiredParams(t *testing.T) {
	s := patchTestServer(t)
	_, err := s.robotDeployWant(map[string]string{"type": "dynamic_background", "name": "bg", "params": "{}"})
	if err == nil || !strings.Contains(err.Error(), "character_id") {
		t.Fatalf("err = %v, want it to name character_id", err)
	}
	for _, w := range s.globalBuilder.GetAllWantStates() {
		if w.Metadata.Name == "bg" {
			t.Errorf("a want was made without its required parameter")
		}
	}
}

func TestFMParseParams(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want string // fmt of the map, or "error"
	}{
		{"from=新宿, to=横浜", "map[from:新宿 to:横浜]"},
		{"from: 新宿、to: 横浜", "map[from:新宿 to:横浜]"},
		{`{"from":"新宿","to":"横浜"}`, "map[from:新宿 to:横浜]"},
		{"to=銀座, time=09:30", "map[time:09:30 to:銀座]"},
		{"service_time=0.5", "map[service_time:0.5]"},
		{"", "map[]"},
		{"{}", "map[]"},
		// What the Mac's model sent: the JSON cut off at its first quote.
		{`{"from":`, "error"},
		{"新宿→横浜", "error"},
	} {
		got, err := robotParseParams(c.raw)
		if c.want == "error" {
			if err == nil {
				t.Errorf("%q: want an error, got %v", c.raw, got)
			}
			continue
		}
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%q: got %v, %v; want %s", c.raw, got, err, c.want)
		}
	}
}

// 「荻窪から国分寺の乗り換え」 on Fly, 2026-11-09: the route put away the other
// way (国分寺→荻窪) was offered as existing; the phone's model chose existing
// with no name, and nothing was made. A question type's put-away wants are not
// candidates — made new, deploy_want brings back the one with those parameters.
func TestPlanCandidatesLeaveOutArchivedQuestions(t *testing.T) {
	s := patchTestServer(t)
	robotArchiveTestWant(t, s, "old-route",
		map[string]string{"mywant.io/archived": "true"},
		map[string]any{"drop": "国分寺", "drop2": "荻窪"})
	named := func() bool {
		cands, err := s.candidatesFor("荻窪から国分寺の乗り換え")
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cands {
			if c.name == "old-route" {
				return true
			}
		}
		return false
	}
	if !named() {
		t.Fatalf("a put-away want plainly asked about should be a candidate")
	}

	def := s.wantTypeLoader.GetDefinition("queue")
	if def.Metadata.Labels == nil {
		def.Metadata.Labels = map[string]string{}
	}
	def.Metadata.Labels[robotAskAgainLabel] = "run"
	t.Cleanup(func() { delete(def.Metadata.Labels, robotAskAgainLabel) })
	if named() {
		t.Errorf("a put-away question (ask-again: run) should not be a candidate")
	}
}

// The robot's conversation is one record (robot_chat.go): a question asked
// from the phone, its answer, and news the robot brings unasked fall in the
// order they were said — not paired by index, which put every answer after the
// news beside the wrong question — each with its turn and cards; a クリア
// (cc_session_resets) starts it afresh.
func TestRobotChatIsOneConversationInOrder(t *testing.T) {
	s := patchTestServer(t)
	if _, err := s.robotWantNow(); err != nil {
		robotArchiveTestWant(t, s, robotWantName, nil, map[string]any{"service_time": 0.1})
	}
	about := robotArchiveTestWant(t, s, "Nakanoの天気", nil, map[string]any{"service_time": 0.1})
	start, err := s.robotChat(0)
	if err != nil {
		t.Fatal(err)
	}
	var after int64
	if n := len(start.Lines); n > 0 {
		after = start.Lines[n-1].Seq
	}

	s.robotAsked("中野の天気は？", "turn-1")
	_ = s.robotSayNotice("「Nakanoの天気」: 晴れ", about, "Nakanoの天気")
	s.sayTurnAnswer("中野は晴れです", nil, nil, "turn-1")

	chat, err := s.robotChat(after)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range chat.Lines {
		got = append(got, l.Who+":"+l.Text)
	}
	want := []string{"person:中野の天気は？", "robot:「Nakanoの天気」: 晴れ", "robot:中野は晴れです"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %v, want %v", got, want)
	}
	if chat.Lines[0].Turn != "turn-1" || chat.Lines[2].Turn != "turn-1" || chat.Lines[1].Turn != "" {
		t.Errorf("turns = %+v", chat.Lines)
	}
	if len(chat.Lines[1].Cards) != 1 || chat.Lines[1].Cards[0].ID != about {
		t.Errorf("the news's card = %+v", chat.Lines[1].Cards)
	}

	// Cleared: what was said before is not the conversation shown.
	robot, _ := s.robotWantNow()
	robot.SetCurrent("cc_session_resets", []any{time.Now().Add(time.Second).Format(time.RFC3339)})
	if chat, _ := s.robotChat(0); len(chat.Lines) != 0 {
		t.Errorf("after クリア: %+v", chat.Lines)
	}
}
