package server

import (
	"path/filepath"
	"strings"
	"testing"

	mywant "mywant/engine/core"
)

func wazaWant(satisfied bool, ids ...string) WazaProgress {
	return WazaProgress{
		Waza:       mywant.Waza{Kind: "want_type", Type: "transit_search"},
		Satisfied:  satisfied,
		MatchedIDs: ids,
	}
}

func wazaThing(subtype string, values ...string) WazaProgress {
	return WazaProgress{
		Waza:       mywant.Waza{Kind: "thing", Subtype: subtype},
		Satisfied:  true,
		MatchedIDs: values,
	}
}

func TestLiveEvidenceSplitsWantsFromThings(t *testing.T) {
	wants, memo := liveEvidence([]WazaProgress{
		wazaThing("station", "国分寺"),
		wazaWant(true, "want-b", "want-a"),
	})

	if len(wants) != 2 || wants[0] != "want-a" || wants[1] != "want-b" {
		t.Errorf("wants = %v, want them sorted", wants)
	}
	// Memo values are prefixed with the catalog key so they match the member ids
	// constellations and canvas tiles are made of.
	if len(memo) != 1 || memo[0] != "stations::国分寺" {
		t.Errorf("memo = %v, want [stations::国分寺]", memo)
	}
}

// An unsatisfied 所作 witnesses nothing, and a `repeat` 所作 is satisfied out of
// the record book — neither may end up on the canvas.
func TestLiveEvidenceIgnoresUnsatisfiedAndRepeat(t *testing.T) {
	wants, memo := liveEvidence([]WazaProgress{
		wazaWant(false, "want-unsatisfied"),
		{
			Waza:       mywant.Waza{Kind: "repeat", Kata: "kata-kasa", MinCount: 3},
			Satisfied:  true,
			MatchedIDs: []string{"kata-kasa:3"},
		},
	})

	if len(wants) != 0 || len(memo) != 0 {
		t.Errorf("evidence = %v / %v, want nothing drawable", wants, memo)
	}
}

func TestMemoCatalogKey(t *testing.T) {
	if got := thingCatalogKey("station"); got != "stations" {
		t.Errorf("station → %q, want stations", got)
	}
	if got := thingCatalogKey("city"); got != "cities" {
		t.Errorf("city → %q, want cities (the catalog key, not a naive plural)", got)
	}
	if got := thingCatalogKey("madeup"); got != "madeups" {
		t.Errorf("unknown type → %q, want the naive plural", got)
	}
}

// A constellation hangs off its things: they chain to each other, and each
// want spokes off the value it names. Two wants are never joined — they share a
// place, not a thread.
func TestKataEdgesSpokesFromMemo(t *testing.T) {
	// An empty store, not no store: kataEdges asks it what each thing is
	// called, because a thing's id is a UUID and no longer says. A real server
	// always has one (server.go), so a bare &Server{} is a shape only a test
	// can produce — and it panicked here rather than testing anything.
	s := &Server{thingStore: &ThingStore{path: filepath.Join(t.TempDir(), "memo.yaml")}}
	edges := s.kataEdges([]string{"cities::Kokubunji", "stations::国分寺"}, []string{"want-a", "want-b"})

	// Nothing is recorded, so no want is named by a value here (nor by a param
	// — there is no builder), and both fall back to the first thing: one
	// thing↔thing link plus one spoke per want.
	if len(edges) != 3 {
		t.Fatalf("edges = %d, want 3", len(edges))
	}
	if edges[0].From != "cities::Kokubunji" || edges[0].To != "stations::国分寺" {
		t.Errorf("first edge = %s→%s, want the memo chain", edges[0].From, edges[0].To)
	}
	for _, e := range edges[1:] {
		if e.From != "cities::Kokubunji" {
			t.Errorf("spoke starts at %s, want the memo hub", e.From)
		}
	}
	for _, e := range edges {
		if strings.HasPrefix(e.From, "want-") && strings.HasPrefix(e.To, "want-") {
			t.Errorf("want↔want edge %s→%s: wants share a place, not a thread", e.From, e.To)
		}
	}
}

