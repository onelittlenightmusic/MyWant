package commands

import "mywant/engine/server"

// SetVersion hands main's build version (stamped from the git tag via -X, or
// "dev" for an unstamped build) to the server package, so a server started by
// this binary reports its real version on /health instead of a placeholder.
func SetVersion(v string) {
	server.SetVersion(v)
}
