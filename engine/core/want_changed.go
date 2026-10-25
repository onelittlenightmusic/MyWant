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
