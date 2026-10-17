package mywant

import "testing"

func TestScheduleOfFallsBackToTypeDefaultEvery(t *testing.T) {
	def := &WantTypeDefinition{}
	def.Metadata.Labels = map[string]string{defaultEveryLabel: " 1h "}

	w := &Want{WantTypeDefinition: def}
	if got := scheduleOf(w); len(got) != 1 || got[0].Every != "1h" {
		t.Fatalf("type default: got %+v, want every 1h", got)
	}

	w.Spec.When = []WhenSpec{{Every: "10m"}}
	if got := scheduleOf(w); len(got) != 1 || got[0].Every != "10m" {
		t.Fatalf("own spec.when wins: got %+v", got)
	}

	if got := scheduleOf(&Want{}); got != nil {
		t.Fatalf("no schedule: got %+v", got)
	}
}
