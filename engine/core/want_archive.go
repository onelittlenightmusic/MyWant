package mywant

// want_archive.go — putting a want away.
//
// An archived want is kept, not run: its metadata, its state and its history
// stay exactly as they were, and its progression loop and agents stop. The
// archive is a label rather than an API call, because the label is how every
// client has always archived (the GUI's archive drawer, the canvas bin), and a
// rule the engine keeps on its own labels holds whichever of them wrote it.
//
// Archiving a want puts away the wants it owns with it: a recipe whose parent
// is in the drawer is not still working underneath it.

// ArchivedLabel marks a want (or a thing) as put away.
const ArchivedLabel = "mywant.io/archived"

// archivedStatusLabel remembers how a want had ended when it was put away, so
// taking it back out returns it to that rather than running it again. Only a
// finished status is kept: a want that was still working is simply restarted.
const archivedStatusLabel = "mywant.io/archived-status"

// IsArchived reports whether the want itself carries the archive label.
func (n *Want) IsArchived() bool {
	return n.GetLabel(ArchivedLabel) == "true"
}

// archiveHeldNow is what the progression loop asks: put away by its own label,
// or by an owner's (as the last archive phase found it).
func (n *Want) archiveHeldNow() bool {
	return n.IsArchived() || n.archiveHeld.Load()
}

// finishedStatus is a status that unarchiving must not undo by running again.
func finishedStatus(s WantStatus) bool {
	switch s {
	case WantStatusAchieved, WantStatusAchievedWithWarning, WantStatusFailed,
		WantStatusTerminated, WantStatusCancelled, WantStatusModuleError:
		return true
	}
	return false
}

// putAway moves the want into the archived status, remembering a finished
// status it had. Its goroutine, if any, must already be on its way out.
func (n *Want) putAway(from WantStatus) {
	if from == WantStatusArchived {
		return
	}
	if finishedStatus(from) {
		n.SetLabel(archivedStatusLabel, string(from))
	}
	n.StoreLog("[ARCHIVE] Want '%s' archived (was %s)", n.Metadata.Name, from)
	n.SetStatus(WantStatusArchived)
}

// takeOut returns an archived want to where it was: the finished status it had,
// or idle — which the start phase picks up and runs.
func (n *Want) takeOut() {
	prior := WantStatus(n.GetLabel(archivedStatusLabel))
	n.DeleteLabel(archivedStatusLabel)
	n.StoreLog("[ARCHIVE] Want '%s' unarchived", n.Metadata.Name)
	if finishedStatus(prior) {
		n.SetStatus(prior)
		return
	}
	n.RestartWant()
}

// archivedInLineage reports whether the want or any want owning it is archived.
func (cb *ChainBuilder) archivedInLineage(w *Want) bool {
	for depth := 0; w != nil && depth < 32; depth++ {
		if w.IsArchived() {
			return true
		}
		var parent *Want
		for _, ref := range w.Metadata.OwnerReferences {
			if ref.Kind == "Want" && ref.Controller {
				if rw, ok := cb.wants[ref.ID]; ok {
					parent = rw.want
				}
				break
			}
		}
		w = parent
	}
	return false
}

// archivePhase brings every want's status in line with its archive label. Runs
// in the reconcile, before the start phase, so a want just taken out starts in
// the same pass and a want just put away is not started.
//
// A want whose goroutine is still running is left to stop itself: the loop
// checks archiveHeldNow every cycle and leaves through putAway, which keeps the
// "stop agents, then say archived" order in one place.
func (cb *ChainBuilder) archivePhase() {
	for _, rw := range cb.wants {
		w := rw.want
		held := cb.archivedInLineage(w)
		w.archiveHeld.Store(held)
		status := w.GetStatus()
		switch {
		case held && status != WantStatusArchived && status != WantStatusDeleting:
			if w.goroutineActive.Load() {
				continue
			}
			w.putAway(status)
		case !held && status == WantStatusArchived:
			w.takeOut()
		}
	}
}
