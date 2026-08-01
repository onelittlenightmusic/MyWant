package mywant

import (
	"strings"
	"testing"

	want_spec "github.com/onelittlenightmusic/want-spec"
)

// newCorrelationTestBuilder wires up the minimum ChainBuilder state that
// buildStateAccessIndex and correlationPhase read, so a correlation can be
// computed without standing up a whole chain.
func newCorrelationTestBuilder(wants ...*Want) *ChainBuilder {
	cb := NewChainBuilder(nil)
	cb.wants = make(map[string]*runtimeWant, len(wants))
	cb.wantNameToID = make(map[string]string, len(wants))
	for _, w := range wants {
		cb.wants[w.Metadata.ID] = &runtimeWant{want: w}
		cb.wantNameToID[w.Metadata.Name] = w.Metadata.ID
	}
	return cb
}

func correlationFor(w *Want, peerID string) (CorrelationEntry, bool) {
	for _, e := range w.Metadata.Correlation {
		if e.WantID == peerID {
			return e, true
		}
	}
	return CorrelationEntry{}, false
}

func hasLabel(entry CorrelationEntry, label string) bool {
	for _, l := range entry.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// Two wants chained through a global parameter — one publishes a state field
// with exposes.asGlobalParam, the other reads it back with
// params.fromGlobalParam — are correlated. This is the transit-search shape: a
// second search starts where the first arrived, and used to report no
// relationship at all because only global STATE links were indexed.
func TestCorrelationViaGlobalParam(t *testing.T) {
	provider := &Want{
		Metadata: Metadata{ID: "want-a", Name: "transit-search-instance", Type: "transit_search"},
		Spec: WantSpec{
			Exposes: []want_spec.ExposeEntry{
				{CurrentState: "to", AsGlobalParam: "transit_search_instance_to"},
				{CurrentState: "arrival", AsGlobalParam: "transit_search_instance_arrival"},
			},
		},
	}
	consumer := &Want{
		Metadata: Metadata{ID: "want-b", Name: "transit-search-copy", Type: "transit_search"},
		Spec: WantSpec{
			Params: map[string]any{
				"from": map[string]any{"fromGlobalParam": "transit_search_instance_to"},
				"time": map[string]any{"fromGlobalParam": "transit_search_instance_arrival"},
				"to":   "戸塚",
			},
		},
	}

	cb := newCorrelationTestBuilder(provider, consumer)
	cb.buildStateAccessIndex()
	cb.correlationPhase()

	fromConsumer, ok := correlationFor(consumer, "want-a")
	if !ok {
		t.Fatalf("consumer has no correlation with the provider; got %+v", consumer.Metadata.Correlation)
	}
	for _, want := range []string{
		"stateAccess/provider:param/transit_search_instance_to",
		"stateAccess/provider:param/transit_search_instance_arrival",
	} {
		if !hasLabel(fromConsumer, want) {
			t.Errorf("consumer is missing label %q; got %v", want, fromConsumer.Labels)
		}
	}
	if fromConsumer.Rate <= 0 {
		t.Errorf("expected a positive correlation rate, got %d", fromConsumer.Rate)
	}

	// Correlation is reciprocal: the provider must see the consumer too.
	fromProvider, ok := correlationFor(provider, "want-b")
	if !ok {
		t.Fatalf("provider has no correlation with the consumer; got %+v", provider.Metadata.Correlation)
	}
	if !hasLabel(fromProvider, "stateAccess/consumer:param/transit_search_instance_to") {
		t.Errorf("provider is missing the consumer label; got %v", fromProvider.Labels)
	}
}

// spec.when reads a global parameter too, and ResolveFromGlobalParams keeps the
// reference in place after filling in at/every, so the link stays visible.
func TestCorrelationViaGlobalParamInWhen(t *testing.T) {
	timer := &Want{
		Metadata: Metadata{ID: "want-timer", Name: "timer-instance", Type: "timer"},
		Spec: WantSpec{
			Exposes: []want_spec.ExposeEntry{
				{CurrentState: "schedule", AsGlobalParam: "global_timer_1"},
			},
		},
	}
	reminder := &Want{
		Metadata: Metadata{ID: "want-reminder", Name: "reminder-instance", Type: "reminder"},
		Spec: WantSpec{
			// Resolved form: at/every filled in, origin retained.
			When: []want_spec.WhenSpec{{Every: "30s", FromGlobalParam: "global_timer_1"}},
		},
	}

	cb := newCorrelationTestBuilder(timer, reminder)
	cb.buildStateAccessIndex()
	cb.correlationPhase()

	entry, ok := correlationFor(reminder, "want-timer")
	if !ok {
		t.Fatalf("reminder has no correlation with the timer; got %+v", reminder.Metadata.Correlation)
	}
	if !hasLabel(entry, "stateAccess/provider:param/global_timer_1") {
		t.Errorf("missing the when-based label; got %v", entry.Labels)
	}
}

// Reading a global parameter nobody publishes is not a relationship, and a want
// that reads a parameter it publishes itself is not correlated with itself.
func TestCorrelationViaGlobalParamNoPhantomLinks(t *testing.T) {
	orphan := &Want{
		Metadata: Metadata{ID: "want-a", Name: "orphan", Type: "x"},
		Spec: WantSpec{
			Params: map[string]any{"p": map[string]any{"fromGlobalParam": "nobody_publishes_this"}},
		},
	}
	selfRef := &Want{
		Metadata: Metadata{ID: "want-b", Name: "self", Type: "y"},
		Spec: WantSpec{
			Exposes: []want_spec.ExposeEntry{{CurrentState: "f", AsGlobalParam: "own_key"}},
			Params:  map[string]any{"p": map[string]any{"fromGlobalParam": "own_key"}},
		},
	}

	cb := newCorrelationTestBuilder(orphan, selfRef)
	cb.buildStateAccessIndex()
	cb.correlationPhase()

	for _, w := range []*Want{orphan, selfRef} {
		for _, e := range w.Metadata.Correlation {
			for _, l := range e.Labels {
				if strings.Contains(l, "param/") {
					t.Errorf("want %q gained an unexpected global-param correlation with %q: %v",
						w.Metadata.Name, e.WantID, e.Labels)
				}
			}
		}
	}
}

// Both channels can be in play at once, and each keeps its own label namespace
// so the two are still tellable apart downstream.
func TestCorrelationGlobalParamAndExposeCoexist(t *testing.T) {
	provider := &Want{
		Metadata: Metadata{ID: "want-a", Name: "provider", Type: "p"},
		Spec: WantSpec{
			Exposes: []want_spec.ExposeEntry{
				{CurrentState: "f1", As: "state_key"},
				{CurrentState: "f2", AsGlobalParam: "param_key"},
			},
		},
	}
	consumer := &Want{
		Metadata: Metadata{ID: "want-b", Name: "consumer", Type: "c"},
		Spec: WantSpec{
			Imports: map[string]string{"state_key": "local"},
			Params:  map[string]any{"p": map[string]any{"fromGlobalParam": "param_key"}},
		},
	}

	cb := newCorrelationTestBuilder(provider, consumer)
	cb.buildStateAccessIndex()
	cb.correlationPhase()

	entry, ok := correlationFor(consumer, "want-a")
	if !ok {
		t.Fatalf("consumer has no correlation with the provider")
	}
	if !hasLabel(entry, "stateAccess/provider:expose/state_key") {
		t.Errorf("missing the global-state label; got %v", entry.Labels)
	}
	if !hasLabel(entry, "stateAccess/provider:param/param_key") {
		t.Errorf("missing the global-param label; got %v", entry.Labels)
	}
}