// 糧 (a restaurant and a budget) declares no thing 所作, so there is no hub to
// hang from — without a fallback its wants would light separately and read as
// unrelated.
func TestKataEdgesChainsWantsWithoutMemo(t *testing.T) {
	s := &Server{}
	edges := s.kataEdges(nil, []string{"want-a", "want-b"})

	if len(edges) != 1 || edges[0].From != "want-a" || edges[0].To != "want-b" {
		t.Errorf("edges = %+v, want the two wants chained", edges)
	}
}

func TestKataEdgesNeedsTwoMembers(t *testing.T) {
	s := &Server{}
	if edges := s.kataEdges(nil, []string{"want-a"}); edges != nil {
		t.Errorf("edges = %v, want none for a single member", edges)
	}
}

// The fingerprint is what puts a kata catching fire into the want collection's
// ETag; it must not depend on map iteration order.
func TestKataLabelFingerprintStable(t *testing.T) {
	a := map[string]map[string]string{
		"want-a": {"kata/kata-kasa": "国分寺", "kata/kata-ate": "国分寺"},
		"want-b": {"kata/kata-sora": "国分寺"},
	}
	b := map[string]map[string]string{
		"want-b": {"kata/kata-sora": "国分寺"},
		"want-a": {"kata/kata-ate": "国分寺", "kata/kata-kasa": "国分寺"},
	}
	if kataLabelFingerprint(a) != kataLabelFingerprint(b) {
		t.Error("fingerprint depends on iteration order")
	}
	if kataLabelFingerprint(nil) != "" {
		t.Error("no labels should fingerprint to the empty string, leaving the ETag untouched")
	}

	c := map[string]map[string]string{"want-a": {"kata/kata-kasa": "中野"}}
	if kataLabelFingerprint(a) == kataLabelFingerprint(c) {
		t.Error("a different constellation must change the fingerprint")
	}
}

