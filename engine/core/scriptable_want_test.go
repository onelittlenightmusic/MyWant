package mywant

import (
	"testing"
)

func TestInlineAgentYAMLLoading(t *testing.T) {
	loader := NewWantTypeLoader("../../yaml/want_types")
	if err := loader.LoadAllWantTypes(); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	def := loader.GetDefinition("echo_monitor")
	if def == nil {
		t.Fatal("echo_monitor type not found in yaml/want_types")
	}

	t.Logf("Loaded: %s", def.Metadata.Name)

	if len(def.InlineAgents) != 2 {
		t.Errorf("expected 2 inline agents, got %d", len(def.InlineAgents))
	}

	for _, ia := range def.InlineAgents {
		t.Logf("  - %s (type=%s, runtime=%s, interval=%ds)", ia.Name, ia.Type, ia.Runtime, ia.Interval)
		if ia.Script == "" {
			t.Errorf("agent %s has empty script", ia.Name)
		}
	}

	if def.AchievedWhen == nil {
		t.Error("expected achievedWhen to be defined")
	} else {
		t.Logf("AchievedWhen: %s %s %v", def.AchievedWhen.Field, def.AchievedWhen.Operator, def.AchievedWhen.Value)
	}
}

func TestScriptableWantFactory(t *testing.T) {
	loader := NewWantTypeLoader("../../yaml/want_types")
	if err := loader.LoadAllWantTypes(); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	def := loader.GetDefinition("echo_monitor")
	if def == nil {
		t.Fatal("echo_monitor type not found")
	}

	cb := NewChainBuilder(Config{})
	cb.StoreWantTypeDefinition(def)

	want := &Want{}
	want.Metadata.Type = "echo_monitor"
	want.Metadata.Name = "test-echo"
	want.Spec.SetParam("limit", 5)

	result, err := cb.TestCreateWantFunction(want)
	if err != nil {
		t.Fatalf("Factory error: %v", err)
	}

	sw, ok := result.(*ScriptableWant)
	if !ok {
		t.Fatalf("expected *ScriptableWant, got %T", result)
	}

	t.Logf("Factory created: %T", sw)
	t.Logf("Requires after registration: %v", def.Requires)

	if len(def.Requires) == 0 {
		t.Error("expected Requires to be populated by registerInlineAgents")
	}
}

func TestEvaluateAchievedWhen(t *testing.T) {
	cases := []struct {
		actual   any
		op       string
		expected any
		want     bool
	}{
		{80, "<", 90, true},
		{90, "<", 80, false},
		{100, ">=", 100, true},
		{99, ">=", 100, false},
		{"done", "==", "done", true},
		{"done", "!=", "pending", true},
		{3, "==", 3, true},
	}

	for _, c := range cases {
		got := evaluateAchievedWhen(c.actual, c.op, c.expected)
		if got != c.want {
			t.Errorf("evaluateAchievedWhen(%v, %s, %v) = %v, want %v",
				c.actual, c.op, c.expected, got, c.want)
		}
	}
}
