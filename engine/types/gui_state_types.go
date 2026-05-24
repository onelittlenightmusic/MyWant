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

	// Robot cursor fields — used by mywant-gui robot overlay
	g.StoreState("robot_visible", false)
	g.StoreState("robot_message", "")
	g.StoreState("robot_target_type", "")    // "want_card" | "nav_wants" | "nav_agents" | "nav_types" | "nav_recipes" | "param_field" | "none"
	g.StoreState("robot_target_id", "")      // want ID, param key, or nav route target
	g.StoreState("robot_action", "")         // "click" | "hover" | "type" | "navigate" | ""
	g.StoreState("robot_action_payload", "") // for "type" action: value to display
	g.StoreState("robot_nonce", int64(0))    // increment each command to re-trigger same target
	g.StoreState("nav_route", "")
	g.StoreState("sidebar_settings_subtab", "") // "params" | "name" | "labels" etc.            // route to navigate to: "/agents", "/recipes", etc.
}

// IsAchieved always returns false — gui_state is a persistent control want.
func (g *GUIStateWant) IsAchieved() bool { return false }

// Progress is a no-op; gui_state is updated externally via the state API.
func (g *GUIStateWant) Progress() {}
