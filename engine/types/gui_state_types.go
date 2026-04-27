package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[GUIStateWant, GUIStateLocals]("gui_state")
	})
}

// GUIStateLocals holds type-specific local state (none needed).
type GUIStateLocals struct{}

// GUIStateWant is a passive state holder that tracks the web dashboard display
// state. It is auto-created on server startup and its state resets on restart.
type GUIStateWant struct {
	Want
}

func (g *GUIStateWant) GetLocals() *GUIStateLocals {
	return CheckLocalsInitialized[GUIStateLocals](&g.Want)
}

func (g *GUIStateWant) Initialize() {
	g.StoreState("source", "")
	g.StoreState("dashboard_status_filter", "")
	g.StoreState("dashboard_search_query", "")
	g.StoreState("sidebar_open", false)
	g.StoreState("sidebar_want_id", "")
	g.StoreState("sidebar_active_tab", "")
}

// IsAchieved always returns false — gui_state is a persistent control want.
func (g *GUIStateWant) IsAchieved() bool { return false }

// Progress is a no-op; gui_state is updated externally via the state API.
func (g *GUIStateWant) Progress() {}
