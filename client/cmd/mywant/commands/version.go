package commands

import mywant "mywant/engine/core"

// SetVersion hands main's build version (stamped from the git tag via -X, or
// "dev" for an unstamped build) to the engine, so a server started by this
// binary reports its real version on /health — and on the want types bundled
// into it — instead of a placeholder.
func SetVersion(v string) {
	mywant.SetVersion(v)
}
