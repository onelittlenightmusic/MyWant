package mywant

// OnWantChanged is told the id of a want that changed — its state, its
// labels, its status, its spec, or its coming and going — wherever the change
// was made: a handler, a want's own Progress, an agent, a tick inside the
// server. The server says so to every page (board_changed), which reads the
// want again.
//
// One hook at the source rather than a call beside each change: a change made
// from inside the engine — a want moving another's label, a plugin's script
// writing a want's state — had no handler there to say it, and a page saw it only on
// its slow catch-up poll. Volatile state (see the state's `volatile`) is left
// out, as it is of the want's hash: it changes too often to be news.
var OnWantChanged func(id string)

// changed says this want changed (OnWantChanged).
func (n *Want) changed() {
	if OnWantChanged != nil && n != nil && n.Metadata.ID != "" {
		OnWantChanged(n.Metadata.ID)
	}
}

// wantChangedByID says the want with this id changed — for one being deleted,
// which no longer has a want to ask.
func wantChangedByID(id string) {
	if OnWantChanged != nil && id != "" {
		OnWantChanged(id)
	}
}

// savesQuietly: a change to this field does not ask for the memory file to be
// written — a type's volatile state, and the engine's bookkeeping
// (bookkeepingStateFields). Its value is still in the want and goes into the
// file with the next write any other change asks for, or the whole rebuild
// (memoryRebuildEvery). On 2026-11-17 a run of smartgolf-available rewrote its
// 1.3MB want for a progress line and a percentage while its answer stood.
//
// So does a field the type declares `persistent: false` — one the restart sets
// back to its initialValue (Want's reset on restart): written, it would only be
// thrown away. Unless the want keeps all of its state over a restart
// (resetOnRestart: false), and except a goal, which Initialize sets again. A
// robot's last_poll_at, declared so, wrote the whole memory file every five
// seconds by itself.
func (n *Want) savesQuietly(key string) bool {
	if n.isVolatileState(key) {
		return true
	}
	for _, k := range bookkeepingStateFields {
		if k == key {
			return true
		}
	}
	return n.resetOnRestart(key)
}

// resetOnRestart: the restart sets this field back to its initialValue.
func (n *Want) resetOnRestart(key string) bool {
	if n.WantTypeDefinition == nil || (n.Spec.ResetOnRestart != nil && !*n.Spec.ResetOnRestart) {
		return false
	}
	for _, sd := range n.WantTypeDefinition.State {
		if sd.Name == key {
			return !sd.Persistent && sd.Label != WantTypeStateLabelGoal
		}
	}
	return false
}

// isVolatileState: the type declares this field volatile.
func (n *Want) isVolatileState(key string) bool {
	if n.WantTypeDefinition == nil {
		return false
	}
	for _, sd := range n.WantTypeDefinition.State {
		if sd.Name == key {
			return sd.Volatile
		}
	}
	return false
}
