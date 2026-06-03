package planner_test

import (
	"testing"

	ws "github.com/onelittlenightmusic/want-spec"

	"mywant/engine/planner"
)

// ─── test fixtures ────────────────────────────────────────────────────────────

func makeSmartGolfDefs() map[string]*ws.WantTypeDefinition {
	return map[string]*ws.WantTypeDefinition{
		"smartgolf_check_reserved": {
			Metadata: ws.WantTypeMetadata{Name: "smartgolf_check_reserved"},
			State: []ws.StateDef{
				{Name: "is_reserved", Type: "boolean", Exposable: true,
					Description: "Whether a future SmartGolf reservation already exists."},
				{Name: "next_datetime", Type: "string", Exposable: true,
					Description: "Date and time of the next upcoming reservation."},
				{Name: "next_store", Type: "string", Exposable: true,
					Description: "Store name of the next upcoming reservation."},
			},
		},
		"smartgolf_list_available": {
			Metadata: ws.WantTypeMetadata{Name: "smartgolf_list_available"},
			State: []ws.StateDef{
				{Name: "smartgolf_all_available_times", Type: "array", Exposable: true,
					Description: "All available SmartGolf booking slots across rooms and dates. " +
						"An array of slot objects with room, date, time fields."},
				{Name: "first_room", Type: "string", Exposable: true,
					Description: "Room name of the earliest available SmartGolf slot."},
				{Name: "first_date", Type: "string", Exposable: true,
					Description: "Date of the earliest available slot in YYYY-MM-DD format."},
				{Name: "first_time", Type: "string", Exposable: true,
					Description: "Time of the earliest available slot in HH:MM format."},
			},
		},
		"choice": {
			Metadata: ws.WantTypeMetadata{Name: "choice"},
			State: []ws.StateDef{
				{Name: "choices", Type: "array", Exposable: false,
					Description: "List of available choices."},
				{Name: "selected", Type: "any", Exposable: true,
					Description: "Currently selected value."},
			},
		},
		"smartgolf_book": {
			Metadata: ws.WantTypeMetadata{Name: "smartgolf_book"},
			Parameters: []ws.ParameterDef{
				{Name: "room", Required: true, Type: "string"},
				{Name: "date", Required: true, Type: "string"},
				{Name: "time", Required: true, Type: "string"},
			},
			State: []ws.StateDef{
				{Name: "summary", Type: "string", Exposable: true,
					Description: "Human-readable summary of the SmartGolf booking confirmation screen."},
				{Name: "reservation_datetime", Type: "string", Exposable: true,
					Description: "Reservation date and time as displayed on the confirmation screen."},
			},
		},
	}
}

