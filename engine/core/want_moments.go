package mywant

// want_moments.go — what a want just did that the board shows for a moment.
//
// A mark over a tile (？ when it went wrong), an effect landing on another
// want (its `impact`), a line it says (its `say`): none of it is the want's
// state in the sense a page reads it for — it is gone in a second, and a page
// that was not looking has missed nothing. Told through board_changed, each
// one sent every page back for the whole want, to find the one field that had
// moved. So the engine says them here as they happen, and the server sends
// each as a small `moment` of its own (server/moments.go), carrying what
// drawing it needs.

// OnWantMoment, when set, hears a want's state key written (value, and what
// it was before) and its status changing (key "status"). Run on a goroutine of
// its own, so the listener may take any lock. Set once, by the server, before
// any want runs.
var OnWantMoment func(n *Want, key string, value, previous any)

// MomentStatusKey is the key a status change is told under.
const MomentStatusKey = "status"

// momentKeys are the keys OnWantMoment is told about — the rest of a want's
// state is not a moment.
var momentKeys = map[string]bool{"error": true, ImpactField: true, "say": true}

// momentOnlyKeys are state keys that are nothing but a moment: written, they
// are told as one (the server's `moment`) and are not a change of the want a
// page reads it again for (OnWantChanged). The state still holds them.
var momentOnlyKeys = map[string]bool{ImpactField: true, "say": true}

// IsMomentOnlyKey: key is written only to be shown for a moment.
func IsMomentOnlyKey(key string) bool { return momentOnlyKeys[key] }

// wantMoment tells OnWantMoment, for the keys it listens to.
func (n *Want) wantMoment(key string, value, previous any) {
	f := OnWantMoment
	if f == nil || n == nil || (key != MomentStatusKey && !momentKeys[key]) {
		return
	}
	go f(n, key, value, previous)
}