// A value in no constellation stands as its own scope, so a form that points one
// want at one remembered value holds without anyone building a group of one.
// A value already in a constellation does NOT also stand alone — it would be
// measured twice and bank two practices for a single piece of evidence.
func TestCollectKataScopesGivesLoneValuesTheirOwnScope(t *testing.T) {
	dir := t.TempDir()
	store := &ThingStore{path: filepath.Join(dir, "memo.yaml")}
	labels := &ThingLabelStore{path: filepath.Join(dir, "memo-labels.yaml")}

	for _, v := range []string{"国分寺", "新宿"} {
		if err := store.Record("station", v); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	if err := store.Record("city", "Kokubunji"); err != nil {
		t.Fatalf("record: %v", err)
	}
	// 国分寺 and Kokubunji are declared one place; 新宿 is left alone.
	//
	// Labelled by the ids the store actually assigned. A thing's identity is a
	// UUID that says nothing about what it is called, so labelling the
	// "stations::国分寺" it used to be named by would hang the constellation off
	// nothing: collectKataScopes looks its members up in the store and skips
	// the ones it cannot find, and the group would come back empty.
	idOf := map[string]string{}
	for _, e := range store.Entries() {
		idOf[e.Catalog+"::"+e.Value] = e.ID
	}
	for _, m := range []string{"stations::国分寺", "cities::Kokubunji"} {
		id := idOf[m]
		if id == "" {
			t.Fatalf("nothing recorded for %s — the store holds %v", m, idOf)
		}
		if err := labels.Set(id, "constellation/国分寺", "true"); err != nil {
			t.Fatalf("label: %v", err)
		}
	}

	s := &Server{thingStore: store, thingLabels: labels}
	byName := map[string]thingScope{}
	for _, g := range s.collectKataScopes() {
		byName[g.Name] = g
	}

	joined, ok := byName["国分寺"]
	if !ok || joined.Lone {
		t.Fatalf("the constellation should be a scope in its own right, got %+v", joined)
	}
	if !joined.has("station") || !joined.has("city") {
		t.Errorf("the constellation should hold both kinds: %+v", joined.BySubtype)
	}

	lone, ok := byName["新宿"]
	if !ok {
		t.Fatal("an ungrouped value should stand as its own scope")
	}
	if !lone.Lone || !lone.has("station") || lone.has("city") {
		t.Errorf("a lone scope holds exactly its one value: %+v", lone)
	}

	if _, duplicated := byName["Kokubunji"]; duplicated {
		t.Error("a value inside a constellation must not also stand alone")
	}
}

// A wire is two halves that have to agree: the provider exposes a state under a
// key, the consumer imports that key into a parameter. Neither half alone is
// the form — which is the whole reason 刻 needed importFrom.
func wiredWant(id, typ string, imports map[string]string, exposes []mywant.ExposeEntry) *mywant.Want {
	w := &mywant.Want{}
	w.Metadata.ID = id
	w.Metadata.Type = typ
	w.Spec.Imports = imports
	w.Spec.Exposes = exposes
	return w
}

func TestImportsFromNeedsBothHalvesOfTheWire(t *testing.T) {
	s := &Server{}
	req := mywant.WazaImport{Type: "transit_search", State: "departure", Into: "event_time"}

	route := wiredWant("route", "transit_search", nil, []mywant.ExposeEntry{
		{CurrentState: "departure", As: "leave_at"},
	})
	byType := map[string][]*mywant.Want{"transit_search": {route}}

	wired := wiredWant("alarm", "reminder", map[string]string{"leave_at": "event_time"}, nil)
	if !s.importsFrom(wired, req, byType, nil) {
		t.Error("an alarm importing the route's departure is wired; importsFrom said it was not")
	}

	// The alarm that merely exists — the case the form used to accept.
	if s.importsFrom(wiredWant("alarm2", "reminder", nil, nil), req, byType, nil) {
		t.Error("an alarm with no imports is not wired to anything")
	}
	// Right key, wrong inlet.
	elsewhere := wiredWant("alarm3", "reminder", map[string]string{"leave_at": "message"}, nil)
	if s.importsFrom(elsewhere, req, byType, nil) {
		t.Error("the departure landing in `message` is not the form; `into` was ignored")
	}
	// Right inlet, but the provider publishes a different state.
	other := map[string][]*mywant.Want{"transit_search": {wiredWant("route2", "transit_search", nil,
		[]mywant.ExposeEntry{{CurrentState: "arrival", As: "leave_at"}})}}
	if s.importsFrom(wired, req, other, nil) {
		t.Error("importing the arrival is not importing the departure")
	}
}

// When an earlier 所作 has already settled which route counts — the one that
// arrives in THIS group — the wire has to run to that one and not to any other.
func TestImportsFromHonoursTheRouteAlreadyPinned(t *testing.T) {
	s := &Server{}
	req := mywant.WazaImport{Type: "transit_search", State: "departure"}
	here := wiredWant("route-here", "transit_search", nil, []mywant.ExposeEntry{
		{CurrentState: "departure", As: "k1"},
	})
	elsewhere := wiredWant("route-elsewhere", "transit_search", nil, []mywant.ExposeEntry{
		{CurrentState: "departure", As: "k2"},
	})
	byType := map[string][]*mywant.Want{"transit_search": {here, elsewhere}}
	pinned := map[string][]string{"transit_search": {"route-here"}}

	if !s.importsFrom(wiredWant("a", "reminder", map[string]string{"k1": "event_time"}, nil), req, byType, pinned) {
		t.Error("wired to the pinned route, which is the one the form is about")
	}
	if s.importsFrom(wiredWant("b", "reminder", map[string]string{"k2": "event_time"}, nil), req, byType, pinned) {
		t.Error("wired to a route that arrives somewhere else — not this form")
	}
}

// A budget is not fed by a wire but by what is under it, so 銭 asks for
// ownership. Same shape of question as importsFrom, different relation.
func TestOwnsWantsTheChildNotTheNeighbour(t *testing.T) {
	s := &Server{}
	req := mywant.WazaOwns{Type: "transit_search"}

	budget := wiredWant("budget", "budget", nil, nil)
	loose := wiredWant("route", "transit_search", nil, nil)
	byType := map[string][]*mywant.Want{"transit_search": {loose}}
	if s.owns(budget, req, byType, nil) {
		t.Error("a route standing beside a budget is not under it")
	}

	child := wiredWant("route2", "transit_search", nil, nil)
	child.Metadata.OwnerReferences = []mywant.OwnerReference{
		{Kind: "Want", Controller: true, ID: "budget"},
	}
	if !s.owns(budget, req, map[string][]*mywant.Want{"transit_search": {child}}, nil) {
		t.Error("a route owned by the budget is under it")
	}
}

// The two halves of a compound offer. What is missing decides which sentence
// is useful: with nothing of the kind on the board, both moves; with one
// standing there unrelated, only the second — placing a second alarm never
// helped anybody.
func TestHintNamesBothMovesOnlyWhenTheWantIsMissing(t *testing.T) {
	s := &Server{}
	wz := mywant.Waza{Kind: "want_type", Type: "reminder", Status: "any",
		ImportFrom: &mywant.WazaImport{Type: "transit_search", State: "departure"}}

	empty := s.evaluateWaza(wz, map[string][]*mywant.Want{}, nil, nil)
	if !strings.Contains(empty.Hint, "Place a reminder and wire") {
		t.Errorf("nothing on the board should ask for both moves, got %q", empty.Hint)
	}

	unwired := map[string][]*mywant.Want{"reminder": {wiredWant("a", "reminder", nil, nil)}}
	standing := s.evaluateWaza(wz, unwired, nil, nil)
	if !strings.HasPrefix(standing.Hint, "Wire the") {
		t.Errorf("an alarm already standing should ask only for the wire, got %q", standing.Hint)
	}
}

// The other inlet. A reminder reads event_time as a parameter, so the wire it
// needs is {fromGlobalParam} on the parameter, not an import into the state of
// the same name — and the two must not be mistaken for each other.
func paramWant(id, typ string, params map[string]any) *mywant.Want {
	w := &mywant.Want{}
	w.Metadata.ID = id
	w.Metadata.Type = typ
	w.Spec.Params = params
	return w
}

func TestParamsFromFeedsTheParameterNotTheState(t *testing.T) {
	s := &Server{}
	req := mywant.WazaImport{Type: "transit_search", State: "departure", Into: "event_time"}

	route := wiredWant("route", "transit_search", nil, []mywant.ExposeEntry{
		{CurrentState: "departure", AsGlobalParam: "leave_at"},
	})
	byType := map[string][]*mywant.Want{"transit_search": {route}}

	fed := paramWant("alarm", "reminder", map[string]any{
		"event_time": map[string]any{"fromGlobalParam": "leave_at"},
	})
	if !s.paramsFrom(fed, req, byType, nil) {
		t.Error("a reminder whose event_time comes from the route's departure is fed by it")
	}

	// A literal time somebody typed in is not the route's departure.
	typed := paramWant("alarm2", "reminder", map[string]any{"event_time": "2026-01-01T09:00:00Z"})
	if s.paramsFrom(typed, req, byType, nil) {
		t.Error("a hand-typed time is not a wire")
	}
	// The import path is a different wire and must not answer for this one.
	imported := wiredWant("alarm3", "reminder", map[string]string{"leave_at": "event_time"}, nil)
	if s.paramsFrom(imported, req, byType, nil) {
		t.Error("an import into state is not a parameter being fed")
	}
	// Right wire, wrong state at the far end.
	other := map[string][]*mywant.Want{"transit_search": {wiredWant("route2", "transit_search", nil,
		[]mywant.ExposeEntry{{CurrentState: "arrival", AsGlobalParam: "leave_at"}})}}
	if s.paramsFrom(fed, req, other, nil) {
		t.Error("the arrival time is not the departure time")
	}
}
