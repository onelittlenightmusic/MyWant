package mywant

import "testing"

func archiveTestBuilder(wants ...*Want) *ChainBuilder {
	cb := NewChainBuilder(nil)
	for _, w := range wants {
		cb.wants[w.Metadata.ID] = &runtimeWant{want: w}
	}
	return cb
}

// A want that is not running is put away by the reconcile itself; one that
// finished remembers how, so taking it out does not run it again.
func TestArchivePhaseKeepsAFinishedWantFinished(t *testing.T) {
	w := &Want{Metadata: Metadata{ID: "w1", Name: "w1"}}
	w.SetStatus(WantStatusAchieved)
	w.SetLabel(ArchivedLabel, "true")
	cb := archiveTestBuilder(w)

	cb.archivePhase()
	if got := w.GetStatus(); got != WantStatusArchived {
		t.Fatalf("archived want: status %s, want archived", got)
	}

	w.DeleteLabel(ArchivedLabel)
	cb.archivePhase()
	if got := w.GetStatus(); got != WantStatusAchieved {
		t.Fatalf("unarchived achieved want: status %s, want achieved", got)
	}
	if _, ok := w.LookupLabel(archivedStatusLabel); ok {
		t.Fatal("the remembered status outlived the archive")
	}
}

// A want archived mid-run is taken out as idle — the start phase runs it again.
func TestArchivePhaseRestartsAnUnfinishedWant(t *testing.T) {
	w := &Want{Metadata: Metadata{ID: "w1", Name: "w1"}}
	w.SetStatus(WantStatusReaching)
	w.SetLabel(ArchivedLabel, "true")
	cb := archiveTestBuilder(w)

	cb.archivePhase()
	if got := w.GetStatus(); got != WantStatusArchived {
		t.Fatalf("archived want: status %s, want archived", got)
	}
	w.DeleteLabel(ArchivedLabel)
	cb.archivePhase()
	if got := w.GetStatus(); got != WantStatusIdle {
		t.Fatalf("unarchived want: status %s, want idle", got)
	}
}

// A running want is left to its own loop, which reads archiveHeldNow.
func TestArchivePhaseLeavesARunningWantToItsLoop(t *testing.T) {
	w := &Want{Metadata: Metadata{ID: "w1", Name: "w1"}}
	w.SetStatus(WantStatusReaching)
	w.SetGoroutineActive(true)
	w.SetLabel(ArchivedLabel, "true")
	cb := archiveTestBuilder(w)

	cb.archivePhase()
	if got := w.GetStatus(); got != WantStatusReaching {
		t.Fatalf("running want changed under its loop: %s", got)
	}
	if !w.archiveHeldNow() {
		t.Fatal("the loop would not see the archive")
	}
}

// Archiving a parent puts away the wants it owns.
func TestArchivePhaseCarriesToOwnedWants(t *testing.T) {
	parent := &Want{Metadata: Metadata{ID: "p", Name: "p"}}
	parent.SetStatus(WantStatusAchieved)
	parent.SetLabel(ArchivedLabel, "true")
	child := &Want{Metadata: Metadata{ID: "c", Name: "c", OwnerReferences: []OwnerReference{{Kind: "Want", ID: "p", Controller: true}}}}
	child.SetStatus(WantStatusIdle)
	cb := archiveTestBuilder(parent, child)

	cb.archivePhase()
	if got := child.GetStatus(); got != WantStatusArchived {
		t.Fatalf("child of an archived want: status %s, want archived", got)
	}
	parent.DeleteLabel(ArchivedLabel)
	cb.archivePhase()
	if got := child.GetStatus(); got != WantStatusIdle {
		t.Fatalf("child after its parent came back: status %s, want idle", got)
	}
}