// makeSmartGolfBookingPlan builds a WantTypePlan for smartgolf_booking.
func makeSmartGolfBookingPlan() *ws.WantTypePlan {
	return &ws.WantTypePlan{
		Achieve: []ws.PlanTarget{
			{Type: "smartgolf_book", Description: "Navigate to booking confirmation"},
		},
		Monitor: []ws.PlanTarget{
			{Type: "smartgolf_check_reserved", Description: "Check for existing reservation"},
		},
		Hints: []ws.PlanHint{
			{For: "smartgolf_book", Use: "smartgolf_list_available",
				Note: "Use list + choice to let the user interactively pick a slot"},
		},
		Constraints: []ws.PlanConstraint{
			{Description: "Skip booking if already reserved",
				When: &ws.ConditionDef{
					Field:    "smartgolf_check_reserved.is_reserved",
					Operator: "==",
					Value:    true,
				}},
		},
	}
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestPlannerExposableIndex(t *testing.T) {
	defs := makeSmartGolfDefs()
	idx := planner.BuildExposableIndexFromDefs(defs)

	all := idx.All()
	if len(all) == 0 {
		t.Fatal("expected non-empty exposable index")
	}

	// smartgolf_list_available should have 4 exposable fields
	listFields := idx.FindByWantType("smartgolf_list_available")
	if len(listFields) != 4 {
		t.Errorf("expected 4 exposable fields for smartgolf_list_available, got %d", len(listFields))
	}

	// choice.choices is NOT exposable; choice.selected IS
	choiceFields := idx.FindByWantType("choice")
	if len(choiceFields) != 1 {
		t.Errorf("expected 1 exposable field for choice (selected), got %d", len(choiceFields))
	}
	if choiceFields[0].Field != "selected" {
		t.Errorf("expected choice exposable field 'selected', got %q", choiceFields[0].Field)
	}
}

func TestPlannerSuffixMatch(t *testing.T) {
	defs := makeSmartGolfDefs()
	idx := planner.BuildExposableIndexFromDefs(defs)

	// "first_room" last token "room" should match param "room"
	results := idx.FindBySuffix("room")
	if len(results) == 0 {
		t.Fatal("expected FindBySuffix('room') to return at least one field")
	}
	found := false
	for _, r := range results {
		if r.WantType == "smartgolf_list_available" && r.Field == "first_room" {
			found = true
		}
	}
	if !found {
		t.Error("expected smartgolf_list_available.first_room to appear in suffix-match for 'room'")
	}

	// "next_datetime" should NOT match "time" (last token is "datetime", not "time")
	timeResults := idx.FindBySuffix("time")
	for _, r := range timeResults {
		if r.Field == "next_datetime" {
			t.Errorf("next_datetime should NOT suffix-match 'time', but it did")
		}
	}
}

func TestPlannerSmartGolfWithHint(t *testing.T) {
	defs := makeSmartGolfDefs()
	idx := planner.BuildExposableIndexFromDefs(defs)
	p := planner.New(idx, defs)

	plan := makeSmartGolfBookingPlan()
	result := p.PlanFromWantType("smartgolf_booking", "smartgolf", plan, nil)

	// Should have at least 4 wants: check_reserved, list_available, choice, book
	if len(result.Recipe.Wants) < 4 {
		t.Errorf("expected at least 4 wants in recipe, got %d: %v",
			len(result.Recipe.Wants), wantNames(result.Recipe.Wants))
	}

	// check_reserved should appear (monitor)
	assertWantType(t, result.Recipe.Wants, "smartgolf_check_reserved", "monitor want")

	// list_available should appear (hint provider)
	assertWantType(t, result.Recipe.Wants, "smartgolf_list_available", "hint provider want")

	// choice should appear (intermediary)
	assertWantType(t, result.Recipe.Wants, "choice", "choice intermediary want")

	// smartgolf_book should appear last (terminal)
	last := result.Recipe.Wants[len(result.Recipe.Wants)-1]
	if last.Metadata.Type != "smartgolf_book" {
		t.Errorf("expected smartgolf_book as last want (terminal), got %q", last.Metadata.Type)
	}

	// Steps should be non-empty
	if len(result.Steps) == 0 {
		t.Error("expected non-empty steps in PlannerResult")
	}

	// Confidence should be "inferred" (hint-guided)
	if result.Confidence == "unknown" {
		t.Errorf("expected confidence 'inferred' or 'certain', got %q", result.Confidence)
	}

	t.Logf("Plan confidence: %s", result.Confidence)
	t.Logf("Wants: %v", wantNames(result.Recipe.Wants))
	for _, step := range result.Steps {
		t.Logf("  [%s] %s (%s): %s", step.Role, step.WantType, step.Confidence, step.Reasoning)
	}
}

func TestPlannerDirectParamMatch(t *testing.T) {
	defs := makeSmartGolfDefs()
	idx := planner.BuildExposableIndexFromDefs(defs)
	p := planner.New(idx, defs)

	// No hints → Planner falls back to suffix-matching first_room/first_date/first_time
	plan := &ws.WantTypePlan{
		Achieve: []ws.PlanTarget{
			{Type: "smartgolf_book"},
		},
	}

	result := p.PlanFromWantType("smartgolf-direct", "smartgolf", plan, nil)

	if len(result.Recipe.Wants) == 0 {
		t.Fatal("expected non-empty recipe wants")
	}

	t.Logf("Direct match confidence: %s", result.Confidence)
	t.Logf("Wants: %v", wantNames(result.Recipe.Wants))
	for _, step := range result.Steps {
		t.Logf("  [%s] %s (%s): %s", step.Role, step.WantType, step.Confidence, step.Reasoning)
	}
}

func TestPlannerMonitorOnly(t *testing.T) {
	defs := makeSmartGolfDefs()
	idx := planner.BuildExposableIndexFromDefs(defs)
	p := planner.New(idx, defs)

	plan := &ws.WantTypePlan{
		Monitor: []ws.PlanTarget{
			{Type: "smartgolf_check_reserved"},
		},
	}

	result := p.PlanFromWantType("check-only", "", plan, nil)

	if len(result.Recipe.Wants) != 1 {
		t.Errorf("expected 1 want for monitor-only plan, got %d", len(result.Recipe.Wants))
	}
	if result.Recipe.Wants[0].Metadata.Type != "smartgolf_check_reserved" {
		t.Errorf("expected smartgolf_check_reserved want, got %q", result.Recipe.Wants[0].Metadata.Type)
	}
	if result.Confidence != "certain" {
		t.Errorf("expected confidence 'certain' for monitor-only, got %q", result.Confidence)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func wantNames(wants []ws.RecipeWant) []string {
	names := make([]string, len(wants))
	for i, w := range wants {
		names[i] = w.Metadata.Type + "(" + w.Metadata.Name + ")"
	}
	return names
}

func assertWantType(t *testing.T, wants []ws.RecipeWant, wantType, msg string) {
	t.Helper()
	for _, w := range wants {
		if w.Metadata.Type == wantType {
			return
		}
	}
	t.Errorf("expected want type %q in recipe (%s): %v", wantType, msg, wantNames(wants))
}
